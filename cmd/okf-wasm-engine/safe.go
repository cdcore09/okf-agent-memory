package main

import (
	"errors"
	"fmt"

	"github.com/okf-memory/okf-agent-memory/pkg/sync"
)

// safeCall runs fn and converts a panic into an error, so one bad request
// cannot exit the Go runtime that every later call depends on.
func safeCall(fn func() (string, error)) (v string, err error) {
	defer func() {
		if r := recover(); r != nil {
			v, err = "", fmt.Errorf("engine panic: %v", r)
		}
	}()
	return fn()
}

// syncOutcome maps Engine.Sync's result to the commit hash. A StateSaveError
// means the hub accepted the commit and only local state wasn't persisted,
// which is a success.
func syncOutcome(res *sync.SyncResult, err error) (string, error) {
	var saveErr *sync.StateSaveError
	if err != nil && !errors.As(err, &saveErr) {
		return "", err
	}
	if res == nil {
		return "", errors.New("sync returned no result")
	}
	return res.CommitHash, nil
}
