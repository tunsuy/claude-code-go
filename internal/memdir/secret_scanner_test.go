package memdir_test

import (
	"strings"
	"testing"

	"github.com/tunsuy/claude-code-go/internal/memdir"
)

func TestScanSecrets_BlocksAPIKey(t *testing.T) {
	err := memdir.ScanSecrets("token: sk-ant-api03-abcdefghijklmnopqrstuvwxyz")
	if err == nil {
		t.Fatal("expected secret scan error")
	}
}

func TestRedactSecrets(t *testing.T) {
	out := memdir.RedactSecrets("key=ghp_abcdefghijklmnopqrstuvwxyz123456")
	if strings.Contains(out, "ghp_") {
		t.Fatalf("expected redacted output, got %q", out)
	}
}
