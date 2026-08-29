package permissions

import (
	"context"
	"testing"
	"time"

	"github.com/tunsuy/claude-code-go/internal/tools"
	"github.com/tunsuy/claude-code-go/pkg/types"
)

// newDenyChecker builds a checker that denies WebFetch (deny rule) with
// interactive channels available, so the downgrade path can fire.
func newDenyChecker(t *testing.T) (Checker, chan AskRequest, chan AskResponse) {
	t.Helper()
	askCh := make(chan AskRequest, 1)
	respCh := make(chan AskResponse, 1)
	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{
			Mode: types.PermissionModeDefault,
			AlwaysDenyRules: types.ToolPermissionRulesBySource{
				types.RuleSourceUser: {"WebFetch"},
			},
		},
		AskCh:  askCh,
		RespCh: respCh,
	})
	return c, askCh, respCh
}

// denyNTimes calls RequestPermission n times against a deny rule and returns
// the final result.
func denyNTimes(t *testing.T, c Checker, n int) tools.PermissionResult {
	t.Helper()
	var result tools.PermissionResult
	for i := 0; i < n; i++ {
		var err error
		result, err = c.RequestPermission(context.Background(), PermissionRequest{
			ToolName:  "WebFetch",
			ToolUseID: "u",
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func TestShouldFallbackToPrompting_ConsecutiveThreshold(t *testing.T) {
	t.Parallel()
	d := &DenialTrackingState{}
	for i := 0; i < DENIAL_LIMITS.MaxConsecutive-1; i++ {
		d.Record(DenialRecord{ToolName: "Bash"})
		if d.shouldFallbackToPrompting() {
			t.Fatalf("downgraded early after %d consecutive denials (limit %d)", i+1, DENIAL_LIMITS.MaxConsecutive)
		}
	}
	d.Record(DenialRecord{ToolName: "Bash"})
	if !d.shouldFallbackToPrompting() {
		t.Errorf("expected downgrade at %d consecutive denials", DENIAL_LIMITS.MaxConsecutive)
	}
}

func TestShouldFallbackToPrompting_TotalThreshold(t *testing.T) {
	t.Parallel()
	d := &DenialTrackingState{}
	for i := 0; i < DENIAL_LIMITS.MaxTotal; i++ {
		// An approval between every denial keeps the consecutive streak at 0,
		// so only the accumulated total can trigger the downgrade.
		d.Record(DenialRecord{ToolName: "Bash"})
		d.RecordApproval()
	}
	if !d.shouldFallbackToPrompting() {
		t.Errorf("expected downgrade at %d total denials", DENIAL_LIMITS.MaxTotal)
	}
}

func TestShouldFallbackToPrompting_NonConsecutiveDoesNotTriggerStreak(t *testing.T) {
	t.Parallel()
	d := &DenialTrackingState{}
	// Alternate denial/approval: 10 denials total (below MaxTotal), streak never
	// exceeds 1.
	for i := 0; i < 10; i++ {
		d.Record(DenialRecord{ToolName: "Bash"})
		if d.shouldFallbackToPrompting() {
			t.Fatalf("downgraded at iteration %d with streak %d and total %d", i, d.consecutive, d.DenialCount)
		}
		d.RecordApproval()
	}
}

func TestRecordApproval_ResetsConsecutive(t *testing.T) {
	t.Parallel()
	d := &DenialTrackingState{}
	for i := 0; i < DENIAL_LIMITS.MaxConsecutive; i++ {
		d.Record(DenialRecord{ToolName: "Bash"})
	}
	d.RecordApproval()
	if d.shouldFallbackToPrompting() {
		t.Error("approval must reset the consecutive streak")
	}
}

func TestRequestPermission_ConsecutiveDenialsFallBackToAsking(t *testing.T) {
	t.Parallel()
	c, askCh, respCh := newDenyChecker(t)

	// The first MaxConsecutive denials are auto-denied without asking.
	for i := 0; i < DENIAL_LIMITS.MaxConsecutive; i++ {
		select {
		case req := <-askCh:
			t.Fatalf("downgraded to ask too early (iteration %d): %+v", i, req)
		default:
		}
		result := denyNTimes(t, c, 1)
		if result.Behavior != tools.PermissionDeny {
			t.Fatalf("iteration %d: expected Deny before threshold, got %q", i, result.Behavior)
		}
	}

	// The next denied call is routed to the user instead of auto-denied.
	respCh <- AskResponse{ID: "u", Decision: tools.PermissionAllow}
	result := denyNTimes(t, c, 1)
	if result.Behavior != tools.PermissionAllow {
		t.Fatalf("expected downgrade to user-confirmed Allow, got %q", result.Behavior)
	}

	select {
	case req := <-askCh:
		if req.Message == "" {
			t.Error("downgrade ask must carry an explanatory message")
		}
	default:
		t.Fatal("threshold denial should have emitted an AskRequest")
	}

	// Approval resets the streak: a fresh denial goes back to auto-deny.
	result = denyNTimes(t, c, 1)
	if result.Behavior != tools.PermissionDeny {
		t.Errorf("after approval, denial streak must restart; got %q", result.Behavior)
	}
}

func TestRequestPermission_NoChannelsMeansNoDowngrade(t *testing.T) {
	t.Parallel()
	c := NewChecker(CheckerConfig{
		PermCtx: types.ToolPermissionContext{
			Mode: types.PermissionModeDefault,
			AlwaysDenyRules: types.ToolPermissionRulesBySource{
				types.RuleSourceUser: {"WebFetch"},
			},
		},
	})
	for i := 0; i < DENIAL_LIMITS.MaxConsecutive+2; i++ {
		result := denyNTimes(t, c, 1)
		if result.Behavior != tools.PermissionDeny {
			t.Fatalf("iteration %d: without UI channels denials must stay denials, got %q", i, result.Behavior)
		}
	}
	if c.GetDenialCount() != DENIAL_LIMITS.MaxConsecutive+2 {
		t.Errorf("GetDenialCount = %d, want %d", c.GetDenialCount(), DENIAL_LIMITS.MaxConsecutive+2)
	}
}

func TestDenialRecord_FillsDeniedAt(t *testing.T) {
	t.Parallel()
	d := &DenialTrackingState{}
	before := time.Now()
	d.Record(DenialRecord{ToolName: "Bash"})
	if d.RecentDenials[0].DeniedAt.Before(before) {
		t.Error("Record must stamp DeniedAt when unset")
	}
}
