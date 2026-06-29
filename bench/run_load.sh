#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DURATION="${DURATION:-30s}"
VUS="${VUS:-10}"
PROXY_URL="${PROXY_URL:-http://localhost:8080}"
BASELINE_URL="${BASELINE_URL:-http://localhost:8081}"

if ! command -v k6 >/dev/null 2>&1; then
  echo "k6 is required (brew install k6)" >&2
  exit 1
fi

mkdir -p bench/results

run_k6() {
  local name="$1" scenario="$2"
  echo "==> k6 ${name} (${scenario})"
  k6 run --summary-export "bench/results/${name}.json" \
    -e SCENARIO="$scenario" \
    -e DURATION="$DURATION" \
    -e VUS="$VUS" \
    -e PROXY_URL="$PROXY_URL" \
    -e BASELINE_URL="$BASELINE_URL" \
    bench/load.js
}

chmod +x bench/wait_proxy.sh
./bench/wait_proxy.sh

run_k6 baseline baseline

MODE=shadow docker compose up -d proxy
sleep 2
./bench/wait_proxy.sh
run_k6 shadow proxy

MODE=enforce docker compose up -d proxy
sleep 2
./bench/wait_proxy.sh
run_k6 enforce proxy

python3 bench/update_runtime_slos.py --load