// Command okf-wasm-engine runs okf's MCP handler and hub sync engine inside a
// JavaScript host (e.g. a Cloudflare Durable Object) via GOOS=js GOARCH=wasm.
package main

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/okf-memory/okf-agent-memory/pkg/okf"
)

// HandleRequest runs one JSON-RPC request line through okf's MCP server
// against bundleDir and returns the single response line.
//
// With okf's bundle cache on, read-only tools reuse the parsed bundle; any
// other tool call clears it before and after, so writes and the reads that
// follow always see the files on disk.
func HandleRequest(bundleDir, requestLine string) string {
	if !isReadOnlyCall(requestLine) {
		okf.InvalidateBundleCache()
		defer okf.InvalidateBundleCache()
	}
	var out bytes.Buffer
	_ = okf.RunMCPServerIO(bundleDir, strings.NewReader(requestLine+"\n"), &out)
	return strings.TrimSpace(out.String())
}

var readOnlyTools = map[string]bool{"okf_search": true, "okf_show": true, "okf_validate": true}

// isReadOnlyCall reports whether the request can be answered from a cached bundle.
// Anything unparseable or unknown counts as a possible write.
func isReadOnlyCall(requestLine string) bool {
	var req struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if json.Unmarshal([]byte(requestLine), &req) != nil {
		return false
	}
	if req.Method != "tools/call" {
		return true // initialize, tools/list, ...: no file access
	}
	return readOnlyTools[req.Params.Name]
}

// InvalidateBundle drops the cached bundle; the host calls it when a pull or
// sync changes files on disk.
func InvalidateBundle() { okf.InvalidateBundleCache() }
