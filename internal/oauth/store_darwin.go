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
		// Not found is a normal case
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 44 {
			return nil, nil
		}
		// Other errors (including "The specified item could not be found")
		return nil, nil
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

	cmd := exec.Command("security", "add-generic-password",
		"-s", k.serviceName,
		"-a", k.accountName,
		"-w", string(data),
	)
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
	cmd.Run() // ignore error (not found is OK)
	return nil
}
