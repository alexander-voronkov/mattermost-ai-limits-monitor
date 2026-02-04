#!/usr/bin/env node
/**
 * Claude rate limit checker for AI Limits Monitor Mattermost plugin.
 *
 * This script is meant to be loaded as a NODE_OPTIONS --require preload
 * when running the `claude` CLI.  It intercepts fetch() to capture the
 * anthropic-ratelimit-* response headers that are only returned to
 * Claude-Code-authenticated requests, then writes the captured data to
 * a JSON file specified by CLAUDE_CHECK_OUTPUT.
 *
 * Usage (called by the Go plugin):
 *   CLAUDE_CODE_OAUTH_TOKEN=<token> \
 *   CLAUDE_CHECK_OUTPUT=/tmp/claude-rate-limits.json \
 *   NODE_OPTIONS="--require /path/to/claude-check.js" \
 *   claude -p "ok" --output-format json --model claude-sonnet-4-20250514 \
 *         --no-session-persistence --max-budget-usd 0.001
 *
 * Output file (JSON):
 * {
 *   "timestamp": 1234567890,
 *   "rateLimits": {
 *     "anthropic-ratelimit-unified-5h-utilization": "0.45",
 *     "anthropic-ratelimit-unified-5h-reset": "1770228000",
 *     ...
 *   },
 *   "usage": { "input_tokens": 3, "output_tokens": 4 }
 * }
 */

const fs = require('fs');
const outputPath = process.env.CLAUDE_CHECK_OUTPUT;

if (outputPath) {
  const origFetch = globalThis.fetch;
  let captured = false;

  globalThis.fetch = async function (...args) {
    const resp = await origFetch.apply(this, args);
    const url = typeof args[0] === 'string' ? args[0] : args[0]?.url;

    // Capture headers from the main /v1/messages call (not count_tokens or batches)
    if (!captured && url && url.includes('/v1/messages') &&
        !url.includes('batch') && !url.includes('count_tokens')) {
      const headers = {};
      for (const [k, v] of resp.headers.entries()) {
        if (k.includes('ratelimit') || k === 'anthropic-organization-id' || k === 'request-id') {
          headers[k] = v;
        }
      }
      // Only write if we got actual rate limit data
      if (Object.keys(headers).some(k => k.includes('ratelimit'))) {
        captured = true;
        const data = { timestamp: Math.floor(Date.now() / 1000), rateLimits: headers };
        try {
          fs.writeFileSync(outputPath, JSON.stringify(data));
        } catch (e) {
          // Silently fail - don't break the CLI
        }
      }
    }
    return resp;
  };
}
