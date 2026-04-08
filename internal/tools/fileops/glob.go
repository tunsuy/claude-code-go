package fileops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anthropics/claude-code-go/internal/tools"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// GlobInput is the validated input for the Glob tools.
type GlobInput struct {
	// Pattern is the glob expression (required). Examples: "**/*.go", "src/**/*.ts"
	Pattern string `json:"pattern"`
	// Path is the directory to search in (optional, defaults to cwd).
	Path string `json:"path,omitempty"`
}

// GlobOutput is the structured output returned by Glob.
type GlobOutput struct {
	// Filenames contains the matched paths sorted by modification time
	// (most recently modified first).
	Filenames []string `json:"filenames"`
	// NumFiles is len(Filenames), provided for convenience.
	NumFiles int `json:"num_files"`
	// Truncated is true when the result list was cut at maxGlobResults.
	Truncated bool `json:"truncated,omitempty"`
}

// maxGlobResults is the upper bound on matched paths we'll return.
const maxGlobResults = 10_000

// ── Tool implementation ───────────────────────────────────────────────────────

type globTool struct{ tools.BaseTool }

// GlobTool is the exported singleton instance.
var GlobTool tools.Tool = &globTool{}

func (t *globTool) Name() string { return "Glob" }

func (t *globTool) Aliases() []string { return nil }

func (t *globTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `Fast file pattern matching tool that works with any codebase size.
- Supports glob patterns like "**/*.js" or "src/**/*.ts"
- Returns matching file paths sorted by modification time (most recently modified first)
- Use this tool when you need to find files by name patterns
- Results are truncated at 10,000 files`
}

func (t *globTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"pattern": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": `The glob pattern to match files against (e.g. "**/*.go", "src/**/*.ts")`,
			}),
			"path": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The directory to search in. If not specified the current working directory is used.",
			}),
		},
		[]string{"pattern"},
	)
}

func (t *globTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *globTool) IsReadOnly(_ tools.Input) bool         { return true }
func (t *globTool) SearchHint() string                   { return "glob pattern file search find" }

func (t *globTool) UserFacingName(input tools.Input) string {
	var in GlobInput
	if json.Unmarshal(input, &in) == nil && in.Pattern != "" {
		return fmt.Sprintf("Glob(%s)", in.Pattern)
	}
	return "Glob"
}

func (t *globTool) ValidateInput(input tools.Input, _ *tools.UseContext) (tools.ValidationResult, error) {
	var in GlobInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.ValidationResult{OK: false, Reason: "invalid JSON: " + err.Error()}, nil
	}
	if strings.TrimSpace(in.Pattern) == "" {
		return tools.ValidationResult{OK: false, Reason: "pattern is required and must be non-empty"}, nil
	}
	return tools.ValidationResult{OK: true}, nil
}

// Call executes the Glob tools.
func (t *globTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in GlobInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	// Resolve base directory.
	baseDir := in.Path
	if baseDir == "" {
		var err error
		baseDir, err = os.Getwd()
		if err != nil {
			return &tools.Result{IsError: true, Content: "could not determine working directory: " + err.Error()}, nil
		}
	} else {
		baseDir = expandPath(baseDir)
	}

	// Verify the base directory exists.
	if info, err := os.Stat(baseDir); err != nil || !info.IsDir() {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("path does not exist or is not a directory: %s", baseDir)}, nil
	}

	// Execute the glob walk.
	type fileEntry struct {
		path    string
		modTime int64 // UnixNano
	}
	var entries []fileEntry
	truncated := false

	err := filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable entries silently.
			return nil
		}
		// Check cancellation.
		if ctx != nil {
			select {
			case <-ctx.Ctx.Done():
				return ctx.Ctx.Err()
			default:
			}
		}
		if d.IsDir() {
			// Skip hidden directories (e.g. .git) to keep results clean.
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Compute the path relative to baseDir for pattern matching.
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return nil
		}

		matched, err := matchGlobPattern(in.Pattern, rel)
		if err != nil || !matched {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if len(entries) >= maxGlobResults {
			truncated = true
			return filepath.SkipAll
		}
		entries = append(entries, fileEntry{path: path, modTime: info.ModTime().UnixNano()})
		return nil
	})
	if err != nil && ctx != nil && ctx.Ctx.Err() != nil {
		return &tools.Result{IsError: true, Content: "glob cancelled"}, nil
	}

	// Sort by modification time descending (most recent first).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime > entries[j].modTime
	})

	filenames := make([]string, len(entries))
	for i, e := range entries {
		filenames[i] = e.path
	}

	out := GlobOutput{
		Filenames: filenames,
		NumFiles:  len(filenames),
		Truncated: truncated,
	}
	return &tools.Result{Content: out}, nil
}

func (t *globTool) MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error) {
	out, ok := output.(GlobOutput)
	if !ok {
		return t.BaseTool.MapResultToToolResultBlock(output, toolUseID)
	}
	var sb strings.Builder
	if out.NumFiles == 0 {
		sb.WriteString("No files found matching the pattern.")
	} else {
		for _, f := range out.Filenames {
			sb.WriteString(f)
			sb.WriteByte('\n')
		}
		if out.Truncated {
			sb.WriteString(fmt.Sprintf("\n[Results truncated at %d files]", maxGlobResults))
		}
	}
	block := map[string]any{
		"type":        "tool_result",
		"tool_use_id": toolUseID,
		"content":     sb.String(),
	}
	return json.Marshal(block)
}

// matchGlobPattern matches a relative file path against a glob pattern.
// It normalises path separators and supports ** double-star matching.
func matchGlobPattern(pattern, rel string) (bool, error) {
	// Normalise separators.
	rel = filepath.ToSlash(rel)
	pattern = filepath.ToSlash(pattern)

	// filepath.Match does not support **, so we implement it ourselves.
	return doubleStarMatch(pattern, rel)
}

// doubleStarMatch implements ** glob semantics similar to zsh/bash.
// A ** segment matches zero or more path components.
func doubleStarMatch(pattern, s string) (bool, error) {
	// Split on ** segments.
	parts := strings.SplitN(pattern, "**", 2)
	if len(parts) == 1 {
		// No **, fall back to filepath.Match.
		return filepath.Match(pattern, s)
	}

	prefix := parts[0]
	suffix := parts[1]

	// Remove trailing/leading slashes from prefix/suffix.
	prefix = strings.TrimRight(prefix, "/")
	suffix = strings.TrimLeft(suffix, "/")

	// The prefix must match the beginning of s.
	if prefix != "" {
		if !strings.HasPrefix(s, prefix+"/") && s != prefix {
			return false, nil
		}
		if s == prefix {
			s = ""
		} else {
			s = s[len(prefix)+1:]
		}
	}

	// If no suffix, ** matches everything.
	if suffix == "" {
		return true, nil
	}

	// suffix may itself contain **, recurse.
	// Try matching suffix against every suffix of s.
	for {
		matched, err := doubleStarMatch(suffix, s)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
		idx := strings.Index(s, "/")
		if idx < 0 {
			break
		}
		s = s[idx+1:]
	}
	return false, nil
}
