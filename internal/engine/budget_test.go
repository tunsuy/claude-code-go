package engine

import (
	"testing"
)

func TestBudgetTracker_NoBudgetPressure(t *testing.T) {
	bt := NewBudgetTracker()
	// Under the 90% completion threshold — should continue without nudge.
	dec := bt.Check(1000, 8192)
	if dec.Action != "continue" {
		t.Fatalf("expected continue, got %q", dec.Action)
	}
	if dec.NudgeMessage != "" {
		t.Fatalf("expected no nudge, got %q", dec.NudgeMessage)
	}
	if dec.ContinuationCount != 0 {
		t.Fatalf("expected continuations=0, got %d", dec.ContinuationCount)
	}
}

func TestBudgetTracker_CompletionThreshold(t *testing.T) {
	bt := NewBudgetTracker()
	// 90% of 8192 = 7372 — trigger nudge.
	maxTokens := 8192
	outputTokens := int(float64(maxTokens)*CompletionThreshold) + 1
	dec := bt.Check(outputTokens, maxTokens)
	if dec.Action != "continue" {
		t.Fatalf("expected continue with nudge, got %q", dec.Action)
	}
	if dec.NudgeMessage == "" {
		t.Fatal("expected nudge message at completion threshold")
	}
	if dec.ContinuationCount != 1 {
		t.Fatalf("expected continuations=1, got %d", dec.ContinuationCount)
	}
}

func TestBudgetTracker_MaxContinuations(t *testing.T) {
	bt := NewBudgetTracker()
	maxTokens := 8192
	overThreshold := int(float64(maxTokens)*CompletionThreshold) + 1

	// Exhaust continuations. Each call uses a higher token count so that
	// the diminishing-returns check (which requires a non-trivial delta)
	// does not fire before MaxContinuations is reached.
	for i := 0; i < MaxContinuations; i++ {
		tokens := overThreshold + i*(DiminishingThreshold+100)
		dec := bt.Check(tokens, maxTokens)
		if dec.Action != "continue" {
			t.Fatalf("turn %d: expected continue, got %q", i+1, dec.Action)
		}
	}

	// Next check should stop.
	dec := bt.Check(overThreshold+MaxContinuations*(DiminishingThreshold+100), maxTokens)
	if dec.Action != "stop" {
		t.Fatalf("expected stop after max continuations, got %q", dec.Action)
	}
}

func TestBudgetTracker_DiminishingReturns(t *testing.T) {
	bt := NewBudgetTracker()
	maxTokens := 8192
	overThreshold := int(float64(maxTokens)*CompletionThreshold) + 1

	// First turn — triggers nudge (continuation 1).
	bt.Check(overThreshold, maxTokens)

	// Second turn — very small delta → diminishing returns.
	// Feed overThreshold + (DiminishingThreshold - 1) to produce a tiny delta.
	smallDelta := DiminishingThreshold - 1
	dec := bt.Check(overThreshold+smallDelta, maxTokens)
	if dec.Action != "stop" {
		t.Fatalf("expected stop on diminishing returns, got %q", dec.Action)
	}
	if !dec.DiminishingReturns {
		t.Fatal("expected DiminishingReturns=true")
	}
}

func TestBudgetTracker_ZeroMaxTokens(t *testing.T) {
	bt := NewBudgetTracker()
	// Zero max_tokens should not panic and should continue.
	dec := bt.Check(1000, 0)
	if dec.Action != "continue" {
		t.Fatalf("expected continue with zero maxTokens, got %q", dec.Action)
	}
}

func TestBudgetTracker_NudgeMessages(t *testing.T) {
	cases := []struct {
		count int
		want  string
	}{
		{1, "wrap up"},
		{2, "very close"},
		{3, "Output limit"},
	}
	for _, tc := range cases {
		msg := buildNudgeMessage(tc.count)
		if msg == "" {
			t.Errorf("buildNudgeMessage(%d) returned empty string", tc.count)
		}
	}
}
