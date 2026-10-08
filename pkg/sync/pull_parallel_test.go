package sync

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	stdsync "sync"
	"testing"
	"time"

	"github.com/okf-memory/okf-agent-memory/pkg/vault"
)

// slowBlobHub wraps the in-memory hub so blob downloads take a while and
// records how many are in flight at once.
type slowBlobHub struct {
	inner     http.Handler
	mu        stdsync.Mutex
	active    int
	maxActive int
}

func (h *slowBlobHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	isBlobGet := r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/")
	if isBlobGet {
		h.mu.Lock()
		h.active++
		if h.active > h.maxActive {
			h.maxActive = h.active
		}
		h.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		defer func() { h.mu.Lock(); h.active--; h.mu.Unlock() }()
	}
	h.inner.ServeHTTP(w, r)
}

// A cold Pull of many files must download blobs concurrently: one round trip
// per file in sequence is what made a fresh device (or a cold remote engine)
// slow, and it grows with the size of the vault.
func TestPullDownloadsBlobsConcurrently(t *testing.T) {
	hub := &slowBlobHub{inner: NewServer("").Handler()}
	key := make([]byte, 32)
	vaultID := "test-vault-parallel-pull"

	src := t.TempDir()
	for i := 0; i < 24; i++ {
		writeFile(t, src, fmt.Sprintf("notes/n%02d.md", i), fmt.Sprintf("note %d\n", i))
	}
	pusher := NewEngine(newTestClientFromHandlerForHub(hub, "t"), vaultID, key, src)
	if _, err := pusher.Push(t.Context(), vault.CommitAuthor{ClientID: "a", Agent: "test"}, "seed"); err != nil {
		t.Fatal(err)
	}

	dst := t.TempDir()
	puller := NewEngine(newTestClientFromHandlerForHub(hub, "t"), vaultID, key, dst)
	res, err := puller.Pull(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.UpdatedFiles) != 24 {
		t.Fatalf("expected 24 files pulled, got %d", len(res.UpdatedFiles))
	}
	for i := 0; i < 24; i++ {
		got, err := os.ReadFile(filepath.Join(dst, "notes", fmt.Sprintf("n%02d.md", i)))
		if err != nil || string(got) != fmt.Sprintf("note %d\n", i) {
			t.Fatalf("file %d wrong after parallel pull: %q, %v", i, got, err)
		}
	}
	if hub.maxActive < 2 {
		t.Fatalf("blob downloads ran one at a time (max in flight = %d)", hub.maxActive)
	}
}
