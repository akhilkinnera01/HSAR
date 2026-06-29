#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PROXY_URL="${PROXY_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-dev-key-1}"
VUS="${VUS:-50}"
DURATION="${DURATION:-3m}"
CHAOS_AT="${CHAOS_AT:-30}"
HEALTHY_DURATION="${HEALTHY_DURATION:-30s}"

if ! command -v k6 >/dev/null 2>&1; then
  echo "k6 is required (brew install k6)" >&2
  exit 1
fi

mkdir -p bench/results
chmod +x bench/wait_proxy.sh

export MODE=enforce
docker compose up -d proxy signal-engine backend
./bench/wait_proxy.sh

echo "==> healthy baseline (${HEALTHY_DURATION})"
k6 run --summary-export bench/results/chaos-healthy.json \
  -e SCENARIO=proxy -e PROXY_URL="$PROXY_URL" -e API_KEY="$API_KEY" \
  -e VUS="$VUS" -e DURATION="$HEALTHY_DURATION" \
  bench/load.js

echo "==> chaos load (${DURATION}), stop engine at T+${CHAOS_AT}s"
k6 run --summary-export bench/results/chaos-outage.json \
  -e SCENARIO=proxy -e PROXY_URL="$PROXY_URL" -e API_KEY="$API_KEY" \
  -e VUS="$VUS" -e DURATION="$DURATION" \
  bench/load.js &
K6_PID=$!

sleep "$CHAOS_AT"
echo "==> stopping signal-engine"
docker compose stop signal-engine

if ! wait "$K6_PID"; then
  docker compose start signal-engine
  echo "chaos_fail_open_success=fail (k6 thresholds failed)" >&2
  exit 1
fi

FAIL_OPEN=$(curl -sf "${PROXY_URL}/metrics" | grep -c 'hsar_fail_open_total{' || true)
echo "fail_open_series_count=$FAIL_OPEN"

docker compose start signal-engine
./bench/wait_proxy.sh

if [[ "$FAIL_OPEN" -lt 1 ]]; then
  echo "chaos_fail_open_success=fail (no fail_open counter increase)" >&2
  exit 1
fi

python3 bench/update_runtime_slos.py --chaos
echo "chaos_fail_open_success=pass"