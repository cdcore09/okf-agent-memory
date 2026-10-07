package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/okf-memory/okf-agent-memory/pkg/vault"
)

// SyncStateFileName is the per-bundle file holding the last-synced head and tree.
//
// The engine needs the last commit it agreed on with the hub (expected head for
// the compare-and-swap) and that commit's tree (the base of the 3-way reconcile
// and the reference for plaintext-hash change detection). Without it, each new
// process starts from an empty base: a second push is rejected with a head
// conflict, every edit looks like a same-file collision during sync, and every
// file is re-encrypted on each push.
//
// The file lives next to .okf-vault.json, is never uploaded (ScanBundle skips
// dotfiles), and records only paths and hashes of files already present in
// plaintext on this machine.
const SyncStateFileName = ".okf-sync-state.json"

const syncStateVersion = 1

type syncState struct {
	Version int             `json:"version"`
	VaultID string          `json:"vault_id"`
	Head    string          `json:"head"`
	Tree    json.RawMessage `json:"tree"`
}

// LoadState enables persistent sync state at path and restores the last-synced
// head and tree from it. A missing file, or one recorded for a different vault,
// leaves the engine in its fresh (never-synced) state.
func (e *Engine) LoadState(path string) error {
	e.statePath = path

	// #nosec G304 -- path is the engine's own state file inside the bundle
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sync: failed to read sync state: %w", err)
	}

	var st syncState
	if err := json.Unmarshal(data, &st); err != nil {
		return fmt.Errorf("sync: failed to parse sync state %s: %w", path, err)
	}
	if st.Version != syncStateVersion || st.VaultID != e.VaultID {
		return nil
	}

	tree, err := vault.ParseTree(st.Tree)
	if err != nil {
		return fmt.Errorf("sync: invalid tree in sync state %s: %w", path, err)
	}

	e.cachedHead = st.Head
	e.cachedTree = tree
	return nil
}

// saveState persists the engine's current head and tree when state is enabled.
// It writes a temporary file and renames it so a crash never leaves a torn file.
func (e *Engine) saveState() error {
	if e.statePath == "" {
		return nil
	}

	treeBytes, err := e.cachedTree.Serialize()
	if err != nil {
		return fmt.Errorf("sync: failed to serialize tree for sync state: %w", err)
	}
	data, err := json.MarshalIndent(syncState{
		Version: syncStateVersion,
		VaultID: e.VaultID,
		Head:    e.cachedHead,
		Tree:    treeBytes,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("sync: failed to encode sync state: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(e.statePath), SyncStateFileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("sync: failed to save sync state: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync: failed to save sync state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("sync: failed to save sync state: %w", err)
	}
	if err := os.Rename(tmp.Name(), e.statePath); err != nil {
		return fmt.Errorf("sync: failed to save sync state: %w", err)
	}
	return nil
}

// StateSaveError reports that a hub operation succeeded remotely but the local
// sync state could not be written. The remote result is still valid; the next
// run will fall back to a full reconcile against the hub.
type StateSaveError struct {
	Err error
}

func (e *StateSaveError) Error() string {
	return fmt.Sprintf("hub operation succeeded but local sync state was not saved: %v", e.Err)
}

func (e *StateSaveError) Unwrap() error { return e.Err }
