#!/usr/bin/env bash
# Put-or-create a Secrets Manager JSON secret, then write a slim pointer prod.env.
# Usage: sync-secret.sh <secret-id> <payload.json> <prod.env-path> [extra-slim-KEY=val ...]
set -euo pipefail

SECRET_ID="${1:?secret-id}"
PAYLOAD_FILE="${2:?payload.json}"
ENV_PATH="${3:?prod.env path}"
REGION="${AWS_REGION:-us-east-1}"
shift 3 || true

if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI required" >&2
  exit 1
fi

raw="$(cat "${PAYLOAD_FILE}")"
if aws secretsmanager put-secret-value \
  --region "${REGION}" \
  --secret-id "${SECRET_ID}" \
  --secret-string "${raw}" >/dev/null 2>&1; then
  echo "updated secret ${SECRET_ID}"
else
  aws secretsmanager create-secret \
    --region "${REGION}" \
    --name "${SECRET_ID}" \
    --secret-string "${raw}" >/dev/null
  echo "created secret ${SECRET_ID}"
fi

umask 077
{
  printf "AWS_REGION='%s'\n" "${REGION}"
  printf "SECRET_ID='%s'\n" "${SECRET_ID}"
  for kv in "$@"; do
    key="${kv%%=*}"
    val="${kv#*=}"
    val="${val//\'/\'\"\'\"\'}"
    printf "%s='%s'\n" "${key}" "${val}"
  done
} > "${ENV_PATH}"
echo "wrote slim ${ENV_PATH}"
