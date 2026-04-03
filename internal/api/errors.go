// Package api provides the Anthropic API client implementation.
package api

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
)

// APIErrorKind classifies the type of API error.
type APIErrorKind string

const (
	ErrKindRateLimit         APIErrorKind = "rate_limit"
	ErrKindOverloaded        APIErrorKind = "overloaded"
	ErrKindUnauthorized      APIErrorKind = "unauthorized"
	ErrKindForbidden         APIErrorKind = "forbidden"
	ErrKindContextWindow     APIErrorKind = "context_window"
	ErrKindInvalidRequest    APIErrorKind = "invalid_request"
	ErrKindServerError       APIErrorKind = "server_error"
	ErrKindConnectionTimeout APIErrorKind = "connection_timeout"
	ErrKindConnectionError   APIErrorKind = "connection_error"
	ErrKindUnknown           APIErrorKind = "unknown"
)

// APIError wraps an HTTP-level error with status code and response headers.
type APIError struct {
	StatusCode int
	Message    string
	Headers    http.Header
	Kind       APIErrorKind
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

// APIErrorData is the JSON structure of an error returned in the SSE stream.
type APIErrorData struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// classifyError maps an HTTP status code to an APIErrorKind.
func classifyError(statusCode int, message string) APIErrorKind {
	switch statusCode {
	case 401:
		return ErrKindUnauthorized
	case 403:
		return ErrKindForbidden
	case 429:
		return ErrKindRateLimit
	case 529:
		return ErrKindOverloaded
	case 400:
		if contextOverflowRe.MatchString(message) {
			return ErrKindContextWindow
		}
		return ErrKindInvalidRequest
	default:
		if statusCode >= 500 {
			return ErrKindServerError
		}
		return ErrKindUnknown
	}
}

var contextOverflowRe = regexp.MustCompile(`(?i)prompt is too long|input length.*exceed.*context|context.*window.*exceeded`)
var contextOverflowDetailRe = regexp.MustCompile(`input length \((\d+) tokens\).*context limit \((\d+) tokens\)`)
var oauthRevokedRe = regexp.MustCompile(`(?i)revoked|token.*invalid|invalid.*token`)

// Is529Error reports whether err represents a 529 Overloaded error.
// Handles both *APIError and string-based checks (SDK serialisation quirk).
func Is529Error(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 529 || apiErr.Kind == ErrKindOverloaded
	}
	// Fallback: string check for cases where SDK serialises it differently
	return false
}

// IsOAuthTokenRevokedError reports whether err is a 403 with a token-revoked message.
func IsOAuthTokenRevokedError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 403 && oauthRevokedRe.MatchString(apiErr.Message)
	}
	return false
}

// ParseContextOverflowError extracts input token count and context limit from
// a context-overflow error. Returns (0, 0, false) when the error does not match.
func ParseContextOverflowError(err error) (inputTokens, contextLimit int, ok bool) {
	if err == nil {
		return 0, 0, false
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return 0, 0, false
	}
	if apiErr.Kind != ErrKindContextWindow {
		return 0, 0, false
	}
	m := contextOverflowDetailRe.FindStringSubmatch(apiErr.Message)
	if len(m) != 3 {
		return 0, 0, false
	}
	input, err1 := strconv.Atoi(m[1])
	limit, err2 := strconv.Atoi(m[2])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return input, limit, true
}

// RetryContext carries context about what was happening when a retry was abandoned.
type RetryContext struct {
	Attempt     int
	Model       string
	QuerySource string
}

// CannotRetryError is returned when retries are exhausted or not possible.
type CannotRetryError struct {
	Cause        error
	RetryContext RetryContext
}

func (e *CannotRetryError) Error() string {
	return fmt.Sprintf("cannot retry after %d attempts: %v", e.RetryContext.Attempt, e.Cause)
}

func (e *CannotRetryError) Unwrap() error { return e.Cause }

// FallbackTriggeredError is returned when 529 errors trigger a model fallback.
type FallbackTriggeredError struct {
	OriginalModel string
	FallbackModel string
}

func (e *FallbackTriggeredError) Error() string {
	return fmt.Sprintf("model fallback triggered: %s → %s", e.OriginalModel, e.FallbackModel)
}
