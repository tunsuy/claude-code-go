package memdir

import (
	"fmt"
	"strings"
	"time"
)

// MemoryType classifies the kind of memory stored.
type MemoryType string

const (
	// MemoryTypeUser represents user preference memories (coding style, tool choices).
	MemoryTypeUser MemoryType = "user"
	// MemoryTypeFeedback represents feedback memories (corrections, improvements).
	MemoryTypeFeedback MemoryType = "feedback"
	// MemoryTypeProject represents project-specific memories (architecture, conventions).
	MemoryTypeProject MemoryType = "project"
	// MemoryTypeReference represents external reference memories (docs, URLs).
	MemoryTypeReference MemoryType = "reference"
)

// ValidMemoryTypes is the set of all valid MemoryType values.
var ValidMemoryTypes = map[MemoryType]bool{
	MemoryTypeUser:      true,
	MemoryTypeFeedback:  true,
	MemoryTypeProject:   true,
	MemoryTypeReference: true,
}

// MemoryHeader is the YAML frontmatter metadata for a memory file.
type MemoryHeader struct {
	// Title is the human-readable title of this memory.
	Title string `json:"title"`
	// Type classifies the memory.
	Type MemoryType `json:"type"`
	// CreatedAt is when the memory was first created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the memory was last modified.
	UpdatedAt time.Time `json:"updated_at"`
	// Tags are optional labels for categorisation and search.
	Tags []string `json:"tags,omitempty"`
	// Source describes where the memory originated (e.g. "auto-extract", "user", "dream").
	Source string `json:"source,omitempty"`
}

// MemoryFile represents a complete memory file (header + body).
type MemoryFile struct {
	// Header is the parsed frontmatter.
	Header MemoryHeader
	// Body is the Markdown content after the frontmatter.
	Body string
	// Path is the absolute file system path.
	Path string
}

// frontmatterSep is the YAML frontmatter delimiter.
const frontmatterSep = "---"

// ParseMemoryFile parses a memory file's raw content into a MemoryFile.
// The expected format is:
//
//	---
//	title: ...
//	type: ...
//	created_at: ...
//	updated_at: ...
//	tags: [...]
//	source: ...
//	---
//	<markdown body>
func ParseMemoryFile(content, filePath string) (*MemoryFile, error) {
	mf := &MemoryFile{Path: filePath}

	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, frontmatterSep) {
		// No frontmatter — treat entire content as body with defaults.
		mf.Body = content
		mf.Header = MemoryHeader{
			Type:      MemoryTypeProject,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		return mf, nil
	}

	// Find the closing "---".
	rest := content[len(frontmatterSep):]
	rest = strings.TrimLeft(rest, "\r\n")
	idx := strings.Index(rest, frontmatterSep)
	if idx < 0 {
		return nil, fmt.Errorf("memdir: unclosed frontmatter in %s", filePath)
	}

	fmRaw := rest[:idx]
	body := rest[idx+len(frontmatterSep):]
	body = strings.TrimLeft(body, "\r\n")

	header, err := parseFrontmatter(fmRaw)
	if err != nil {
		return nil, fmt.Errorf("memdir: parse frontmatter in %s: %w", filePath, err)
	}

	mf.Header = header
	mf.Body = strings.TrimSpace(body)
	return mf, nil
}

// parseFrontmatter parses a simple YAML-like frontmatter block.
// We use a simple line-based parser to avoid pulling in a full YAML library.
func parseFrontmatter(raw string) (MemoryHeader, error) {
	h := MemoryHeader{
		Type:      MemoryTypeProject,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		val := strings.TrimSpace(line[colonIdx+1:])

		switch key {
		case "title":
			h.Title = trimQuotes(val)
		case "type":
			mt := MemoryType(trimQuotes(val))
			if ValidMemoryTypes[mt] {
				h.Type = mt
			}
		case "created_at":
			if t, err := time.Parse(time.RFC3339, trimQuotes(val)); err == nil {
				h.CreatedAt = t
			}
		case "updated_at":
			if t, err := time.Parse(time.RFC3339, trimQuotes(val)); err == nil {
				h.UpdatedAt = t
			}
		case "tags":
			h.Tags = parseTags(val)
		case "source":
			h.Source = trimQuotes(val)
		}
	}
	return h, nil
}

// FormatFrontmatter serialises a MemoryHeader into YAML frontmatter.
func FormatFrontmatter(h MemoryHeader) string {
	var sb strings.Builder
	sb.WriteString(frontmatterSep)
	sb.WriteString("\n")
	if h.Title != "" {
		sb.WriteString(fmt.Sprintf("title: %s\n", h.Title))
	}
	sb.WriteString(fmt.Sprintf("type: %s\n", h.Type))
	sb.WriteString(fmt.Sprintf("created_at: %s\n", h.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("updated_at: %s\n", h.UpdatedAt.Format(time.RFC3339)))
	if len(h.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("tags: [%s]\n", strings.Join(h.Tags, ", ")))
	}
	if h.Source != "" {
		sb.WriteString(fmt.Sprintf("source: %s\n", h.Source))
	}
	sb.WriteString(frontmatterSep)
	sb.WriteString("\n")
	return sb.String()
}

// FormatMemoryFile serialises a MemoryFile into its full file content.
func FormatMemoryFile(mf *MemoryFile) string {
	return FormatFrontmatter(mf.Header) + "\n" + mf.Body + "\n"
}

// trimQuotes removes surrounding single or double quotes from a string.
func trimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// parseTags parses a YAML-like inline list, e.g. "[go, testing, architecture]".
func parseTags(val string) []string {
	val = strings.TrimSpace(val)
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = trimQuotes(p)
		if p != "" {
			tags = append(tags, p)
		}
	}
	return tags
}
