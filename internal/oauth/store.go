package oauth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// TokenStore is the interface for persisting OAuth tokens.
// Supports macOS Keychain, encrypted file, and in-memory implementations.
type TokenStore interface {
	// Load loads tokens; returns (nil, nil) if none exist.
	Load() (*OAuthTokens, error)
	// Save persists tokens.
	Save(tokens *OAuthTokens) error
	// Delete removes tokens (logout).
	Delete() error
}

// NewTokenStore returns the platform-appropriate TokenStore:
//   - darwin → KeychainStore
//   - other  → FileStore (AES-256-GCM encrypted)
func NewTokenStore() TokenStore {
	if runtime.GOOS == "darwin" {
		return &KeychainStore{
			serviceName: "claude-code",
			accountName: "oauth-tokens",
		}
	}
	path := defaultTokenFilePath()
	return &FileStore{path: path}
}

// defaultTokenFilePath returns $XDG_CONFIG_HOME/claude-code/tokens.enc
// (falling back to $HOME/.config/claude-code/tokens.enc).
func defaultTokenFilePath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "claude-code", "tokens.enc")
}

// ─── MemoryStore ──────────────────────────────────────────────────────────────

// MemoryStore is a thread-safe in-memory token store for testing.
type MemoryStore struct {
	mu     sync.Mutex
	tokens *OAuthTokens
}

func (m *MemoryStore) Load() (*OAuthTokens, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tokens == nil {
		return nil, nil
	}
	t := *m.tokens
	return &t, nil
}

func (m *MemoryStore) Save(tokens *OAuthTokens) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := *tokens
	m.tokens = &t
	return nil
}

func (m *MemoryStore) Delete() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens = nil
	return nil
}

// ─── FileStore (AES-256-GCM) ─────────────────────────────────────────────────

// FileStore encrypts tokens with AES-256-GCM and stores them in a file.
//
// Encryption details (non-macOS fallback):
//   - Algorithm:    AES-256-GCM (AEAD)
//   - Key derivation: PBKDF2-SHA256 with a machine-specific salt derived from
//     hostname + OS-specific ID (best-effort). 100,000 iterations, 32-byte key.
//   - Format:       [4-byte version][12-byte nonce][ciphertext+tag]
//   - Version:      0x00000001 (v1, reserved for algorithm migration)
//
// Key storage: the derived key is not stored; it is re-derived on each access.
// Machine migration (e.g. copying tokens.enc to a new machine) will fail to
// decrypt unless the machine ID is identical. This is intentional.
type FileStore struct {
	path string
}

func (f *FileStore) Load() (*OAuthTokens, error) {
	ciphertext, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("filestore: read: %w", err)
	}
	plaintext, err := decryptTokenFile(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("filestore: decrypt: %w", err)
	}
	var tokens OAuthTokens
	if err := json.Unmarshal(plaintext, &tokens); err != nil {
		return nil, fmt.Errorf("filestore: unmarshal: %w", err)
	}
	return &tokens, nil
}

func (f *FileStore) Save(tokens *OAuthTokens) error {
	plaintext, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("filestore: marshal: %w", err)
	}
	ciphertext, err := encryptTokenFile(plaintext)
	if err != nil {
		return fmt.Errorf("filestore: encrypt: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return fmt.Errorf("filestore: mkdir: %w", err)
	}
	if err := os.WriteFile(f.path, ciphertext, 0o600); err != nil {
		return fmt.Errorf("filestore: write: %w", err)
	}
	return nil
}

func (f *FileStore) Delete() error {
	err := os.Remove(f.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
