// Package parity — first batch of comparison cases.
//
// Case selection rule: only paths observable WITHOUT an LLM call (no API key,
// no network): --version, --help, subcommand tree, error messages. LLM-driven
// behavior is future work (recorded playback), see cases.md.
//
// Three assertion tiers (README §断言分三档):
//
//	Tier-0 同形: exit codes agree
//	Tier-1 语义: key tokens (subcommand / flag names) present in BOTH outputs
//	Tier-2 逐字: normalized outputs identical (stable, version-free text only)
package parity

import (
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Tier-0: same exit code shape
// ─────────────────────────────────────────────────────────────────────────────

func TestParityTier0(t *testing.T) {
	oracle, target := requireBoth(t)

	cases := []struct {
		name string
		args []string
	}{
		{"version-long", []string{"--version"}},
		{"version-short", []string{"-v"}},
		{"help-long", []string{"--help"}},
		{"help-short", []string{"-h"}},
		{"unknown-subcommand", []string{"definitely-not-a-command"}},
		// -p with no prompt must fail (usage error) on both, not hang.
		{"print-no-prompt", []string{"-p"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := runBinary(t, oracle, tc.args...)
			got := runBinary(t, target, tc.args...)

			if want.ExitCode != got.ExitCode {
				t.Errorf("exit code mismatch: oracle=%d target=%d\noracle stderr: %s\ntarget stderr: %s",
					want.ExitCode, got.ExitCode, want.Stderr, got.Stderr)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Tier-1: semantic tokens present in both outputs
// ─────────────────────────────────────────────────────────────────────────────

func TestParityTier1HelpTokens(t *testing.T) {
	oracle, target := requireBoth(t)

	// Tokens that must appear in `--help` output of ANY Claude Code build.
	// Keep this list to genuinely stable surface (documented CLI contract).
	tokens := []string{
		"-p", "--print", // headless mode flag
		"--model",         // model override
		"--output-format", // headless output format
		"--resume",        // session resume
		"--dangerously-skip-permissions",
	}

	want := runBinary(t, oracle, "--help")
	got := runBinary(t, target, "--help")

	for _, tok := range tokens {
		t.Run(tok, func(t *testing.T) {
			if !strings.Contains(want.Stdout, tok) {
				t.Errorf("oracle --help no longer contains %q — oracle contract changed, update cases.md", tok)
			}
			if !strings.Contains(got.Stdout, tok) {
				t.Errorf("target --help missing %q (oracle has it)", tok)
			}
		})
	}
}

func TestParityTier1SubcommandTree(t *testing.T) {
	oracle, target := requireBoth(t)

	// Top-level subcommands documented in the original CLI. Each must be
	// recognized (exit 0 for --help, NOT "unknown command") by both binaries.
	subcommands := []string{"mcp", "plugin", "auth"}

	for _, sub := range subcommands {
		t.Run(sub, func(t *testing.T) {
			want := runBinary(t, oracle, sub, "--help")
			got := runBinary(t, target, sub, "--help")

			// Both must exit 0 when showing help for a REAL subcommand.
			if want.ExitCode != 0 {
				t.Errorf("oracle %s --help exited %d — subcommand vanished from oracle?", sub, want.ExitCode)
			}
			if got.ExitCode != 0 {
				t.Errorf("target %s --help exited %d: %s", sub, got.ExitCode, got.Stderr)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Tier-2: exact (normalized) output match — only stable, version-free text
// ─────────────────────────────────────────────────────────────────────────────

func TestParityTier2VersionFormat(t *testing.T) {
	oracle, target := requireBoth(t)

	want := runBinary(t, oracle, "--version")
	got := runBinary(t, target, "--version")

	// Format "claude <semver>" with version number normalized to a
	// placeholder: the FORMAT must match, the number itself may differ.
	if n, g := normalizeOutput(want.Stdout), normalizeOutput(got.Stdout); n != g {
		t.Errorf("--version output format mismatch:\noracle: %q\ntarget: %q", n, g)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Offline check that does not need the oracle: the Go build must expose the
// subcommand tree it claims (guards against the headless fast-path in
// bootstrap.go accidentally hiding subcommands from --help).
// ─────────────────────────────────────────────────────────────────────────────

func TestTargetSubcommandsRegistered(t *testing.T) {
	target := requireTarget(t)

	got := runBinary(t, target, "--help")

	// These subcommands are registered in internal/bootstrap/bootstrap.go
	// (interactive mode only). --help runs without -p so all must appear.
	for _, sub := range []string{"auth", "mcp", "plugin", "doctor", "update", "agents"} {
		if !strings.Contains(got.Stdout, sub) {
			t.Errorf("target --help missing subcommand %q", sub)
		}
	}
}
