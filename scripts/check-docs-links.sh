#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

README="${1:-README.md}"
fail=0

if [[ ! -f "$README" ]]; then
  echo "fail: $README not found" >&2
  exit 1
fi

echo "==> checking markdown links in $README"

while IFS= read -r link; do
  target="${link#*(}"
  target="${target%)*}"
  target="${target%% *}"
  target="${target//\"/}"
  target="${target%%#*}"

  if [[ "$target" == http://* || "$target" == https://* || "$target" == mailto:* || "$target" == \#* || -z "$target" ]]; then
    continue
  fi

  if [[ ! -e "$target" ]]; then
    echo "fail: broken link -> $target" >&2
    fail=1
  else
    echo "ok: $target"
  fi
done < <(grep -oE '\[[^]]+\]\([^)]+\)' "$README" || true)

if [[ "$fail" -ne 0 ]]; then
  echo "check-docs-links: FAILED" >&2
  exit 1
fi

echo "check-docs-links: all relative links resolve"