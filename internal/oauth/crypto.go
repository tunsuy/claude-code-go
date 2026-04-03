package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// GenerateCodeVerifier generates a 32-byte random base64url-encoded code_verifier.
// Corresponds to TS generateCodeVerifier().
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateCodeChallenge computes the S256 code_challenge from a code_verifier.
// SHA-256 hash → base64url encoding (no padding), per RFC 7636.
// Corresponds to TS generateCodeChallenge(verifier).
func GenerateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// GenerateState generates a 32-byte random base64url-encoded state parameter.
// Used for CSRF protection. Corresponds to TS generateState().
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
