//go:build !(js && wasm)

package main

// The engine only runs as Wasm; this keeps `go test`/`go vet` happy natively.
func main() {}
