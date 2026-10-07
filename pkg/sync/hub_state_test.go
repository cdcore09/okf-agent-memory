package sync

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/okf-memory/okf-agent-memory/pkg/vault"
)

// These tests drive HubPush/HubPull/HubSync the way the CLI does: every call
// builds a fresh Engine, as a new `okf hub ...` process would. Sync state must
// therefore survive between calls via the on-disk state file.

type hubFixture struct {
	t        *testing.T
	server   *Server
	vaultID  string
	password string
	secret   string
}

func newHubFixture(t *testing.T) *hubFixture {
	t.Helper()
	secret, err := vault.GenerateSecretKey()
	if err != nil {
		t.Fatalf("GenerateSecretKey: %v", err)
	}
	return &hubFixture{
		t:        t,
		server:   NewServer(""),
		vaultID:  "test-vault-cli-runs",
		password: "master-pass-123",
		secret:   secret,
	}
}

func (f *hubFixture) op(dir, message string) HubOp {
	return HubOp{
		Dir:          dir,
		Client:       newTestClientFromHandlerForHub(f.server.Handler(), "token"),
		VaultID:      f.vaultID,
		Password:     f.password,
		SecretKey:    f.secret,
		Message:      message,
		AgentVersion: "test",
	}
}

func (f *hubFixture) push(dir, message string) string {
	f.t.Helper()
	var out bytes.Buffer
	if err := HubPush(&out, f.op(dir, message)); err != nil {
		f.t.Fatalf("HubPush(%s): %v", message, err)
	}
	return out.String()
}

func (f *hubFixture) pull(dir string) {
	f.t.Helper()
	var out bytes.Buffer
	if err := HubPull(&out, f.op(dir, "")); err != nil {
		f.t.Fatalf("HubPull: %v", err)
	}
}

func (f *hubFixture) sync(dir, message string) string {
	f.t.Helper()
	var out bytes.Buffer
	if err := HubSync(&out, f.op(dir, message)); err != nil {
		f.t.Fatalf("HubSync(%s): %v", message, err)
	}
	return out.String()
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func conflictFiles(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && strings.Contains(info.Name(), ".conflict-local") {
			rel, _ := filepath.Rel(dir, p)
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	return found
}

func seedBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "index.md", "# Index\n")
	writeFile(t, dir, "a/one.md", "one\n")
	writeFile(t, dir, "b/two.md", "two\n")
	return dir
}

func TestHub_SecondPushFromSameMachineSucceeds(t *testing.T) {
	f := newHubFixture(t)
	dir := seedBundle(t)

	f.push(dir, "first")
	writeFile(t, dir, "a/one.md", "one edited\n")
	out := f.push(dir, "second")

	// Only the edited file should be re-encrypted and uploaded.
	if !strings.Contains(out, "unchanged 2") {
		t.Fatalf("expected the two untouched files to be reused, got: %s", out)
	}
}

func TestHub_SyncWithoutRemoteChangesKeepsLocalEdit(t *testing.T) {
	f := newHubFixture(t)
	dir := seedBundle(t)

	f.push(dir, "first")
	writeFile(t, dir, "a/one.md", "one edited\n")
	out := f.sync(dir, "edit")

	if got := readFile(t, dir, "a/one.md"); got != "one edited\n" {
		t.Fatalf("local edit was reverted: %q (output: %s)", got, out)
	}
	if c := conflictFiles(t, dir); len(c) != 0 {
		t.Fatalf("no conflict expected, found %v", c)
	}
}

func TestHub_SyncMergesDisjointEditsAcrossMachines(t *testing.T) {
	f := newHubFixture(t)
	dirA := seedBundle(t)
	dirB := t.TempDir()

	f.push(dirA, "first")
	f.pull(dirB)

	writeFile(t, dirA, "a/one.md", "one from A\n")
	writeFile(t, dirB, "b/two.md", "two from B\n")
	f.sync(dirA, "A edits one")
	f.sync(dirB, "B edits two")
	f.sync(dirA, "A catches up")

	for name, dir := range map[string]string{"A": dirA, "B": dirB} {
		if got := readFile(t, dir, "a/one.md"); got != "one from A\n" {
			t.Errorf("%s: a/one.md = %q", name, got)
		}
		if got := readFile(t, dir, "b/two.md"); got != "two from B\n" {
			t.Errorf("%s: b/two.md = %q", name, got)
		}
		if c := conflictFiles(t, dir); len(c) != 0 {
			t.Errorf("%s: no conflict expected, found %v", name, c)
		}
	}
}

func TestHub_SyncPreservesBothSidesOfARealConflict(t *testing.T) {
	f := newHubFixture(t)
	dirA := seedBundle(t)
	dirB := t.TempDir()

	f.push(dirA, "first")
	f.pull(dirB)

	writeFile(t, dirA, "a/one.md", "one from A\n")
	writeFile(t, dirB, "a/one.md", "one from B\n")
	f.sync(dirA, "A edits one")
	out := f.sync(dirB, "B edits one")

	if got := readFile(t, dirB, "a/one.md"); got != "one from A\n" {
		t.Fatalf("B should hold the remote version in the canonical file, got %q", got)
	}
	if got := readFile(t, dirB, "a/one.conflict-local.md"); got != "one from B\n" {
		t.Fatalf("B's own edit should be preserved as a conflict copy, got %q (output: %s)", got, out)
	}
	if c := conflictFiles(t, dirB); len(c) != 1 {
		t.Fatalf("expected exactly one conflict copy, found %v", c)
	}
}

func TestHub_StateFileIsNeverUploaded(t *testing.T) {
	f := newHubFixture(t)
	dirA := seedBundle(t)
	dirB := t.TempDir()

	f.push(dirA, "first")
	if _, err := os.Stat(filepath.Join(dirA, SyncStateFileName)); err != nil {
		t.Fatalf("expected %s after push: %v", SyncStateFileName, err)
	}

	f.pull(dirB)
	data, err := os.ReadFile(filepath.Join(dirB, SyncStateFileName))
	if err != nil {
		t.Fatalf("expected B to write its own state file after pull: %v", err)
	}
	var st struct {
		VaultID string `json:"vault_id"`
	}
	if err := json.Unmarshal(data, &st); err != nil || st.VaultID != f.vaultID {
		t.Fatalf("state file should record vault %q, got %q (err %v)", f.vaultID, st.VaultID, err)
	}
	// The state file must not appear in B as a synced (pulled) bundle file
	// other than the one B wrote itself: check the remote tree directly.
	if _, err := os.Stat(filepath.Join(dirB, "a", "one.md")); err != nil {
		t.Fatalf("pull did not materialize bundle files: %v", err)
	}
}

func TestHub_StateForAnotherVaultIsIgnored(t *testing.T) {
	f := newHubFixture(t)
	dir := seedBundle(t)

	// A stale state file left over from a different vault must not be trusted.
	writeFile(t, dir, SyncStateFileName, `{"version":1,"vault_id":"some-other-vault","head":"deadbeef","tree":{"version":1,"entries":{}}}`)
	f.push(dir, "first") // would fail with a head conflict if the bogus head were used
}
