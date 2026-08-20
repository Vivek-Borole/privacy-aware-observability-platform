#!/usr/bin/env bash
# Conservative, dependency-free guard for high-confidence credential formats.
# It deliberately reports only a filename so CI never echoes a possible secret.
set -euo pipefail

pattern='(AKIA[0-9A-Z]{16}|ASIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-(proj-)?[A-Za-z0-9_-]{20,}|-----BEGIN([[:space:]][A-Z]+){0,3}[[:space:]]PRIVATE KEY-----)'
failed=0

while IFS= read -r -d '' file; do
  if rg --pcre2 --text -q "$pattern" "$file"; then
    echo "high-confidence credential signature found in tracked source: $file" >&2
    failed=1
  fi
done < <(rg --files -0 -g '!node_modules/**' -g '!docs/evidence/**')

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi

echo 'secret signature scan passed'
