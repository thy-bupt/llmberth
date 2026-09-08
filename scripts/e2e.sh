#!/bin/sh
# scripts/e2e.sh — local end-to-end acceptance for llmberth (plan §9.6):
#   init -> up -> create key -> chat (fake provider) -> revoke -> 401
# Usage: make e2e   (or ./scripts/e2e.sh)
set -eu

REPO="$(cd "$(dirname "$0")/.." && pwd)"
WORK="${E2E_DIR:-$(mktemp -d /tmp/llmberth-e2e.XXXXXX)}"
PROJ="$WORK/e2e-app"

echo "==> preflight: docker daemon + compose"
if ! docker info >/dev/null 2>&1; then
  echo "FAIL: docker daemon not reachable (OrbStack: run 'orb start'; Docker Desktop: start it)."
  exit 1
fi

echo "==> preflight: stale e2e-app containers from failed runs"
STALE=$(docker ps -aq --filter name=e2e-app- 2>/dev/null)
if [ -n "$STALE" ]; then
  echo "  removing leftover containers: $STALE"
  docker rm -f $STALE >/dev/null 2>&1 || echo "  WARNING: could not remove leftovers"
  # OrbStack/desktop daemons may hold port proxies briefly after a forced
  # removal; give them a moment so the (reused) loopback ports are free.
  sleep 3
fi

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
# --build is essential: the project name (and image tag) is stable across
# runs, so without it compose would silently reuse a stale app image.
docker compose up -d --build --wait

cleanup() {
  echo "==> compose down"
  if ! docker compose down -v --remove-orphans >/dev/null 2>&1; then
    echo "WARNING: compose down failed — leftover containers may occupy ports; run 'docker ps' and clean up before rerunning."
  fi
}
trap cleanup EXIT

echo "==> healthz"
HEALTHY=0
for i in $(seq 1 30); do
  if curl -fsS -m 2 http://127.0.0.1:8080/healthz 2>/dev/null | grep -q '"ok"'; then
    HEALTHY=1; break
  fi
  sleep 1
done
[ "$HEALTHY" = "1" ] || { echo "FAIL: app never became healthy"; exit 1; }

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

echo "==> prod profile config validity (Caddy TLS shape)"
APP_DOMAIN=api.example.test docker compose -f docker-compose.yml -f docker-compose.prod.yml config -q || { echo "FAIL: prod config invalid"; exit 1; }
echo "==> usage aggregate"
curl -fsS "http://127.0.0.1:8090/admin/usage?by=model" \
  -H "Authorization: Bearer $(cat .llmberth-admin-token)" | grep -q 'fake-chat'

echo "==> usage CLI (by model, budget line)"
CLI_USAGE=$("$WORK/llmberth" usage --by model)
echo "$CLI_USAGE" | grep -q 'fake-chat' || { echo "FAIL: usage CLI missing model row"; exit 1; }
echo "$CLI_USAGE" | grep -q 'Monthly budget' || { echo "FAIL: usage CLI missing budget line"; exit 1; }

echo "==> status budget line"
"$WORK/llmberth" status | grep -q 'budget:' || { echo "FAIL: status missing budget line"; exit 1; }

echo "==> revoke -> instant 401"
curl -fsS -X POST "http://127.0.0.1:8090/admin/keys/$KEY_ID/revoke" \
  -H "Authorization: Bearer $(cat .llmberth-admin-token)" >/dev/null
CODE=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $FULL_KEY" -H "Content-Type: application/json" \
  -d '{"model":"fake-chat","messages":[{"role":"user","content":"still?"}]}')
[ "$CODE" = "401" ] || { echo "FAIL: expected 401 after revoke, got $CODE"; exit 1; }

echo "==> 429 rate_limited (rps=1 key, burst exceeded)"
RATE_JSON=$(curl -fsS http://127.0.0.1:8090/admin/keys \
  -H "Authorization: Bearer $(cat .llmberth-admin-token)" -d '{"name":"rate","rate_limit_rps":1}')
RATE_KEY=$(printf '%s' "$RATE_JSON" | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')
RATE_ID=$(printf '%s' "$RATE_JSON" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
LIMITED=0
for i in 1 2 3 4 5 6 7 8; do
  RESP=$(curl -s http://127.0.0.1:8080/v1/chat/completions \
    -H "Authorization: Bearer $RATE_KEY" -H "Content-Type: application/json" \
    -d '{"model":"fake-chat","messages":[{"role":"user","content":"hi"}]}' || true)
  case "$RESP" in
    *rate_limited*) LIMITED=1; break ;;
  esac
done
[ "$LIMITED" = "1" ] || { echo "FAIL: never hit rate_limited"; exit 1; }
curl -fsS -X POST "http://127.0.0.1:8090/admin/keys/$RATE_ID/revoke" \
  -H "Authorization: Bearer $(cat .llmberth-admin-token)" >/dev/null

echo "==> 429 budget_exhausted (tiny per-key budget)"
BUD_JSON=$(curl -fsS http://127.0.0.1:8090/admin/keys \
  -H "Authorization: Bearer $(cat .llmberth-admin-token)" -d '{"name":"budget","budget_usd":0.0001}')
BUD_KEY=$(printf '%s' "$BUD_JSON" | sed -n 's/.*"key":"\([^"]*\)".*/\1/p')
BUD_ID=$(printf '%s' "$BUD_JSON" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
# First request passes (ledger sum still below budget) and books a cost;
# the next request must be rejected with reason=budget_exhausted.
curl -s -o /dev/null http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $BUD_KEY" -H "Content-Type: application/json" \
  -d '{"model":"fake-chat","messages":[{"role":"user","content":"spend one"}]}'
BUD_RESP=$(curl -s http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $BUD_KEY" -H "Content-Type: application/json" \
  -d '{"model":"fake-chat","messages":[{"role":"user","content":"spend two"}]}')
case "$BUD_RESP" in
  *budget_exhausted*) echo "budget exhaustion OK: $BUD_RESP" ;;
  *) echo "FAIL: expected budget_exhausted, got: $BUD_RESP"; exit 1 ;;
esac
curl -fsS -X POST "http://127.0.0.1:8090/admin/keys/$BUD_ID/revoke" \
  -H "Authorization: Bearer $(cat .llmberth-admin-token)" >/dev/null

echo "E2E PASSED ✓ (workdir kept: $WORK)"
