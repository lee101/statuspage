#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-$((18000 + RANDOM % 1000))}"
APP_URL="${APP_URL:-http://localhost:${PORT}}"
RESULT_DIR="${RESULT_DIR:-test-results}"
mkdir -p "${RESULT_DIR}"

if ! command -v google-chrome >/dev/null 2>&1; then
  echo "google-chrome is required for e2e tests" >&2
  exit 1
fi

go build -o "${RESULT_DIR}/statuspage-e2e" .

AWS_SMTP_USERNAME="${AWS_SMTP_USERNAME:-disabled}" \
PORT="${PORT}" \
APP_URL="${APP_URL}" \
"${RESULT_DIR}/statuspage-e2e" >"${RESULT_DIR}/server.log" 2>&1 &
server_pid=$!
trap 'kill ${server_pid} >/dev/null 2>&1 || true' EXIT

for _ in $(seq 1 40); do
  if curl -fsS "${APP_URL}/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.25
done

timeout 30s google-chrome \
  --headless=new \
  --disable-gpu \
  --no-sandbox \
  --virtual-time-budget=5000 \
  --dump-dom \
  "${APP_URL}/?test=true" >"${RESULT_DIR}/jasmine-dom.html"

if ! grep -q 'id="jasmine-result"' "${RESULT_DIR}/jasmine-dom.html"; then
  echo "Jasmine did not publish a result marker" >&2
  tail -80 "${RESULT_DIR}/server.log" >&2 || true
  exit 1
fi

if ! grep -q 'data-status="passed"' "${RESULT_DIR}/jasmine-dom.html"; then
  echo "Jasmine e2e failed" >&2
  grep 'id="jasmine-result"' "${RESULT_DIR}/jasmine-dom.html" >&2 || true
  exit 1
fi

echo "Jasmine e2e passed"
