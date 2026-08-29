package fileops

import (
	"encoding/json"
	"fmt"
	"strings"
)

// dangerousNames lists file base names whose modification would compromise the
// user's environment or the agent harness itself (shell init files, git
// config, MCP config, harness config).
var dangerousNames = map[string]struct{}{
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

// dangerousDirNames lists directory names whose contents must not be modified
// by the agent: VCS internals, editor configs, and the harness config dir.
var dangerousDirNames = map[string]struct{}{
	".git":    {},
	".vscode": {},
	".idea":   {},
	".claude": {},
}

// checkDangerousPath reports whether path targets a protected file or a path
// inside a protected directory. Both the file base name and every parent
// directory segment are checked, so writes inside .git/ or .claude/ are caught
// regardless of depth. The comparison is case-insensitive to handle
// case-insensitive filesystems (macOS, Windows).
//
// Returns the matched protected name and a denial reason when dangerous.
func checkDangerousPath(path string) (matched string, reason string, dangerous bool) {
	cleaned := strings.TrimPrefix(strings.TrimSpace(path), "~")
	segments := strings.Split(strings.Trim(cleaned, "/"), "/")
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		lower := strings.ToLower(seg)
		if _, ok := dangerousDirNames[lower]; ok {
			return seg, fmt.Sprintf("path is inside the protected directory %q; modifications are not allowed for safety", seg), true
		}
		if i == len(segments)-1 {
			if _, ok := dangerousNames[lower]; ok {
				return seg, fmt.Sprintf("%q is a protected configuration file; modifying it is not allowed for safety", seg), true
			}
		}
	}
	return "", "", false
}

// writeTargetPath extracts the file path targeted by a write-type tool input.
// Supported fields: file_path (Write/Edit), notebook_path (NotebookEdit).
// Returns "" when the input carries no path.
func writeTargetPath(input []byte) string {
	var in struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return ""
	}
	if in.FilePath != "" {
		return in.FilePath
	}
	return in.NotebookPath
}
