//go:build darwin

package oauth

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// KeychainStore uses the macOS system keychain via the `security` CLI tool.
// This avoids CGo and external library dependencies.
// Service: "claude-code", Account: "oauth-tokens"
type KeychainStore struct {
	serviceName string
	accountName string
}

func (k *KeychainStore) Load() (*OAuthTokens, error) {
	cmd := exec.Command("security", "find-generic-password",
		"-s", k.serviceName,
		"-a", k.accountName,
		"-w",
	)
	out, err := cmd.Output()
	if err != nil {
		// Exit code 44 means "item not found" — normal case, return (nil, nil).
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 44 {
			return nil, nil
		}
		// All other errors (keychain locked, permission denied, etc.) must be propagated
		// so callers can distinguish "not found" from an actual keychain error (P1-2).
		return nil, fmt.Errorf("keychain: find-generic-password: %w", err)
	}
	data := strings.TrimSpace(string(out))
	if data == "" {
		return nil, nil
	}
	var tokens OAuthTokens
	if err := json.Unmarshal([]byte(data), &tokens); err != nil {
		return nil, fmt.Errorf("keychain: unmarshal: %w", err)
	}
	return &tokens, nil
}

func (k *KeychainStore) Save(tokens *OAuthTokens) error {
	data, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("keychain: marshal: %w", err)
	}

	// Delete first to avoid "duplicate item" error
	_ = k.Delete()

	// Pass token data via stdin instead of as a command-line argument to
	// prevent credential exposure through process argument lists (ps aux).
	// `security add-generic-password -w` without a value reads the password
	// from stdin.
	cmd := exec.Command("security", "add-generic-password",
		"-s", k.serviceName,
		"-a", k.accountName,
		"-U",
		"-w",
	)
	cmd.Stdin = strings.NewReader(string(data))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("keychain: add-generic-password: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (k *KeychainStore) Delete() error {
	cmd := exec.Command("security", "delete-generic-password",
		"-s", k.serviceName,
		"-a", k.accountName,
	)
	_ = cmd.Run() // ignore error (not found is OK)
	return nil
}
