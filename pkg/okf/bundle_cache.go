package okf

import "sync"

// bundleCache holds parsed bundles for long-lived hosts (the Wasm engine behind
// okf-hub's remote MCP) where re-reading every file on every call is the main
// cost, and where the host sees every change to the files and can invalidate.
// It is off by default: the CLI and the stdio MCP server always parse afresh.
var bundleCache struct {
	sync.Mutex
	enabled bool
	byRoot  map[string]*Bundle
}

// SetBundleCacheEnabled turns the cache on or off and empties it either way.
func SetBundleCacheEnabled(on bool) {
	bundleCache.Lock()
	defer bundleCache.Unlock()
	bundleCache.enabled = on
	bundleCache.byRoot = nil
}

// InvalidateBundleCache drops every cached bundle; call it whenever files change.
func InvalidateBundleCache() {
	bundleCache.Lock()
	defer bundleCache.Unlock()
	bundleCache.byRoot = nil
}

func cachedBundle(root string) (*Bundle, bool) {
	bundleCache.Lock()
	defer bundleCache.Unlock()
	if !bundleCache.enabled {
		return nil, false
	}
	b, ok := bundleCache.byRoot[root]
	return b, ok
}

func storeBundle(root string, b *Bundle) {
	bundleCache.Lock()
	defer bundleCache.Unlock()
	if !bundleCache.enabled {
		return
	}
	if bundleCache.byRoot == nil {
		bundleCache.byRoot = map[string]*Bundle{}
	}
	bundleCache.byRoot[root] = b
}
