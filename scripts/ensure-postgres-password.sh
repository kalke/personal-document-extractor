#!/usr/bin/env bash
# Ensure POSTGRES_PASSWORD is set in prod.env for Compose interpolation.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ ! -f prod.env ]]; then
  echo "prod.env missing in ${ROOT}" >&2
  exit 1
fi

pw="$(awk -F= '/^POSTGRES_PASSWORD=/{sub(/^[^=]*=/,""); gsub(/^['\''"]+|['\''"]+$/,""); print; exit}' prod.env || true)"
if [[ -n "${pw}" ]]; then
  exit 0
fi

pw="$(openssl rand -hex 16)"
if grep -qE '^POSTGRES_PASSWORD=' prod.env; then
  awk -v pw="$pw" '
    BEGIN { done=0 }
    /^POSTGRES_PASSWORD=/ && !done { print "POSTGRES_PASSWORD='\''" pw "'\''"; done=1; next }
    { print }
    END { if (!done) print "POSTGRES_PASSWORD='\''" pw "'\''" }
  ' prod.env > prod.env.tmp
  mv prod.env.tmp prod.env
else
  printf "\nPOSTGRES_PASSWORD='%s'\n" "$pw" >> prod.env
fi
echo "generated POSTGRES_PASSWORD and wrote it to prod.env"
