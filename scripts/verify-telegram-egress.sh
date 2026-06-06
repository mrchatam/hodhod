#!/usr/bin/env bash
# Verify Telegram egress: SOCKS relay, .env, and container env match.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/docker-net-common.sh
source "$ROOT/scripts/docker-net-common.sh"

cd "$ROOT"
[[ -f .env ]] || { echo "FAIL — no .env"; exit 1; }

# shellcheck disable=SC1091
source .env 2>/dev/null || true
HTTP_PORT="${HTTP_PORT:-${HTTP_ADDR#:}}"
HTTP_PORT="${HTTP_PORT:-8080}"

echo "=== Telegram egress verification ==="
echo ""

echo "1) .env"
grep '^OUTBOUND_SOCKS_PROXY=' .env 2>/dev/null || echo "   OUTBOUND_SOCKS_PROXY=<empty> (direct — will fail if bridge egress broken)"
echo ""

echo "2) hodhod-app container env"
cid="$(docker compose ps hodhod-app -q 2>/dev/null | head -1 || true)"
if [[ -z "$cid" ]]; then
  echo "   FAIL — hodhod-app not running"
else
  docker inspect "$cid" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
    | grep -E '^OUTBOUND_SOCKS_PROXY=' || echo "   FAIL — OUTBOUND_SOCKS_PROXY not in container (run: docker compose up -d --force-recreate hodhod-app)"
fi
echo ""

echo "3) App startup log (proxy line)"
docker compose logs hodhod-app 2>/dev/null | grep -E 'outbound proxy|listening' | tail -5 || true
echo ""

echo "4) SOCKS relay container"
if docker ps --format '{{.Names}}' | grep -qx "${HODHOD_SOCKS_CONTAINER:-hodhod-egress-socks}"; then
  echo "   OK — ${HODHOD_SOCKS_CONTAINER:-hodhod-egress-socks} running"
  ss -tlnp 2>/dev/null | grep ":${HODHOD_SOCKS_PORT:-10810} " | sed 's/^/   /' || true
else
  echo "   FAIL — SOCKS relay not running (sudo bash scripts/setup-host-socks-relay.sh)"
fi
echo ""

echo "5) Compose network → Telegram (curl via proxy)"
net="$(hodhod_compose_network "$ROOT")"
proxy="$(socks_proxy_url "$ROOT" 2>/dev/null || echo "${OUTBOUND_SOCKS_PROXY:-}")"
if [[ -n "$net" && -n "$proxy" ]]; then
  code="$(docker run --rm --network "$net" curlimages/curl:8.5.0 -sS --max-time 20 -o /dev/null -w '%{http_code}' \
    --proxy "$proxy" "https://api.telegram.org/bot123:fake/getMe" 2>/dev/null || echo 000)"
  if [[ "$code" == "401" ]]; then
    echo "   OK — HTTP 401 via $proxy"
  else
    echo "   FAIL — HTTP $code via $proxy"
  fi
else
  echo "   SKIP — network or proxy unset"
fi
echo ""

echo "=== If container env is wrong ==="
echo "  docker compose up -d --force-recreate hodhod-app"
echo "  docker compose logs hodhod-app | grep 'outbound proxy'"
echo ""
echo "=== If all OK but bot add fails ==="
echo "  Paste the exact error from the UI"
echo "  Revoke old token in @BotFather and paste a fresh token"
