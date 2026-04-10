// Package bootstrap initializes and wires up all application components,
// then hands control to the cobra root command.
//
// Initialization order (mirrors TS src/entrypoints/init.ts):
//
//	Phase 0  Fast-path detection (--version, etc.)
//	Phase 1  Config system init (settings.json merge, env-var overrides)
//	Phase 2  Runtime safety / network (graceful-shutdown, proxy)
//	Phase 3  Auth pre-warm (OAuth token check/refresh)
//	Phase 4  Feature/policy loading
//	Phase 5  Data migrations
//	Phase 6  Service layer init (deferred until first REPL render)
package bootstrap

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// appVersion is the canonical binary version string.
// Overridden at link-time via -ldflags "-X bootstrap.appVersion=x.y.z".
var appVersion = "0.1.0"

// HandleFastPath inspects raw os.Args and handles zero-dependency flags
// (--version, -v) before cobra is initialised.
// Returns true if the process should exit immediately (clean exit).
func HandleFastPath(args []string) bool {
	for _, arg := range args[1:] {
		switch arg {
		case "--version", "-v":
			fmt.Printf("claude %s\n", appVersion)
			return true
		case "--":
			// End of flags — stop scanning.
			return false
		}
	}
	return false
}

// Run is the main entry point called from cmd/claude/main.go.
// It builds the cobra command tree and executes it.
func Run(args []string) error {
	// Determine if we are in headless / non-interactive mode early so we can
	// skip registering the heavyweight interactive sub-commands (performance
	// optimisation mirroring the TS ~65 ms saving for -p mode).
	headless := isPrintMode(args)

	// P1-A: rootCmd is a local variable — no concurrent access issue.
	// Each call to Run() creates its own cobra.Command tree; there is no
	// package-level rootCmd variable that goroutines could race on.
	rootCmd := buildRootCmd(headless)

	// Pass the raw args through (cobra uses os.Args by default but we set
	// them explicitly so tests can override easily).
	rootCmd.SetArgs(args[1:])
	return rootCmd.Execute()
}

// Execute is kept for backward-compatibility (called by older stubs).
func Execute() error {
	return Run(os.Args)
}

// isPrintMode scans raw args for -p / --print before cobra parsing.
func isPrintMode(args []string) bool {
	for _, a := range args[1:] {
		if a == "-p" || a == "--print" {
			return true
		}
		if a == "--" {
			break
		}
	}
	return false
}

// buildRootCmd constructs the full cobra command tree.
// When headless is true, the interactive sub-commands are omitted to save
// startup time.
func buildRootCmd(headless bool) *cobra.Command {
	root := newRootCmd()

	if !headless {
		// Register the full interactive sub-command tree.
		root.AddCommand(
			newAuthCmd(),
			newMCPCmd(),
			newPluginCmd(),
			newDoctorCmd(),
			newUpdateCmd(),
			newAgentsCmd(),
			newInstallCmd(),
		)
	}

	return root
}

// longDesc returns a trimmed multi-line long description, removing leading
// whitespace so the source code can be indented for readability.
func longDesc(s string) string {
	return strings.TrimSpace(s)
}
