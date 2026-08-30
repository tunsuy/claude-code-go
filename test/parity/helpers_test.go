package parity

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)


// versionRe matches "claude 1.2.3" style version output so two builds with
// different version numbers can still be compared.
var versionRe = regexp.MustCompile(`(?m)^(\s*claude\s+)\d+\.\d+\.\d+`)

// oracleVersionRe matches the TS original's version line "2.1.251 (Claude Code)"
// (no binary-name prefix, trailing product tag) so it normalizes to the same
// placeholder as the Go side once the Go format is aligned (parity case A1).
var oracleVersionRe = regexp.MustCompile(`(?m)^(\s*)\d+\.\d+\.\d+\s+\(Claude Code\)`)

// absPathRe matches absolute home/tmp paths so machine-specific prefixes do
// not create false diffs.
var absPathRe = regexp.MustCompile(`/(Users|home|tmp|var)/[^\s,"']+`)

// runResult captures the observable behavior of one CLI invocation.
type runResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// runBinary executes the given binary with args and captures the three
// observable channels (stdout, stderr, exit code). A 15-second timeout guards
// against accidental interactive mode hanging the test run.
func runBinary(t *testing.T, binary string, args ...string) runResult {
	t.Helper()

	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(),
		"CLAUDE_CODE_GO_PARITY=1", // marker, lets binaries detect test harness
		"NO_COLOR=1",              // strip ANSI so string asserts are stable
		"TERM=dumb",
	)
	// Stdin stays nil (= /dev/null): an unexpected REPL must see EOF and exit,
	// and if it does not, the timeout below fails the test instead of hanging.
	cmd.Stdin = nil

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", binary, err)
	}
	go func() { done <- cmd.Wait() }()

	var waitErr error
	select {
	case waitErr = <-done:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("%s %v: timed out after 15s (interactive mode reached?)", binary, args)
	}

	exitCode := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("wait %s: %v", binary, waitErr)
		}
	}
	return runResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exitCode}
}

// oraclePath returns the configured original-binary path, or "" when absent.
func oraclePath() string { return strings.TrimSpace(os.Getenv("CCG_PARITY_ORACLE")) }

// targetPath returns the Go build path, defaulting to bin/claude at the repo
// root (two levels up from this package directory).
func targetPath() string {
	if p := strings.TrimSpace(os.Getenv("CCG_PARITY_TARGET")); p != "" {
		return p
	}
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "bin", "claude")
}

// requireBoth skips the test unless both binaries are available. Skipping
// (rather than failing) keeps CI green when no oracle is published — the
// comparison is opt-in by setting CCG_PARITY_ORACLE.
func requireBoth(t *testing.T) (oracle, target string) {
	t.Helper()

	oracle = oraclePath()
	if oracle == "" {
		t.Skip("CCG_PARITY_ORACLE not set — parity comparison is opt-in")
	}
	if _, err := os.Stat(oracle); err != nil {
		t.Skipf("oracle binary not found at %s: %v", oracle, err)
	}

	target = targetPath()
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target binary not found at %s — run `make build` first: %v", target, err)
	}
	return oracle, target
}

// requireTarget needs only the target binary; it runs even without an oracle
// variable. It SKIPS (not fails) when the target is absent so that a bare
// `go test ./...` without `make build` stays green — use `make parity`
// (build + test) to get hard failures.
func requireTarget(t *testing.T) string {
	t.Helper()

	if oracle := oraclePath(); oracle != "" {
		if _, err := os.Stat(oracle); err != nil {
			t.Skipf("oracle binary not found at %s: %v", oracle, err)
		}
	}

	target := targetPath()
	if _, err := os.Stat(target); err != nil {
		t.Skipf("target binary not found at %s — run `make parity` (build + test): %v", target, err)
	}
	return target
}

// normalizeVersion replaces concrete version numbers with a placeholder so
// "claude 1.2.3" and "claude 0.1.0" compare equal, and normalizes the
// oracle-style "2.1.251 (Claude Code)" to the same placeholder.
func normalizeVersion(line string) string {
	line = versionRe.ReplaceAllString(line, "${1}<version>")
	return oracleVersionRe.ReplaceAllString(line, "${1}<version> (Claude Code)")
}

// normalizePath replaces absolute paths with a placeholder.
func normalizePath(line string) string {
	return absPathRe.ReplaceAllString(line, "/<path>")
}

// normalizeOutput strips run-to-run noise before comparison: version numbers,
// absolute paths, and trailing whitespace.
func normalizeOutput(s string) string {
	lines := make([]string, 0)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t\r")
		line = normalizeVersion(line)
		line = normalizePath(line)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
