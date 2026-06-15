#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PROXY_URL="${PROXY_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-dev-key-1}"
COMPOSE="docker compose"

echo "==> rebuilding proxy with MODE=enforce"
MODE=enforce ENFORCE_KILL_SWITCH=false $COMPOSE up --build -d proxy

for _ in $(seq 1 30); do
  if curl -sf "$PROXY_URL/healthz" >/dev/null 2>&1; then
    echo "ok: proxy ready (enforce mode)"
    break
  fi
  sleep 1
done

echo "==> enforce mode returns 200"
resp="$(curl -sf -X POST "$PROXY_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'content-type: application/json' \
  -H 'X-Request-ID: smoke-enforce-1' \
  -d '{"messages":[{"role":"user","content":"THIS IS UNACCEPTABLE!!!"}]}')"
echo "$resp" | grep -q 'echo backend ok' || {
  echo "fail: expected echo backend response" >&2
  echo "$resp" >&2
  exit 1
}

echo "==> proxy logs contain enforce policy_trace"
$COMPOSE logs proxy 2>/dev/null | grep -q 'policy_trace' || {
  echo "fail: expected policy_trace log" >&2
  exit 1
}

echo "==> restore shadow mode"
MODE=shadow $COMPOSE up -d proxy >/dev/null

echo "smoke-enforce: all checks passed"