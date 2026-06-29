#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PROXY_URL="${PROXY_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-dev-key-1}"
COMPOSE="docker compose"

require_cmd() {
  local cmd="$1"
  local hint="$2"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "fail: required command '$cmd' not found — $hint" >&2
    exit 1
  fi
}

require_cmd docker "install Docker Desktop or Docker Engine (https://docs.docker.com/get-docker/)"
require_cmd curl "install curl (e.g. brew install curl)"

wait_for() {
  local url="$1"
  local name="$2"
  for _ in $(seq 1 60); do
    if curl -sf "$url" >/dev/null 2>&1; then
      echo "ok: $name ready"
      return 0
    fi
    sleep 1
  done
  echo "fail: $name not ready at $url" >&2
  return 1
}

wait_policy_trace() {
  local req_id="$1"
  for _ in $(seq 1 50); do
    if $COMPOSE logs proxy 2>/dev/null | grep "policy_trace" | grep -q "$req_id"; then
      return 0
    fi
    sleep 0.2
  done
  return 1
}

print_policy_trace() {
  local req_id="$1"
  local line
  line="$($COMPOSE logs proxy 2>/dev/null | grep "policy_trace" | grep "$req_id" | tail -1 || true)"
  if [[ -z "$line" ]]; then
    echo "    policy_trace: (not found for $req_id)" >&2
    return 1
  fi
  echo "    policy_trace: $line"
}

echo "==> HSAR Demo (enforce mode)"
echo "==> starting stack with MODE=enforce"
MODE=enforce ENFORCE_KILL_SWITCH=false $COMPOSE up --build -d

echo "==> preflight: proxy health"
wait_for "$PROXY_URL/healthz" "proxy"

echo "==> preflight: signal-engine running"
if ! $COMPOSE ps --status running signal-engine 2>/dev/null | grep -q signal-engine; then
  echo "fail: signal-engine is not running — demo requires healthy perception (no fail-open path)" >&2
  exit 1
fi
echo "ok: signal-engine running"

echo "==> Step 1/2: calm request (passthrough)"
resp1="$(curl -sf -X POST "$PROXY_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'content-type: application/json' \
  -H 'X-Request-ID: demo-calm-1' \
  -d '{"messages":[{"role":"user","content":"Hello, I need help checking my order status."}]}')"
echo "$resp1" | grep -q 'echo backend ok' || {
  echo "fail: expected echo backend response on calm request" >&2
  echo "$resp1" >&2
  exit 1
}
wait_policy_trace "demo-calm-1" || {
  echo "fail: policy_trace not found for demo-calm-1" >&2
  exit 1
}
echo "==> Step 1/2: calm request ... OK"
print_policy_trace "demo-calm-1"

echo "==> Step 2/2: high-risk request (governance)"
resp2="$(curl -sf -X POST "$PROXY_URL/v1/chat/completions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'content-type: application/json' \
  -H 'X-Request-ID: demo-risk-1' \
  -d '{"messages":[{"role":"user","content":"THIS IS UNACCEPTABLE!!! I want a manager NOW!!!"}]}')"
echo "$resp2" | grep -q 'echo backend ok' || {
  echo "fail: expected echo backend response on high-risk request" >&2
  echo "$resp2" >&2
  exit 1
}
wait_policy_trace "demo-risk-1" || {
  echo "fail: policy_trace not found for demo-risk-1" >&2
  exit 1
}
trace2="$($COMPOSE logs proxy 2>/dev/null | grep "policy_trace" | grep "demo-risk-1" | tail -1 || true)"
if [[ -z "$trace2" ]] || ! echo "$trace2" | grep -qE 'INJECT_SYSTEM_CONTEXT|DAMPEN_VERBOSITY|ESCALATE|BLOCK'; then
  echo "fail: expected applied governance action in policy_trace for demo-risk-1" >&2
  echo "$trace2" >&2
  exit 1
fi
echo "==> Step 2/2: high-risk request ... OK"
print_policy_trace "demo-risk-1"

echo "demo: all checks passed"