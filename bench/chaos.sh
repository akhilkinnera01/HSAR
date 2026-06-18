#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PROXY_URL="${PROXY_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-dev-key-1}"
VUS="${VUS:-50}"
DURATION="${DURATION:-3m}"
CHAOS_AT="${CHAOS_AT:-30}"

if ! command -v k6 >/dev/null 2>&1; then
  echo "k6 is required (brew install k6)" >&2
  exit 1
fi

export MODE=enforce
docker compose up -d --build proxy signal-engine backend

echo "Starting k6 load (enforce mode)..."
SCENARIO=proxy PROXY_URL="$PROXY_URL" API_KEY="$API_KEY" VUS="$VUS" DURATION="$DURATION" \
  k6 run bench/load.js > bench/results/chaos-k6.json &
K6_PID=$!

sleep "$CHAOS_AT"
echo "Stopping signal-engine at T+${CHAOS_AT}s..."
docker compose stop signal-engine

wait "$K6_PID" || {
  docker compose start signal-engine
  exit 1
}

FAIL_OPEN=$(curl -sf "${PROXY_URL}/metrics" | grep -c 'hsar_fail_open_total{' || true)
echo "fail_open_series_count=$FAIL_OPEN"

docker compose start signal-engine

if [[ "$FAIL_OPEN" -lt 1 ]]; then
  echo "chaos_fail_open_success=fail (no fail_open counter increase observed)" >&2
  exit 1
fi

echo "chaos_fail_open_success=pass"