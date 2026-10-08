package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/okf-memory/okf-agent-memory/pkg/sync"
)

// A panic inside one request must become that request's error, not exit the
// Go runtime that every later call depends on.
func TestSafeCallTurnsPanicIntoError(t *testing.T) {
	v, err := safeCall(func() (string, error) { panic("boom") })
	if err == nil || !strings.Contains(err.Error(), "boom") || v != "" {
		t.Fatalf("expected a recovered error mentioning boom, got v=%q err=%v", v, err)
	}
	if v, err := safeCall(func() (string, error) { return "ok", nil }); v != "ok" || err != nil {
		t.Fatalf("normal calls must pass through, got v=%q err=%v", v, err)
	}
}

// When the hub accepted the commit but local sync state could not be saved,
// the sync succeeded: report the commit, not a failure.
func TestSyncOutcomeTreatsStateSaveErrorAsSuccess(t *testing.T) {
	res := &sync.SyncResult{CommitHash: "abc"}
	if v, err := syncOutcome(res, &sync.StateSaveError{Err: errors.New("disk full")}); v != "abc" || err != nil {
		t.Fatalf("StateSaveError should be success, got v=%q err=%v", v, err)
	}
	if _, err := syncOutcome(nil, errors.New("head conflict")); err == nil {
		t.Fatal("real sync errors must still fail")
	}
}
