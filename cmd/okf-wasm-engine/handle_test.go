package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/okf-memory/okf-agent-memory/pkg/okf"
)

func call(t *testing.T, dir, name string, args map[string]any) map[string]any {
	t.Helper()
	line, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args},
	})
	var resp map[string]any
	if err := json.Unmarshal([]byte(HandleRequest(dir, string(line))), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", resp)
	}
	return result
}

// One request line in, one response line out, with no initialize handshake:
// this is how the Durable Object drives okf's MCP handler.
func TestHandleRequestCreateThenSearch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "knowledge")
	if err := okf.InitBundle(dir); err != nil {
		t.Fatal(err)
	}
	created := call(t, dir, "okf_create", map[string]any{
		"concept_id": "decisions/alpha", "type": "Decision",
		"title": "Alpha", "description": "Alpha summary.",
	})
	if created["isError"] == true {
		t.Fatalf("create failed: %v", created)
	}
	found := call(t, dir, "okf_search", map[string]any{"query": "alpha"})
	text, _ := json.Marshal(found["content"])
	if !strings.Contains(string(text), "decisions/alpha") {
		t.Fatalf("search did not find the new concept: %s", text)
	}
}

func TestHandleRequestReturnsErrorForUnknownTool(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "knowledge")
	if err := okf.InitBundle(dir); err != nil {
		t.Fatal(err)
	}
	out := HandleRequest(dir, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"okf_nope","arguments":{}}}`)
	if !strings.Contains(out, `"id":7`) || (!strings.Contains(out, `"error"`) && !strings.Contains(out, `"isError":true`)) {
		t.Fatalf("expected an error response for id 7, got %s", out)
	}
}

// Read tools reuse the cached bundle; any other tool call clears it so okf's
// writes (and the reads that follow) always see the files on disk.
func TestHandleRequestCachesReadsAndInvalidatesOnWrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "knowledge")
	if err := okf.InitBundle(dir); err != nil {
		t.Fatal(err)
	}
	okf.SetBundleCacheEnabled(true)
	t.Cleanup(func() { okf.SetBundleCacheEnabled(false) })

	call(t, dir, "okf_search", map[string]any{"query": "anything"}) // warms the cache
	created := call(t, dir, "okf_create", map[string]any{
		"concept_id": "decisions/beta", "type": "Decision",
		"title": "Beta", "description": "Beta summary.",
	})
	if created["isError"] == true {
		t.Fatalf("create failed: %v", created)
	}
	found := call(t, dir, "okf_search", map[string]any{"query": "beta"})
	text, _ := json.Marshal(found["content"])
	if !strings.Contains(string(text), "decisions/beta") {
		t.Fatalf("a read after a write must see the write: %s", text)
	}
	shown := call(t, dir, "okf_show", map[string]any{"concept_id": "decisions/beta"})
	if shown["isError"] == true {
		t.Fatalf("show after create failed: %v", shown)
	}
}

// The host clears the cache itself when a pull changes files on disk.
func TestInvalidateBundleMakesPulledFilesVisible(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "knowledge")
	if err := okf.InitBundle(dir); err != nil {
		t.Fatal(err)
	}
	okf.SetBundleCacheEnabled(true)
	t.Cleanup(func() { okf.SetBundleCacheEnabled(false) })
	call(t, dir, "okf_search", map[string]any{"query": "gamma"})

	other := filepath.Join(t.TempDir(), "knowledge")
	_ = okf.InitBundle(other)
	call(t, other, "okf_create", map[string]any{"concept_id": "notes/gamma", "type": "Note", "title": "Gamma", "description": "Gamma summary."})
	data, _ := os.ReadFile(filepath.Join(other, "notes", "gamma.md"))
	_ = os.MkdirAll(filepath.Join(dir, "notes"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "notes", "gamma.md"), data, 0o644) // as a pull would

	InvalidateBundle()
	found := call(t, dir, "okf_search", map[string]any{"query": "gamma"})
	text, _ := json.Marshal(found["content"])
	if !strings.Contains(string(text), "notes/gamma") {
		t.Fatalf("pulled concept not visible after invalidation: %s", text)
	}
}
