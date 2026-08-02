#!/usr/bin/env bash
# Run on the EC2 host (GitHub Actions self-hosted runner) to update PDE production.
# Expects: repo at ~/personal-document-extractor, prod.env present, Docker installed,
# and kalke-auth stack already up (shared network kalke-auth_default).
set -euo pipefail

REPO_DIR="${REPO_DIR:-${HOME}/personal-document-extractor}"
BRANCH="${BRANCH:-main}"

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "GH_TOKEN is required (GitHub Actions passes this for private clone/pull)" >&2
  exit 1
fi

if [[ ! -d "${REPO_DIR}/.git" ]]; then
  echo "==> Cloning personal-document-extractor into ${REPO_DIR}"
  git clone "https://x-access-token:${GH_TOKEN}@github.com/kalke/personal-document-extractor.git" "${REPO_DIR}"
fi

cd "${REPO_DIR}"

if [[ ! -f prod.env ]]; then
  echo "prod.env missing in ${REPO_DIR}. Create it once on the VM before CI deploy." >&2
  exit 1
fi

echo "==> Updating to origin/${BRANCH}"
git remote set-url origin "https://x-access-token:${GH_TOKEN}@github.com/kalke/personal-document-extractor.git"
git fetch --depth=1 origin "${BRANCH}"
git checkout -B "${BRANCH}" "FETCH_HEAD"
git remote set-url origin "https://github.com/kalke/personal-document-extractor.git"

echo "==> Freeing disk before build"
docker image prune -f >/dev/null 2>&1 || true

echo "==> Building and restarting PDE"
make aws-up

echo "==> Status"
make aws-ps
