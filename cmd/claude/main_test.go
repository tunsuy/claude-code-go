package main

import (
	"os"
	"testing"

	"github.com/anthropics/claude-code-go/internal/bootstrap"
)

// TestMain restores os.Args after the test suite completes so that any
// parallel test infrastructure is not polluted.
func TestMain(m *testing.M) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Exit(m.Run())
}

// TestMain_HelpFlag verifies that passing --help to bootstrap.Run does not
// panic.  Cobra prints usage and returns an error (or nil) for --help; both
// outcomes are acceptable here — the important invariant is no panic.
func TestMain_HelpFlag(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("bootstrap.Run panicked with --help: %v", r)
		}
	}()

	// bootstrap.Run uses the supplied args slice directly; it does NOT call
	// os.Exit, so this is safe to call from a test.
	_ = bootstrap.Run([]string{"claude", "--help"})
}

// TestMain_VersionFlag verifies that HandleFastPath returns true when
// --version is supplied, indicating the process should exit cleanly.
func TestMain_VersionFlag(t *testing.T) {
	os.Args = []string{"claude", "--version"}

	handled := bootstrap.HandleFastPath(os.Args)
	if !handled {
		t.Error("HandleFastPath returned false for --version, expected true")
	}
}
