#!/bin/bash
# Claude rate limit checker + push to Mattermost AI Limits Monitor plugin.
#
# Runs Claude CLI with fetch interceptor to capture rate limit headers,
# then pushes the data to the plugin's webhook endpoint.
#
# Usage: claude-push.sh
#
# Environment variables (or edit below):
#   CLAUDE_OAUTH_TOKEN  - OAuth token (sk-ant-oat01-...)
#   MM_URL              - Mattermost URL (e.g. https://mm.fambear.online)
#   MM_BOT_TOKEN        - Mattermost bot token for auth
#   CLAUDE_CHECK_JS     - Path to claude-check.js (default: same directory)
#
# Add to crontab: */30 * * * * /path/to/claude-push.sh

set -e

# Config (override via env vars)
CLAUDE_OAUTH_TOKEN="${CLAUDE_OAUTH_TOKEN:-}"
MM_URL="${MM_URL:-https://mm.fambear.online}"
MM_BOT_TOKEN="${MM_BOT_TOKEN:-8ahfnjfcp7n5xgr66dxprydeww}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CLAUDE_CHECK_JS="${CLAUDE_CHECK_JS:-${SCRIPT_DIR}/claude-check.js}"

if [ -z "$CLAUDE_OAUTH_TOKEN" ]; then
    echo "Error: CLAUDE_OAUTH_TOKEN not set"
    exit 1
fi

if [ ! -f "$CLAUDE_CHECK_JS" ]; then
    echo "Error: claude-check.js not found at $CLAUDE_CHECK_JS"
    exit 1
fi

if ! command -v claude &>/dev/null; then
    echo "Error: claude CLI not found. Install: npm install -g @anthropic-ai/claude-code"
    exit 1
fi

TMPFILE=$(mktemp /tmp/claude-ratelimit-XXXXXX.json)
trap "rm -f $TMPFILE" EXIT

# Run claude CLI with fetch interceptor
export CLAUDE_CODE_OAUTH_TOKEN="$CLAUDE_OAUTH_TOKEN"
export CLAUDE_CHECK_OUTPUT="$TMPFILE"
export NODE_OPTIONS="--require $CLAUDE_CHECK_JS"

# Use script for PTY (required by claude CLI)
timeout 45 script -qc "claude -p ok --output-format json --model claude-sonnet-4-20250514 --no-session-persistence --max-budget-usd 0.01" /dev/null >/dev/null 2>/dev/null || true

if [ ! -s "$TMPFILE" ]; then
    # Push error to plugin
    curl -s -X POST "${MM_URL}/plugins/com.fambear.ai-limits-monitor/api/v1/claude-push" \
        -H "Authorization: Bearer ${MM_BOT_TOKEN}" \
        -H "Content-Type: application/json" \
        -d '{"timestamp":'"$(date +%s)"',"rateLimits":{},"error":"Claude CLI failed to return rate limit data"}' \
        >/dev/null 2>&1
    exit 1
fi

# Push data to plugin
curl -s -X POST "${MM_URL}/plugins/com.fambear.ai-limits-monitor/api/v1/claude-push" \
    -H "Authorization: Bearer ${MM_BOT_TOKEN}" \
    -H "Content-Type: application/json" \
    -d @"$TMPFILE" \
    >/dev/null 2>&1

echo "OK: Claude rate limits pushed to ${MM_URL}"
