// Command okf-wasm-engine runs okf's MCP handler and hub sync engine inside a
// JavaScript host (e.g. a Cloudflare Durable Object) via GOOS=js GOARCH=wasm.
package main

import (
	"bytes"
	"strings"

	"github.com/okf-memory/okf-agent-memory/pkg/okf"
)

// HandleRequest runs one JSON-RPC request line through okf's MCP server
// against bundleDir and returns the single response line.
func HandleRequest(bundleDir, requestLine string) string {
	var out bytes.Buffer
	_ = okf.RunMCPServerIO(bundleDir, strings.NewReader(requestLine+"\n"), &out)
	return strings.TrimSpace(out.String())
}
