package fileops

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/anthropics/claude-code-go/internal/tools"
)

// ── Input / Output types ──────────────────────────────────────────────────────

// GrepInput is the validated input for the Grep tools.
type GrepInput struct {
	// Pattern is the regular expression to search for (required).
	Pattern string `json:"pattern"`
	// Path is the file or directory to search (optional, defaults to cwd).
	Path string `json:"path,omitempty"`
	// Include is a glob pattern to filter which files are searched (optional).
	// Example: "*.go", "**/*.ts"
	Include string `json:"include,omitempty"`
	// OutputMode selects the output format:
	//   "content"           – matching lines (default)
	//   "files_with_matches"– file paths only
	//   "count"             – match count per file
	OutputMode string `json:"output_mode,omitempty"`
	// MaxResults caps the number of results returned (default: unlimited).
	MaxResults int `json:"max_results,omitempty"`
}

// GrepMatch holds a single matching line.
type GrepMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// GrepOutput is the structured output returned by Grep.
type GrepOutput struct {
	Matches   []GrepMatch       `json:"matches,omitempty"`
	Files     []string          `json:"files,omitempty"`  // for files_with_matches mode
	Counts    map[string]int    `json:"counts,omitempty"` // for count mode
	NumResults int              `json:"num_results"`
	Truncated  bool             `json:"truncated,omitempty"`
}

const (
	maxGrepResults = 10_000
	outputModeContent          = "content"
	outputModeFilesWithMatches = "files_with_matches"
	outputModeCount            = "count"
)

// ── Tool implementation ───────────────────────────────────────────────────────

type grepTool struct{ tools.BaseTool }

// GrepTool is the exported singleton instance.
var GrepTool tools.Tool = &grepTool{}

func (t *grepTool) Name() string { return "Grep" }

func (t *grepTool) Description(_ tools.Input, _ tools.PermissionContext) string {
	return `A powerful search tool that searches file contents using regular expressions.
- Supports full regex syntax (e.g. "log.*Error", "function\\s+\\w+")
- Filter files with the include parameter (e.g. "*.go", "**/*.tsx")
- Output modes: "content" shows matching lines (default), "files_with_matches" shows file paths, "count" shows match counts
- Results are sorted by modification time (most recent first)
- Results are truncated at 10,000 matches`
}

func (t *grepTool) InputSchema() tools.InputSchema {
	return tools.NewInputSchema(
		map[string]json.RawMessage{
			"pattern": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "The regular expression pattern to search for in file contents",
			}),
			"path": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": "File or directory to search (defaults to current working directory)",
			}),
			"include": tools.PropSchema(map[string]any{
				"type":        "string",
				"description": `Glob pattern to filter which files are searched (e.g. "*.go", "**/*.ts")`,
			}),
			"output_mode": tools.PropSchema(map[string]any{
				"type":        "string",
				"enum":        []string{"content", "files_with_matches", "count"},
				"description": `Output mode: "content" (default), "files_with_matches", or "count"`,
			}),
			"max_results": tools.PropSchema(map[string]any{
				"type":        "integer",
				"description": "Maximum number of results to return",
			}),
		},
		[]string{"pattern"},
	)
}

func (t *grepTool) IsConcurrencySafe(_ tools.Input) bool { return true }
func (t *grepTool) IsReadOnly(_ tools.Input) bool         { return true }
func (t *grepTool) SearchHint() string                   { return "grep search content regex pattern find" }

func (t *grepTool) UserFacingName(input tools.Input) string {
	var in GrepInput
	if json.Unmarshal(input, &in) == nil && in.Pattern != "" {
		return fmt.Sprintf("Grep(%s)", in.Pattern)
	}
	return "Grep"
}

func (t *grepTool) ValidateInput(input tools.Input, _ *tools.UseContext) (tools.ValidationResult, error) {
	var in GrepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tools.ValidationResult{OK: false, Reason: "invalid JSON: " + err.Error()}, nil
	}
	if strings.TrimSpace(in.Pattern) == "" {
		return tools.ValidationResult{OK: false, Reason: "pattern is required and must be non-empty"}, nil
	}
	if _, err := regexp.Compile(in.Pattern); err != nil {
		return tools.ValidationResult{OK: false, Reason: "invalid regular expression: " + err.Error()}, nil
	}
	if in.OutputMode != "" && in.OutputMode != outputModeContent &&
		in.OutputMode != outputModeFilesWithMatches && in.OutputMode != outputModeCount {
		return tools.ValidationResult{
			OK:     false,
			Reason: fmt.Sprintf("output_mode must be one of %q, %q, %q", outputModeContent, outputModeFilesWithMatches, outputModeCount),
		}, nil
	}
	return tools.ValidationResult{OK: true}, nil
}

// Call executes the Grep tools.
func (t *grepTool) Call(input tools.Input, ctx *tools.UseContext, _ tools.OnProgressFn) (*tools.Result, error) {
	var in GrepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return &tools.Result{IsError: true, Content: "invalid input: " + err.Error()}, nil
	}

	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return &tools.Result{IsError: true, Content: "invalid regex: " + err.Error()}, nil
	}

	// Resolve search root.
	root := in.Path
	if root == "" {
		root, err = os.Getwd()
		if err != nil {
			return &tools.Result{IsError: true, Content: "could not determine working directory: " + err.Error()}, nil
		}
	} else {
		root = expandPath(root)
	}

	// Determine effective output mode.
	mode := in.OutputMode
	if mode == "" {
		mode = outputModeContent
	}

	limit := in.MaxResults
	if limit <= 0 || limit > maxGrepResults {
		limit = maxGrepResults
	}

	// Collect and sort candidate files by modification time.
	type fileInfo struct {
		path    string
		modTime int64
	}

	info, statErr := os.Stat(root)
	if statErr != nil {
		return &tools.Result{IsError: true, Content: fmt.Sprintf("cannot access path %s: %v", root, statErr)}, nil
	}

	var candidates []fileInfo
	if info.Mode().IsRegular() {
		candidates = []fileInfo{{path: root, modTime: info.ModTime().UnixNano()}}
	} else if info.IsDir() {
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error { //nolint:errcheck
			if err != nil {
				return nil
			}
			if ctx != nil {
				select {
				case <-ctx.Ctx.Done():
					return ctx.Ctx.Err()
				default:
				}
			}
			if d.IsDir() {
				if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if in.Include != "" {
				rel, _ := filepath.Rel(root, path)
				matched, _ := matchGlobPattern(in.Include, filepath.ToSlash(rel))
				if !matched {
					return nil
				}
			}
			fi, err := d.Info()
			if err != nil {
				return nil
			}
			candidates = append(candidates, fileInfo{path: path, modTime: fi.ModTime().UnixNano()})
			return nil
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime > candidates[j].modTime
	})

	// Search each candidate file.
	out := GrepOutput{
		Counts: make(map[string]int),
	}
	total := 0
	truncated := false

	for _, c := range candidates {
		if ctx != nil {
			select {
			case <-ctx.Ctx.Done():
				truncated = true
				goto done
			default:
			}
		}
		fileMatches, err := grepFile(c.path, re)
		if err != nil || len(fileMatches) == 0 {
			continue
		}

		switch mode {
		case outputModeFilesWithMatches:
			if total < limit {
				out.Files = append(out.Files, c.path)
				total++
			} else {
				truncated = true
			}

		case outputModeCount:
			out.Counts[c.path] = len(fileMatches)
			total += len(fileMatches)

		default: // content
			for _, m := range fileMatches {
				if total >= limit {
					truncated = true
					goto done
				}
				out.Matches = append(out.Matches, m)
				total++
			}
		}
	}

done:
	out.NumResults = total
	out.Truncated = truncated

	// Clean up empty maps/slices.
	if len(out.Counts) == 0 {
		out.Counts = nil
	}

	return &tools.Result{Content: out}, nil
}

// grepFile returns all matching lines in a file.
func grepFile(path string, re *regexp.Regexp) ([]GrepMatch, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var matches []GrepMatch
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1 MiB line buffer
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, GrepMatch{
				Path:    path,
				Line:    lineNum,
				Content: line,
			})
		}
	}
	return matches, scanner.Err()
}

func (t *grepTool) MapResultToToolResultBlock(output any, toolUseID string) (json.RawMessage, error) {
	out, ok := output.(GrepOutput)
	if !ok {
		return t.BaseTool.MapResultToToolResultBlock(output, toolUseID)
	}

	var sb strings.Builder
	switch {
	case out.Files != nil:
		for _, f := range out.Files {
			sb.WriteString(f)
			sb.WriteByte('\n')
		}
	case out.Counts != nil:
		for path, count := range out.Counts {
			sb.WriteString(fmt.Sprintf("%s: %d\n", path, count))
		}
	default:
		for _, m := range out.Matches {
			sb.WriteString(fmt.Sprintf("%s:%d:%s\n", m.Path, m.Line, m.Content))
		}
	}

	if out.NumResults == 0 {
		sb.WriteString("No matches found.")
	} else if out.Truncated {
		sb.WriteString(fmt.Sprintf("\n[Results truncated at %d matches]", maxGrepResults))
	}

	block := map[string]any{
		"type":        "tool_result",
		"tool_use_id": toolUseID,
		"content":     sb.String(),
	}
	return json.Marshal(block)
}
