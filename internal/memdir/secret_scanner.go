package memdir

import (
	"fmt"
	"regexp"
	"strings"
)

// secretRules holds regex patterns for common credential formats.
var secretRules = []*regexp.Regexp{
	regexp.MustCompile(`\b(AKIA|ASIA|ABIA|ACCA)[A-Z0-9]{16}\b`),
	regexp.MustCompile(`\bsk-ant-api[0-9A-Za-z_-]{20,}\b`),
	regexp.MustCompile(`\bghp_[0-9A-Za-z]{36}\b`),
	regexp.MustCompile(`\bgho_[0-9A-Za-z]{36}\b`),
	regexp.MustCompile(`-----BEGIN[A-Z ]*PRIVATE KEY-----`),
}

// ScanSecrets returns an error if content appears to contain secrets.
func ScanSecrets(content string) error {
	for _, rule := range secretRules {
		if rule.MatchString(content) {
			return fmt.Errorf("content appears to contain a secret (%s); memory write blocked", rule.String())
		}
	}
	return nil
}

// RedactSecrets replaces detected secret substrings with a placeholder.
func RedactSecrets(content string) string {
	out := content
	for _, rule := range secretRules {
		out = rule.ReplaceAllString(out, "[REDACTED]")
	}
	return strings.TrimSpace(out)
}
