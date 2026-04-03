package fileops

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path atomically by writing to a temp file
// in the same directory and then renaming. This prevents partial writes from
// leaving the file in a corrupted state.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".write-*.tmp")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpName := tmp.Name()

	// Clean up on any error path.
	var writeErr error
	defer func() {
		if writeErr != nil {
			os.Remove(tmpName) //nolint:errcheck
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		writeErr = fmt.Errorf("cannot write to temp file: %w", err)
		return writeErr
	}
	if err := tmp.Close(); err != nil {
		writeErr = fmt.Errorf("cannot close temp file: %w", err)
		return writeErr
	}

	// Copy permissions from existing file if it exists.
	if info, err := os.Stat(path); err == nil {
		os.Chmod(tmpName, info.Mode()) //nolint:errcheck
	}

	if err := os.Rename(tmpName, path); err != nil {
		writeErr = fmt.Errorf("cannot rename temp file to %s: %w", path, err)
		return writeErr
	}
	return nil
}
