// Package fs provides filesystem utilities for claude-code-go.
package fs

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
)

// EnsureDir ensures the directory exists (mkdir -p semantics).
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// SafeReadFile reads a file and returns its contents.
// Returns (nil, nil) if the file does not exist, and (nil, err) for other errors.
func SafeReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if isNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

// AtomicWriteFile writes data to path atomically using a temp-file + rename strategy.
// On POSIX systems this guarantees that readers never see a partial write.
// The temp file is sync'd before rename for durability on power loss.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	// Sync before rename for durability (N-11 from review).
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err = tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err = os.Chmod(tmpName, perm); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// ProjectHash computes the canonical 8-character hex hash used to derive the
// session directory path from a project root.
//
// Algorithm: sha256(abs(projectRoot))[0:4] encoded as lowercase hex.
// This algorithm MUST be identical in every component that constructs or
// resolves session paths (session store, bootstrap, etc.).
func ProjectHash(projectRoot string) (string, error) {
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(h[:4]), nil // 8 hex chars
}

// isNotExist wraps errors.Is with fs.ErrNotExist for Go 1.13+ compatibility.
func isNotExist(err error) bool {
	return err != nil && (os.IsNotExist(err) || isErrNotExist(err))
}

func isErrNotExist(err error) bool {
	unwrapped := err
	for unwrapped != nil {
		if unwrapped == fs.ErrNotExist {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := unwrapped.(unwrapper)
		if !ok {
			break
		}
		unwrapped = u.Unwrap()
	}
	return false
}
