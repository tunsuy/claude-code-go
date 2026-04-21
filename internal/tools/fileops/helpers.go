// Package fileops implements file-operation tools: Read, Write, Edit, Glob, Grep, NotebookEdit.
package fileops

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// blockedDevicePaths is the list of device files that must never be read.
var blockedDevicePaths = map[string]bool{
	"/dev/zero":    true,
	"/dev/random":  true,
	"/dev/urandom": true,
	"/dev/full":    true,
	"/dev/stdin":   true,
	"/dev/tty":     true,
	"/dev/console": true,
	"/dev/stdout":  true,
	"/dev/stderr":  true,
	"/dev/fd/0":    true,
	"/dev/fd/1":    true,
	"/dev/fd/2":    true,
}

// imageExtensions lists file extensions treated as binary images.
var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".webp": true, ".svg": true, ".ico": true,
	".tiff": true, ".tif": true,
}

// expandPath converts a raw path from the model into an absolute OS path.
// It handles:
//   - "~" home-directory prefix
//   - Already-absolute paths (returned as-is after cleaning)
//   - Relative paths (resolved from the working directory)
//   - Windows-style backslashes converted to forward slashes on non-Windows
func expandPath(raw string) string {
	if raw == "" {
		return raw
	}
	// Normalise separators on non-Windows
	if runtime.GOOS != "windows" {
		raw = strings.ReplaceAll(raw, "\\", "/")
	}
	// Expand leading ~
	if strings.HasPrefix(raw, "~/") || raw == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			raw = filepath.Join(home, raw[1:])
		}
	}
	if !filepath.IsAbs(raw) {
		if wd, err := os.Getwd(); err == nil {
			raw = filepath.Join(wd, raw)
		}
	}
	return filepath.Clean(raw)
}

// isBlockedDevicePath returns true if the given absolute path is a blocked
// device file.
func isBlockedDevicePath(path string) bool {
	return blockedDevicePaths[path]
}

// isImageExtension returns true for image file extensions.
func isImageExtension(ext string) bool {
	return imageExtensions[strings.ToLower(ext)]
}
