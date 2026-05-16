#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

export PATH="/home/administrator/.bun/bin:/root/.bun/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:$PATH"

R2_ENDPOINT="${R2_ENDPOINT:-https://f76d25b8b86cfa5638f43016510d8f77.r2.cloudflarestorage.com}"
R2_BUCKET="${R2_BUCKET:-appstatic}"
R2_PREFIX="${R2_PREFIX:-statuspage}"
PUBLIC_BASE_PATH="${PUBLIC_BASE_PATH:-/$R2_PREFIX}"
BUILD_OUT_DIR="${BUILD_OUT_DIR:-dist/appstatic}"
DEPLOY_BINARY="${DEPLOY_BINARY:-.deploy/statuspage}"
STATUSPAGE_BINARY_WAS_DIRTY=0

if [[ -d ".git" ]] && ! git diff --quiet -- statuspage 2>/dev/null; then
  STATUSPAGE_BINARY_WAS_DIRTY=1
fi

if [[ -f ".env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

if [[ -z "${AWS_ACCESS_KEY_ID:-}" && -n "${CLOUDFLARE_R2_ACCESS_KEY_ID:-}" ]]; then
  export AWS_ACCESS_KEY_ID="$CLOUDFLARE_R2_ACCESS_KEY_ID"
fi
if [[ -z "${AWS_SECRET_ACCESS_KEY:-}" && -n "${CLOUDFLARE_R2_SECRET_ACCESS_KEY:-}" ]]; then
  export AWS_SECRET_ACCESS_KEY="$CLOUDFLARE_R2_SECRET_ACCESS_KEY"
fi

BUN_BIN="${BUN_BIN:-$(command -v bun)}"
AWS_BIN="${AWS_BIN:-$(command -v aws)}"

echo "== statuspage deploy =="
echo "Repo: $SCRIPT_DIR"
echo "Bucket: s3://${R2_BUCKET}/${R2_PREFIX}"
echo "URL: https://appstatic.app.nz/${R2_PREFIX}/"
echo "Bun: $BUN_BIN"
echo "AWS: $AWS_BIN"

echo
echo "Step 1: Building production app"
"$BUN_BIN" run build
mkdir -p "$(dirname "$DEPLOY_BINARY")"
cp statuspage "$DEPLOY_BINARY"
chmod +x "$DEPLOY_BINARY"

echo
echo "Step 2: Running Go tests"
go test ./...

if [[ -d ".git" && "$STATUSPAGE_BINARY_WAS_DIRTY" -eq 0 ]] && ! git diff --quiet -- statuspage 2>/dev/null; then
  git restore -- statuspage
fi

echo
echo "Step 3: Building static bucket output"
OUT_DIR="$BUILD_OUT_DIR" PUBLIC_BASE_PATH="$PUBLIC_BASE_PATH" "$BUN_BIN" run build:static

if [[ ! -f "$BUILD_OUT_DIR/index.html" ]]; then
  echo "FATAL: $BUILD_OUT_DIR/index.html missing after build" >&2
  exit 1
fi

INDEX_SIZE="$(stat -c%s "$BUILD_OUT_DIR/index.html" 2>/dev/null || stat -f%z "$BUILD_OUT_DIR/index.html")"
if [[ "$INDEX_SIZE" -lt 100 ]]; then
  echo "FATAL: $BUILD_OUT_DIR/index.html too small (${INDEX_SIZE} bytes)" >&2
  exit 1
fi

echo
echo "Step 4: Syncing static output to Cloudflare R2"
"$AWS_BIN" s3 sync "$BUILD_OUT_DIR" "s3://${R2_BUCKET}/${R2_PREFIX}" \
  --endpoint-url "$R2_ENDPOINT" \
  --delete \
  --cache-control "no-cache"

if [[ -d "$BUILD_OUT_DIR/assets" ]]; then
  "$AWS_BIN" s3 cp "$BUILD_OUT_DIR/assets" "s3://${R2_BUCKET}/${R2_PREFIX}/assets" \
    --endpoint-url "$R2_ENDPOINT" \
    --recursive \
    --cache-control "public, max-age=31536000, immutable"
fi

"$AWS_BIN" s3api put-object \
  --endpoint-url "$R2_ENDPOINT" \
  --bucket "$R2_BUCKET" \
  --key "${R2_PREFIX}/" \
  --body "$BUILD_OUT_DIR/index.html" \
  --content-type "text/html; charset=utf-8" \
  --cache-control "no-cache" >/dev/null

"$AWS_BIN" s3api put-object \
  --endpoint-url "$R2_ENDPOINT" \
  --bucket "$R2_BUCKET" \
  --key "$R2_PREFIX" \
  --body "$BUILD_OUT_DIR/index.html" \
  --content-type "text/html; charset=utf-8" \
  --cache-control "no-cache" >/dev/null

echo
echo "Deploy complete: https://appstatic.app.nz/${R2_PREFIX}/"
