package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// ─── isOpusModel ──────────────────────────────────────────────────────────────

func TestIsOpusModel(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"claude-3-opus-20240229", true},
		{"claude-opus-4-5", true},
		{"Opus-model", true},
		{"claude-3-sonnet-20240229", false},
		{"claude-haiku", false},
		{"", false},
		{"op", false},
		{"opus", true},
	}
	for _, tc := range cases {
		got := isOpusModel(tc.model)
		if got != tc.want {
			t.Errorf("isOpusModel(%q) = %v, want %v", tc.model, got, tc.want)
		}
	}
}

// ─── RetryDelay ───────────────────────────────────────────────────────────────

func TestRetryDelay_ExponentialGrowth(t *testing.T) {
	// delay should be roughly base * 2^(attempt-1)
	d1 := RetryDelay(1, "", MaxDelayMS)
	d2 := RetryDelay(2, "", MaxDelayMS)
	if d2 <= d1 {
		t.Errorf("delay should grow: attempt 1 = %v, attempt 2 = %v", d1, d2)
	}
}

func TestRetryDelay_MaxCap(t *testing.T) {
	// high attempt number should be capped at maxDelay * 1.25 (jitter)
	d := RetryDelay(100, "", MaxDelayMS)
	maxAllowed := time.Duration(float64(MaxDelayMS)*1.25) * time.Millisecond
	if d > maxAllowed {
		t.Errorf("delay %v exceeds max+jitter %v", d, maxAllowed)
	}
}

func TestRetryDelay_RetryAfterHeader(t *testing.T) {
	d := RetryDelay(1, "5", MaxDelayMS)
	if d != 5*time.Second {
		t.Errorf("want 5s, got %v", d)
	}
}

func TestRetryDelay_InvalidRetryAfterFallsBack(t *testing.T) {
	d := RetryDelay(1, "not-a-number", MaxDelayMS)
	if d <= 0 {
		t.Errorf("expected positive fallback delay, got %v", d)
	}
}

// ─── isRetryable ──────────────────────────────────────────────────────────────

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{429, true},
		{529, true},
		{408, true},
		{409, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
		{401, true},
		{400, false},
		{403, false},
		{404, false},
	}
	for _, tc := range cases {
		apiErr := &APIError{StatusCode: tc.status}
		got := isRetryable(apiErr, apiErr)
		if got != tc.want {
			t.Errorf("isRetryable(status=%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestIsRetryable_NilAPIErr(t *testing.T) {
	// Network-level errors (nil APIError) are retryable
	if !isRetryable(errors.New("network error"), nil) {
		t.Error("nil APIError should be retryable (network error)")
	}
}

// ─── WithRetry ────────────────────────────────────────────────────────────────

func TestWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	calls := 0
	result, err := WithRetry(context.Background(), func(_ context.Context, attempt int) (string, error) {
		calls++
		return "ok", nil
	}, RetryOptions{MaxRetries: 3})
	if err != nil || result != "ok" || calls != 1 {
		t.Errorf("expected success on first call, got err=%v result=%q calls=%d", err, result, calls)
	}
}

func TestWithRetry_RetriesTransientErrors(t *testing.T) {
	calls := 0
	result, err := WithRetry(context.Background(), func(_ context.Context, attempt int) (string, error) {
		calls++
		if calls < 3 {
			return "", &APIError{StatusCode: 500, Kind: ErrKindServerError}
		}
		return "done", nil
	}, RetryOptions{MaxRetries: 5})
	if err != nil || result != "done" {
		t.Errorf("expected success after retries, got err=%v result=%q", err, result)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestWithRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := WithRetry(ctx, func(_ context.Context, _ int) (string, error) {
		return "", nil
	}, RetryOptions{MaxRetries: 3})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestWithRetry_ContextWindow400NotRetried(t *testing.T) {
	calls := 0
	_, err := WithRetry(context.Background(), func(_ context.Context, _ int) (string, error) {
		calls++
		return "", &APIError{StatusCode: 400, Kind: ErrKindContextWindow}
	}, RetryOptions{MaxRetries: 5})
	if calls != 1 {
		t.Errorf("context-window error should not retry, got %d calls", calls)
	}
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestWithRetry_XShouldRetryFalse(t *testing.T) {
	calls := 0
	h := http.Header{}
	h.Set("x-should-retry", "false")
	_, err := WithRetry(context.Background(), func(_ context.Context, _ int) (string, error) {
		calls++
		return "", &APIError{StatusCode: 429, Headers: h}
	}, RetryOptions{MaxRetries: 5})
	if calls != 1 {
		t.Errorf("x-should-retry:false should stop at first attempt, got %d calls", calls)
	}
	var cannotRetry *CannotRetryError
	if !errors.As(err, &cannotRetry) {
		t.Errorf("expected CannotRetryError, got %T: %v", err, err)
	}
}

func TestWithRetry_529BackgroundAbandons(t *testing.T) {
	calls := 0
	_, err := WithRetry(context.Background(), func(_ context.Context, _ int) (string, error) {
		calls++
		return "", &APIError{StatusCode: 529}
	}, RetryOptions{MaxRetries: 5, QuerySource: "background"})
	if calls != 1 {
		t.Errorf("background 529 should abandon immediately, got %d calls", calls)
	}
	var cannotRetry *CannotRetryError
	if !errors.As(err, &cannotRetry) {
		t.Errorf("expected CannotRetryError, got %T: %v", err, err)
	}
}

func TestWithRetry_529OpusFallback(t *testing.T) {
	_, err := WithRetry(context.Background(), func(_ context.Context, _ int) (string, error) {
		return "", &APIError{StatusCode: 529}
	}, RetryOptions{
		MaxRetries:    10,
		Model:         "claude-opus-4",
		FallbackModel: "claude-haiku",
		QuerySource:   "repl_main_thread",
	})
	var fallbackErr *FallbackTriggeredError
	if !errors.As(err, &fallbackErr) {
		t.Errorf("expected FallbackTriggeredError after 3×529 on Opus, got %T: %v", err, err)
	}
	if fallbackErr.FallbackModel != "claude-haiku" {
		t.Errorf("unexpected fallback model: %s", fallbackErr.FallbackModel)
	}
}

func TestWithRetry_MaxRetriesExceeded(t *testing.T) {
	calls := 0
	_, err := WithRetry(context.Background(), func(_ context.Context, _ int) (string, error) {
		calls++
		return "", &APIError{StatusCode: 500}
	}, RetryOptions{MaxRetries: 2})
	if err == nil {
		t.Error("expected error after max retries")
	}
	// calls should be maxRetries+1 (initial + maxRetries retries)
	if calls != 3 {
		t.Errorf("expected 3 calls (initial + 2 retries), got %d", calls)
	}
}
