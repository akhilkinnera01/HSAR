#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PROXY_URL="${PROXY_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-dev-key-1}"
COMPOSE="docker compose"

wait_for() {
  local url="$1"
  local name="$2"
  for _ in $(seq 1 30); do
    if curl -sf "$url" >/dev/null 2>&1; then
      echo "ok: $name ready"
      return 0
    fi
    sleep 1
  done
  echo "fail: $name not ready at $url" >&2
  return 1
}

echo "==> waiting for services"
wait_for "$PROXY_URL/healthz" "proxy"

echo "==> missing API key returns 401"
code="$(curl -s -o /dev/null -w "%{http_code}" -X POST "$PROXY_URL/v1/chat/completions" \
  -H 'content-type: application/json' \
  -d '{"messages":[{"role":"user","content":"no auth"}]}')"
if [ "$code" != "401" ]; then
  echo "fail: expected 401 without API key, got $code" >&2
  exit 1
fi

echo "==> chat completion returns 200 with auth"
resp="$(curl -sf -X POST "$PROXY_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'content-type: application/json' \
  -H 'X-Request-ID: smoke-test-1' \
  -d '{"messages":[{"role":"user","content":"I AM SO ANGRY!!!"}]}')"
echo "$resp" | grep -q 'echo backend ok' || {
  echo "fail: expected echo backend response" >&2
  echo "$resp" >&2
  exit 1
}

echo "==> proxy logs contain signal frame"
$COMPOSE logs proxy 2>/dev/null | grep -q 'signal_engine_signalframe' || {
  echo "fail: expected shadow signal frame log from proxy" >&2
  exit 1
}

echo "==> fail-open with signal engine stopped"
$COMPOSE stop signal-engine >/dev/null
sleep 2

resp2="$(curl -sf -X POST "$PROXY_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'content-type: application/json' \
  -H 'X-Request-ID: smoke-test-2' \
  -d '{"messages":[{"role":"user","content":"still works"}]}')"
echo "$resp2" | grep -q 'echo backend ok' || {
  echo "fail: expected passthrough with engine down" >&2
  echo "$resp2" >&2
  exit 1
}

$COMPOSE start signal-engine >/dev/null

echo "smoke: all checks passed"