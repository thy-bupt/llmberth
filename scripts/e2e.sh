#!/bin/sh
# scripts/e2e.sh — local end-to-end acceptance for llmberth (plan §9.6):
#   init -> up -> create key -> chat (fake provider) -> revoke -> 401
# Usage: make e2e   (or ./scripts/e2e.sh)
set -eu

REPO="$(cd "$(dirname "$0")/.." && pwd)"
WORK="${E2E_DIR:-$(mktemp -d /tmp/llmberth-e2e.XXXXXX)}"
PROJ="$WORK/e2e-app"

echo "==> building llmberth"
(cd "$REPO" && go build -o "$WORK/llmberth" ./cmd/llmberth)

echo "==> init (provider=fake, ui=single-page) in $WORK"
(cd "$WORK" && "$WORK/llmberth" init e2e-app --provider=fake --ui=single-page --module=example.org/e2eapp)

cd "$PROJ"

echo "==> go build + test (fake provider, no network)"
go mod tidy
go build ./...
go test ./...

echo "==> compose up"
docker compose up -d --wait

cleanup() {
  echo "==> compose down"
  docker compose down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> healthz"
curl -fsS http://127.0.0.1:8080/healthz | grep -q '"ok"'

echo "==> create key via loopback admin API"
KEY_JSON=$(curl -fsS http://127.0.0.1:8090/admin/keys \
  -H "Authorization: Bearer $(cat .llmberth-admin-token)" \
  -d '{"name":"e2e"}')
FULL_KEY=$(printf '%s' "$KEY_JSON" | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')
KEY_ID=$(printf '%s' "$KEY_JSON" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
[ -n "$FULL_KEY" ] || { echo "FAIL: no key returned"; exit 1; }

echo "==> chat (non-stream, fake provider)"
curl -fsS http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $FULL_KEY" -H "Content-Type: application/json" \
  -d '{"model":"fake-chat","messages":[{"role":"user","content":"hello llmberth"}]}' \
  | grep -q '"finish_reason":"stop"'

echo "==> chat (stream)"
curl -fsS -N http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $FULL_KEY" -H "Content-Type: application/json" \
  -d '{"model":"fake-chat","messages":[{"role":"user","content":"hi"}],"stream":true}' \
  | grep -q '\[DONE\]'

echo "==> usage aggregate"
curl -fsS "http://127.0.0.1:8090/admin/usage?by=model" \
  -H "Authorization: Bearer $(cat .llmberth-admin-token)" | grep -q 'fake-chat'

echo "==> revoke -> instant 401"
curl -fsS -X POST "http://127.0.0.1:8090/admin/keys/$KEY_ID/revoke" \
  -H "Authorization: Bearer $(cat .llmberth-admin-token)" >/dev/null
CODE=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $FULL_KEY" -H "Content-Type: application/json" \
  -d '{"model":"fake-chat","messages":[{"role":"user","content":"still?"}]}')
[ "$CODE" = "401" ] || { echo "FAIL: expected 401 after revoke, got $CODE"; exit 1; }

echo "E2E PASSED ✓ (workdir kept: $WORK)"
