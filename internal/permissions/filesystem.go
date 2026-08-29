package permissions

import (
	"os"
	"path/filepath"
	"strings"
)

// DANGEROUS_FILES lists files whose modification would compromise the user's
// environment or the agent harness itself (shell init files, git config, MCP
// config, harness config). Writes/edits to these files are denied.
var DANGEROUS_FILES = map[string]struct{}{
	".gitconfig":    {},
	".gitmodules":   {},
	".bashrc":       {},
	".bash_profile": {},
	".zshrc":        {},
	".zprofile":     {},
	".profile":      {},
	".ripgreprc":    {},
	".mcp.json":     {},
	".claude.json":  {},
}

// DANGEROUS_DIRECTORIES lists directories whose contents must not be modified
// by the agent: VCS internals, editor configs, and the harness config dir.
var DANGEROUS_DIRECTORIES = map[string]struct{}{
	".git":    {},
	".vscode": {},
	".idea":   {},
	".claude": {},
}

// DangerousPathResult describes why a path is considered dangerous.
type DangerousPathResult struct {
	// Dangerous is true when the path targets a protected file or directory.
	Dangerous bool
	// Matched is the file base name or directory segment that triggered the match.
	Matched string
	// Reason is a human-readable explanation for the denial.
	Reason string
}

// IsDangerousPath reports whether path targets a protected file or directory.
// Both the final path segment (file name) and every intermediate segment
// (directory names) are checked, so writes inside .git/ or .claude/ are caught
// regardless of depth. The comparison is case-insensitive to handle
// case-insensitive filesystems (macOS, Windows).
func IsDangerousPath(path string) bool {
	return CheckDangerousPath(path).Dangerous
}

// CheckDangerousPath returns a detailed result for path, including the matched
// protected name and a denial reason when dangerous. A "~" prefix is expanded
// to the user's home directory before evaluation.
func CheckDangerousPath(path string) DangerousPathResult {
	expanded, err := expandHome(path)
	if err == nil {
		path = expanded
	}
	cleaned := filepath.ToSlash(filepath.Clean(path))

	// Every path segment is a potential dangerous directory; the final segment
	// may additionally be a dangerous file name.
	segments := strings.Split(cleaned, "/")
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		lower := strings.ToLower(seg)
		if _, ok := DANGEROUS_DIRECTORIES[lower]; ok {
			kind := "directory"
			if i == len(segments)-1 {
				kind = "directory"
			}
			return DangerousPathResult{
				Dangerous: true,
				Matched:   seg,
				Reason: "path is inside the protected " + kind + " " + quote(seg) +
					"; modifications to it are not allowed for safety",
			}
		}
		if i == len(segments)-1 {
			if _, ok := DANGEROUS_FILES[lower]; ok {
				return DangerousPathResult{
					Dangerous: true,
					Matched:   seg,
					Reason: quote(seg) + " is a protected configuration file; " +
						"modifying it is not allowed for safety",
				}
			}
		}
	}
	return DangerousPathResult{}
}

// expandHome replaces a leading "~" with the user's home directory.
// Returns the input unchanged when the home directory cannot be determined.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, path[2:]), nil
}

func quote(s string) string {
	return `"` + s + `"`
}
