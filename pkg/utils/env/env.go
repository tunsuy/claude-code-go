// Package env provides environment variable helpers.
package env

import (
	"os"
	"strings"
)

// GetEnv returns the value of the environment variable named by key,
// or defaultValue if the variable is unset or empty.
func GetEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// IsEnvTruthy reports whether the environment variable named key has a truthy
// value ("1", "true", "yes", "on" — case-insensitive).
func IsEnvTruthy(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
