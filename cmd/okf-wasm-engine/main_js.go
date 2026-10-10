//go:build js && wasm

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/okf-memory/okf-agent-memory/pkg/okf"
	"io"
	"net/http"
	"path/filepath"
	"syscall/js"

	"github.com/okf-memory/okf-agent-memory/pkg/sync"
	"github.com/okf-memory/okf-agent-memory/pkg/vault"
)

const bundleDir = "/vault/knowledge"

var engine *sync.Engine

// await blocks the calling goroutine until a JS promise settles.
func await(p js.Value) (js.Value, error) {
	ok := make(chan js.Value, 1)
	fail := make(chan error, 1)
	then := js.FuncOf(func(_ js.Value, a []js.Value) any { ok <- a[0]; return nil })
	catch := js.FuncOf(func(_ js.Value, a []js.Value) any { fail <- errors.New(a[0].Call("toString").String()); return nil })
	defer then.Release()
	defer catch.Release()
	p.Call("then", then).Call("catch", catch)
	select {
	case v := <-ok:
		return v, nil
	case err := <-fail:
		return js.Value{}, err
	}
}

// promise runs fn on a goroutine and exposes the outcome as a JS Promise.
func promise(fn func() (string, error)) js.Value {
	executor := js.FuncOf(func(_ js.Value, a []js.Value) any {
		resolve, reject := a[0], a[1]
		go func() {
			v, err := safeCall(fn) // a panic rejects this call instead of exiting the runtime
			if err != nil {
				reject.Invoke(js.Global().Get("Error").New(err.Error()))
				return
			}
			resolve.Invoke(v)
		}()
		return nil
	})
	return js.Global().Get("Promise").New(executor)
}

// hubTransport sends the sync client's requests to the host's okfHubFetch,
// which routes them to the hub in-process (no network hop).
type hubTransport struct{}

func (hubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := js.Null()
	if req.Body != nil {
		data, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		if len(data) > 0 {
			arr := js.Global().Get("Uint8Array").New(len(data))
			js.CopyBytesToJS(arr, data)
			body = arr
		}
	}
	res, err := await(js.Global().Call("okfHubFetch", req.Method, req.URL.RequestURI(), body))
	if err != nil {
		return nil, err
	}
	jsBody := res.Get("body")
	data := make([]byte, jsBody.Get("length").Int())
	js.CopyBytesToGo(data, jsBody)
	status := res.Get("status").Int()
	return &http.Response{
		Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), StatusCode: status,
		Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)),
		ContentLength: int64(len(data)), Request: req,
	}, nil
}

func initEngine(vaultID, keyHex string) error {
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return errors.New("vault key must be 64 hex characters")
	}
	client := sync.NewClient("https://hub.internal", "")
	client.HTTPClient.Transport = hubTransport{}
	e := sync.NewEngine(client, vaultID, key, bundleDir)
	if err := e.LoadState(filepath.Join(bundleDir, sync.SyncStateFileName)); err != nil {
		return err
	}
	engine = e
	lastPulledHead = ""
	InvalidateBundle()
	return nil
}

// lastPulledHead is the head the files on disk match; a pull that lands a
// different head has changed them, so the bundle cache is dropped.
var lastPulledHead string

func main() {
	// Long-lived host: parse the bundle once and reuse it until files change.
	okf.SetBundleCacheEnabled(true)
	// Every export returns a Promise and does its work on a new goroutine:
	// file I/O goes through async JS callbacks, and blocking on one inside a
	// synchronous js.FuncOf call deadlocks the event loop.
	js.Global().Set("okfEngineInit", js.FuncOf(func(_ js.Value, a []js.Value) any {
		vaultID, keyHex := a[0].String(), a[1].String()
		return promise(func() (string, error) {
			return "", initEngine(vaultID, keyHex)
		})
	}))
	js.Global().Set("okfEnginePull", js.FuncOf(func(_ js.Value, _ []js.Value) any {
		return promise(func() (string, error) {
			if engine == nil {
				return "", errors.New("engine not initialized")
			}
			res, err := engine.Pull(context.Background())
			if err != nil {
				InvalidateBundle() // a failed pull may have written some files
				return "", err
			}
			if res.CommitHash != lastPulledHead {
				InvalidateBundle()
				lastPulledHead = res.CommitHash
			}
			return res.CommitHash, nil
		})
	}))
	js.Global().Set("okfEngineSync", js.FuncOf(func(_ js.Value, a []js.Value) any {
		message := a[0].String()
		return promise(func() (string, error) {
			if engine == nil {
				return "", errors.New("engine not initialized")
			}
			author := vault.CommitAuthor{ClientID: "okf-hub-mcp", Agent: "okf-wasm-engine"}
			// Sync can merge remote changes into the files, and the new head is
			// not one a later pull would detect as changed: always drop the cache.
			defer func() { InvalidateBundle(); lastPulledHead = "" }()
			return syncOutcome(engine.Sync(context.Background(), author, message))
		})
	}))
	js.Global().Set("okfEngineHandle", js.FuncOf(func(_ js.Value, a []js.Value) any {
		line := a[0].String()
		return promise(func() (string, error) {
			return HandleRequest(bundleDir, line), nil
		})
	}))
	select {}
}
