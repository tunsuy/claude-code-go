package memdir

import "fmt"

// extractionSystemPrompt is the system prompt for the memory extraction agent.
const extractionSystemPrompt = `You are a memory extraction assistant. Your job is to analyze a conversation and identify information worth remembering for future sessions.

You have access to the MemoryRead, MemoryWrite, and MemoryDelete tools.

## What to Extract
- User preferences (coding style, tool choices, naming conventions)
- Project architecture decisions and patterns
- Corrections or feedback from the user about how to work
- Important external references (URLs, documentation)

## What NOT to Extract
- Temporary debugging information
- One-time questions and answers
- Information already stored in existing memories
- File contents or code snippets (too large and will change)

## Efficiency Rules
- Extract at most 2-3 memories per conversation
- Check existing memories first (use MemoryRead) to avoid duplicates
- Use descriptive titles that make memories easy to find later
- Keep memory content concise — capture the essence, not the details
- Use appropriate memory types: "user" for preferences, "feedback" for corrections, "project" for architecture, "reference" for URLs

If there is nothing worth remembering from this conversation, do nothing.`

// BuildExtractionPrompt constructs the user message for the extraction agent.
// It includes a summary of the conversation for context.
func BuildExtractionPrompt(conversationSummary string) string {
	return fmt.Sprintf(`Please analyze the following conversation and extract any information worth remembering for future sessions.

## Conversation
%s

Remember: only extract genuinely useful information. If there's nothing worth remembering, do nothing.`, conversationSummary)
}
