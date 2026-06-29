#!/usr/bin/env bash
set -euo pipefail

PROXY_URL="${PROXY_URL:-http://localhost:8080}"
for _ in $(seq 1 90); do
  if curl -sf "${PROXY_URL}/healthz" >/dev/null 2>&1; then
    exit 0
  fi
  sleep 1
done
echo "proxy not healthy at ${PROXY_URL}" >&2
exit 1