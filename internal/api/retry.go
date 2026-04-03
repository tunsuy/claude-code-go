package api

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// Retry constants, faithfully ported from TS withRetry.ts.
const (
	DefaultMaxRetries    = 10
	BaseDelayMS          = 500
	MaxDelayMS           = 32_000
	Max529Retries        = 3
	PersistentMaxBackoff = 5 * 60 * 1000  // ms: 5 minutes (UNATTENDED_RETRY mode)
	PersistentResetCap   = 6 * 60 * 60 * 1000 // ms: 6 hours

	// HeartbeatInterval is consumed by the UNATTENDED_RETRY retry monitor
	// to emit periodic "still alive" log entries during long backoff waits.
	HeartbeatInterval = 30 * time.Second
)

// foreground529RetrySources mirrors TS FOREGROUND_529_RETRY_SOURCES.
var foreground529RetrySources = map[string]bool{
	"repl_main_thread": true,
	"sdk":              true,
	"agent:default":    true,
	"agent:custom":     true,
	"compact":          true,
}

// RetryableFunc is an API operation that can be retried.
type RetryableFunc[T any] func(ctx context.Context, attempt int) (T, error)

// RetryOptions controls retry behaviour.
type RetryOptions struct {
	MaxRetries            int
	Model                 string
	FallbackModel         string
	QuerySource           string
	InitialConsecutive529 int
	Signal                context.Context
}

// WithRetry executes fn with exponential backoff retry logic.
//
// Retry rules (matching TS behaviour):
//   - 401 / OAuth revoked → triggers token refresh callback, then retry
//   - 429 / 529 foreground → exponential backoff with retry-after header
//   - 529 background → abandon immediately (avoid capacity cascade)
//   - 3 consecutive 529 + Opus model → return FallbackTriggeredError
//   - 400 context overflow → propagate immediately (caller adjusts max_tokens)
//   - 5xx / 408 / 409 / connection reset → retry
//   - x-should-retry: false → respect (subscription user)
func WithRetry[T any](
	ctx context.Context,
	fn RetryableFunc[T],
	opts RetryOptions,
) (T, error) {
	var zero T

	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}

	consecutive529 := opts.InitialConsecutive529
	isForeground := foreground529RetrySources[opts.QuerySource]

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Check context before each attempt
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		result, err := fn(ctx, attempt)
		if err == nil {
			return result, nil
		}

		// Determine if we should retry
		var apiErr *APIError
		isAPIErr := errors.As(err, &apiErr)

		// Never retry on context cancellation
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return zero, err
		}

		// 400 context overflow → propagate immediately
		if isAPIErr && apiErr.Kind == ErrKindContextWindow {
			return zero, err
		}

		// Check x-should-retry header
		if isAPIErr && apiErr.Headers != nil {
			if xRetry := apiErr.Headers.Get("x-should-retry"); xRetry == "false" {
				return zero, &CannotRetryError{
					Cause:        err,
					RetryContext: RetryContext{Attempt: attempt, Model: opts.Model, QuerySource: opts.QuerySource},
				}
			}
		}

		if isAPIErr && apiErr.StatusCode == 529 {
			consecutive529++

			// 529 background → abandon
			if !isForeground {
				return zero, &CannotRetryError{
					Cause:        err,
					RetryContext: RetryContext{Attempt: attempt, Model: opts.Model, QuerySource: opts.QuerySource},
				}
			}

			// Consecutive 529 with Opus model → trigger fallback
			if consecutive529 >= Max529Retries && opts.FallbackModel != "" && isOpusModel(opts.Model) {
				return zero, &FallbackTriggeredError{
					OriginalModel: opts.Model,
					FallbackModel: opts.FallbackModel,
				}
			}
		} else {
			consecutive529 = 0
		}

		// Decide if this error is retryable
		if !isRetryable(err, apiErr) {
			return zero, err
		}

		if attempt >= maxRetries {
			break
		}

		// Compute delay
		var retryAfterHeader string
		if isAPIErr && apiErr.Headers != nil {
			retryAfterHeader = apiErr.Headers.Get("retry-after")
		}
		delay := RetryDelay(attempt+1, retryAfterHeader, MaxDelayMS)

		// Wait
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}
	}

	var zero2 T
	return zero2, fmt.Errorf("api: max retries (%d) exceeded", maxRetries)
}

// isRetryable decides whether the given error warrants a retry attempt.
func isRetryable(err error, apiErr *APIError) bool {
	if apiErr == nil {
		// Network-level errors are generally retryable
		return true
	}
	switch apiErr.StatusCode {
	case 429, 529:
		return true
	case 408, 409:
		return true
	case http.StatusInternalServerError, 502, 503, 504:
		return true
	case 401:
		return true // token refresh will be handled by caller
	default:
		return false
	}
}

// isOpusModel returns true if the model name refers to a Claude Opus model.
func isOpusModel(model string) bool {
	// Match any model containing "opus" (case-insensitive via lowercase check)
	for i := 0; i < len(model)-3; i++ {
		if model[i] == 'o' || model[i] == 'O' {
			sub := model[i:]
			if len(sub) >= 4 && (sub[:4] == "opus" || sub[:4] == "Opus") {
				return true
			}
		}
	}
	return false
}

// RetryDelay computes the backoff duration for a given attempt (1-indexed).
//
//	delay = min(baseDelay * 2^(attempt-1), maxDelay) * (1 + rand*0.25)
//
// If retryAfterHeader is set and parseable, it takes precedence.
func RetryDelay(attempt int, retryAfterHeader string, maxDelayMS int) time.Duration {
	if retryAfterHeader != "" {
		if seconds, err := strconv.ParseFloat(retryAfterHeader, 64); err == nil {
			return time.Duration(seconds * float64(time.Second))
		}
	}

	base := BaseDelayMS
	shift := attempt - 1
	if shift > 30 {
		shift = 30 // prevent overflow
	}
	delay := base * (1 << shift)
	if delay > maxDelayMS {
		delay = maxDelayMS
	}
	// Add 25% jitter
	jitter := float64(delay) * (1 + rand.Float64()*0.25)
	return time.Duration(jitter) * time.Millisecond
}
