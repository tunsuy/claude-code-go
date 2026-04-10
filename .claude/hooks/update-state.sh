#!/usr/bin/env bash
# update-state.sh — PostToolUse hook for Claude Code Go
# Updates the last_updated field in .claude/harness/state.json
# after any Write or Edit tool use.
#
# Usage: bash .claude/hooks/update-state.sh "$CLAUDE_TOOL_RESULT_FILE"

set -euo pipefail

STATE_FILE="$(git rev-parse --show-toplevel 2>/dev/null || echo ".")/.claude/harness/state.json"

if [ ! -f "$STATE_FILE" ]; then
  exit 0
fi

TODAY=$(date +%Y-%m-%d)

# Use Python for JSON update (available on macOS/Linux without extra deps)
python3 - <<EOF
import json, sys

state_file = "$STATE_FILE"
today = "$TODAY"

try:
    with open(state_file, 'r') as f:
        state = json.load(f)

    state['last_updated'] = today

    with open(state_file, 'w') as f:
        json.dump(state, f, ensure_ascii=False, indent=2)
        f.write('\n')
except Exception as e:
    # Non-fatal: hook failure should not block the tool use
    sys.stderr.write(f"update-state.sh: warning: {e}\n")
    sys.exit(0)
EOF
