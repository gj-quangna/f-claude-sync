#!/bin/bash
# Check if claude-sync is configured and return status as additionalContext

if ! command -v claude-sync &> /dev/null; then
    echo "claude-sync is not installed. Install with: npm install -g @tawandotorg/claude-sync"
    exit 0
fi

MISSING=""
for VAR in CLAUDE_SYNC_CONFIG_PATH CLAUDE_SYNC_AGE_PATH CLAUDE_SYNC_STATE_PATH; do
    [ -z "${!VAR}" ] && MISSING="$MISSING $VAR"
done
if [ -n "$MISSING" ]; then
    echo "claude-sync: required environment variable(s) not set:$MISSING. Export them (paths to config.yaml, age-key.txt, state.json) before using claude-sync."
    exit 0
fi

CONFIG_FILE="${CLAUDE_SYNC_CONFIG_PATH/#\~/$HOME}"
KEY_FILE="${CLAUDE_SYNC_AGE_PATH/#\~/$HOME}"

if [ ! -f "$CONFIG_FILE" ]; then
    echo "claude-sync is not configured. Run /sync-init to set up cross-device sync."
    exit 0
fi

if [ ! -f "$KEY_FILE" ]; then
    echo "claude-sync encryption key not found. Run /sync-init to configure."
    exit 0
fi

# Check if there are pending changes
STATUS=$(claude-sync status -q 2>/dev/null)
if [ $? -eq 0 ] && [ -n "$STATUS" ]; then
    COUNT=$(echo "$STATUS" | wc -l | tr -d ' ')
    echo "claude-sync: $COUNT file(s) pending. Use /sync-push to upload or /sync-pull to download."
fi
