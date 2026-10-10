package okf

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConcept(t *testing.T, dir, rel, title string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\ntype: Note\ntitle: " + title + "\ndescription: " + title + " summary.\n---\n\n# " + title + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A long-lived host (the Wasm engine) re-parsing every file on every call is
// the main cost of a remote tool call; the cache lets it parse once.
func TestBundleCacheReusesTheLoadedBundleUntilInvalidated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "knowledge")
	if err := InitBundle(dir); err != nil {
		t.Fatal(err)
	}
	writeConcept(t, dir, "notes/a.md", "A")
	SetBundleCacheEnabled(true)
	t.Cleanup(func() { SetBundleCacheEnabled(false) })

	first, err := LoadBundle(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeConcept(t, dir, "notes/b.md", "B") // a change the cache can't see
	second, _ := LoadBundle(dir)
	if second != first {
		t.Fatal("expected the cached bundle on the second load")
	}
	if _, ok := second.Concepts["notes/b"]; ok {
		t.Fatal("cached bundle should not include a file written after it was loaded")
	}

	InvalidateBundleCache()
	third, _ := LoadBundle(dir)
	if _, ok := third.Concepts["notes/b"]; !ok {
		t.Fatal("after invalidation the new concept must be loaded")
	}
}

func TestBundleCacheIsOffByDefault(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "knowledge")
	if err := InitBundle(dir); err != nil {
		t.Fatal(err)
	}
	first, _ := LoadBundle(dir)
	second, _ := LoadBundle(dir)
	if first == second {
		t.Fatal("without the cache every load must parse afresh")
	}
}
