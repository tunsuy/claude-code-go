package permissions

import "time"

// DenialLimits are the thresholds at which the permission system downgrades
// itself to prompting mode: a run of consecutive denials (the model repeatedly
// retrying the same denied action) or a large accumulated total both suggest
// the deny rules are fighting the session instead of guarding it.
type DenialLimits struct {
	// MaxConsecutive is the number of denials in a row (with no intervening
	// allow) that triggers the downgrade.
	MaxConsecutive int
	// MaxTotal is the cumulative number of denials in the session that
	// triggers the downgrade.
	MaxTotal int
}

// DENIAL_LIMITS is the default downgrade threshold set.
var DENIAL_LIMITS = DenialLimits{
	MaxConsecutive: 3,
	MaxTotal:       20,
}

// DenialTrackingState tracks permission denials across a session.
// A high denial count triggers automatic prompting-mode downgrade.
type DenialTrackingState struct {
	// DenialCount is the total number of permission denials recorded.
	DenialCount int
	// LastDeniedAt is the timestamp of the most-recent denial.
	LastDeniedAt time.Time
	// RecentDenials is the list of recent denial records.
	RecentDenials []DenialRecord
	// consecutive tracks denials recorded with no intervening approval.
	consecutive int
}

// DenialRecord is a single permission-denial event.
type DenialRecord struct {
	// ToolName is the name of the tool that was denied.
	ToolName string
	// ToolUseID is the API-level tool_use block ID.
	ToolUseID string
	// Reason is a human-readable description of why the tool was denied.
	Reason string
	// DeniedAt is when the denial was recorded.
	DeniedAt time.Time
}

// Record appends a new denial and increments DenialCount.
func (d *DenialTrackingState) Record(rec DenialRecord) {
	if rec.DeniedAt.IsZero() {
		rec.DeniedAt = time.Now()
	}
	d.DenialCount++
	d.consecutive++
	d.LastDeniedAt = rec.DeniedAt
	d.RecentDenials = append(d.RecentDenials, rec)
}

// RecordApproval resets the consecutive-denial streak (the model found an
// approved path; a later denial streak starts counting from zero again).
func (d *DenialTrackingState) RecordApproval() {
	d.consecutive = 0
}

// shouldFallbackToPrompting reports whether the permission pipeline should
// downgrade to prompting mode: the model keeps hitting denials (consecutive
// streak or accumulated total), so asking the user directly is cheaper than
// continuing to auto-deny.
func (d *DenialTrackingState) shouldFallbackToPrompting() bool {
	return d.consecutive >= DENIAL_LIMITS.MaxConsecutive ||
		d.DenialCount >= DENIAL_LIMITS.MaxTotal
}
