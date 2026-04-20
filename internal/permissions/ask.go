package permissions

import "github.com/tunsuy/claude-code-go/internal/tools"

// AskRequest is sent from the permission system to the TUI layer when user
// confirmation is needed for a tool call.
type AskRequest struct {
	// ID is a unique request identifier, used to correlate with AskResponse.
	ID string
	// ToolName is the name of the tool requiring permission.
	ToolName string
	// ToolUseID is the API-level tool_use block ID.
	ToolUseID string
	// Message is the human-readable permission explanation to show the user.
	Message string
	// Input is the raw JSON input of the tool call (for display).
	Input tools.Input
	// Suggestions is the list of one-click rule-update options offered to the user.
	Suggestions []tools.PermissionResult
	// BlockedPath is the file path blocked by the permission check (if any).
	BlockedPath string
}

// AskResponse is sent from the TUI layer back to the permission system in reply
// to an AskRequest.
type AskResponse struct {
	// ID must match the AskRequest.ID this response is for.
	ID string
	// Decision is the user's choice: PermissionAllow or PermissionDeny.
	Decision tools.PermissionBehavior
	// UserModified is true if the user altered the tool input before approving.
	UserModified bool
}
