package engine

import (
	"time"
)

// BudgetTracker manages the token-budget accounting across continuation rounds.
// It determines whether the engine should nudge the LLM to wrap up or halt.
//
// Corresponds to TS tokenBudget.ts / BudgetTracker.
type BudgetTracker struct {
	// ContinuationCount counts how many "nudge" messages have been injected.
	ContinuationCount int
	// LastDeltaTokens is the output-token count from the most-recent turn.
	LastDeltaTokens int
	// LastGlobalTurnTokens is the cumulative output tokens seen so far.
	LastGlobalTurnTokens int
	// StartedAt records when the current query loop began.
	StartedAt time.Time
}

// Token-budget constants (mirrors TS tokenBudget.ts).
const (
	// CompletionThreshold is the fraction of max_tokens at which a nudge is
	// injected to encourage the model to wrap up.
	CompletionThreshold = 0.90

	// DiminishingThreshold is the minimum per-turn output-token delta below
	// which we consider the model to have diminishing returns.
	DiminishingThreshold = 500

	// MaxContinuations is the maximum number of nudge messages before we
	// forcibly stop the query loop.
	MaxContinuations = 3
)

// TokenBudgetDecision is the result of a budget check at the end of each turn.
type TokenBudgetDecision struct {
	// Action is "continue" or "stop".
	Action string
	// NudgeMessage is the message to prepend when Action == "continue".
	NudgeMessage string
	// ContinuationCount is the current continuation counter.
	ContinuationCount int
	// Pct is the percentage of the token budget consumed (0-100).
	Pct int
	// TurnTokens is the output-token count for the last turn.
	TurnTokens int
	// Budget is the max_tokens ceiling.
	Budget int
	// DiminishingReturns is set when the turn produced fewer tokens than the threshold.
	DiminishingReturns bool
	// DurationMs is the wall-clock milliseconds since StartedAt.
	DurationMs int64
}

// NewBudgetTracker returns a new BudgetTracker with StartedAt set to now.
func NewBudgetTracker() *BudgetTracker {
	return &BudgetTracker{StartedAt: time.Now()}
}

// Check evaluates the current token usage and decides whether to continue or stop.
//
//   - outputTokens: cumulative output tokens consumed so far.
//   - maxTokens:    the max_tokens value for the current model/request.
func (b *BudgetTracker) Check(outputTokens, maxTokens int) TokenBudgetDecision {
	now := time.Now()
	durationMs := now.Sub(b.StartedAt).Milliseconds()

	deltaSinceLastTurn := outputTokens - b.LastGlobalTurnTokens
	b.LastDeltaTokens = deltaSinceLastTurn
	b.LastGlobalTurnTokens = outputTokens

	pct := 0
	if maxTokens > 0 {
		pct = int(float64(outputTokens) / float64(maxTokens) * 100)
	}

	decision := TokenBudgetDecision{
		ContinuationCount: b.ContinuationCount,
		Pct:               pct,
		TurnTokens:        deltaSinceLastTurn,
		Budget:            maxTokens,
		DurationMs:        durationMs,
	}

	// Check for diminishing returns: very little was produced this turn.
	if b.ContinuationCount > 0 && deltaSinceLastTurn < DiminishingThreshold {
		decision.Action = "stop"
		decision.DiminishingReturns = true
		return decision
	}

	// Check if we've exceeded the maximum number of continuations.
	if b.ContinuationCount >= MaxContinuations {
		decision.Action = "stop"
		return decision
	}

	// Check if we've crossed the completion threshold.
	if maxTokens > 0 && float64(outputTokens)/float64(maxTokens) >= CompletionThreshold {
		b.ContinuationCount++
		decision.ContinuationCount = b.ContinuationCount
		decision.Action = "continue"
		decision.NudgeMessage = buildNudgeMessage(b.ContinuationCount)
		return decision
	}

	decision.Action = "continue"
	return decision
}

// buildNudgeMessage returns the continuation-nudge message for the given count.
func buildNudgeMessage(count int) string {
	switch count {
	case 1:
		return "You are approaching the output token limit. Please wrap up your current task and provide a final response."
	case 2:
		return "You are very close to the output token limit. Please finish your response immediately with what you have so far."
	default:
		return "Output limit reached. Provide your final response now."
	}
}
