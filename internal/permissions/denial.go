package permissions

import "time"

// DenialTrackingState tracks permission denials across a session.
// A high denial count triggers automatic prompting-mode downgrade.
type DenialTrackingState struct {
	// DenialCount is the total number of permission denials recorded.
	DenialCount int
	// LastDeniedAt is the timestamp of the most-recent denial.
	LastDeniedAt time.Time
	// RecentDenials is the list of recent denial records.
	RecentDenials []DenialRecord
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
	d.LastDeniedAt = rec.DeniedAt
	d.RecentDenials = append(d.RecentDenials, rec)
}
