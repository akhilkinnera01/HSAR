#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

OLLAMA_URL="${OLLAMA_URL:-http://localhost:11434}"
PROXY_URL="${PROXY_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-dev-key-1}"
MODEL="${OLLAMA_MODEL:-llama3.2}"
COMPOSE="docker compose"

echo "==> starting Ollama profile"
$COMPOSE --profile ollama up -d ollama

for _ in $(seq 1 60); do
  if curl -sf "$OLLAMA_URL/api/tags" >/dev/null 2>&1; then
    echo "ok: ollama ready"
    break
  fi
  sleep 2
done

if ! curl -sf "$OLLAMA_URL/api/tags" >/dev/null 2>&1; then
  echo "fail: ollama not ready at $OLLAMA_URL" >&2
  exit 1
fi

echo "==> ollama direct chat completion"
direct="$(curl -sf -X POST "$OLLAMA_URL/v1/chat/completions" \
  -H 'content-type: application/json' \
  -d "{\"model\":\"$MODEL\",\"stream\":false,\"messages\":[{\"role\":\"user\",\"content\":\"say hi\"}]}" \
  --max-time 120 2>/dev/null || true)"
if [ -z "$direct" ]; then
  echo "warn: direct ollama chat failed (model $MODEL may need: ollama pull $MODEL)" >&2
else
  echo "ok: ollama direct response received"
fi

if [ "${SMOKE_UPSTREAM:-}" = "ollama" ]; then
  echo "==> proxy pointed at ollama (recreate proxy)"
  UPSTREAM_BASE_URL=http://ollama:11434 $COMPOSE up -d --no-deps proxy

  for _ in $(seq 1 30); do
    if curl -sf "$PROXY_URL/healthz" >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done

  if [ -n "$direct" ]; then
    proxy_resp="$(curl -sf -X POST "$PROXY_URL/v1/chat/completions" \
      -H "Authorization: Bearer $API_KEY" \
      -H 'content-type: application/json' \
      -d "{\"model\":\"$MODEL\",\"stream\":false,\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}" \
      --max-time 120)"
    if [ -z "$proxy_resp" ]; then
      echo "fail: proxy→ollama request failed" >&2
      exit 1
    fi
    echo "ok: proxy→ollama chat completion"
  fi
fi

echo "smoke-ollama: checks passed"