#!/usr/bin/env bash
# Diagnose blank UI / login page issues (host routing, nginx port, static assets).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
  echo "FAIL — no .env in $ROOT"
  exit 1
fi

# shellcheck disable=SC1091
source .env 2>/dev/null || true
HTTP_PORT="${HTTP_PORT:-${HTTP_ADDR#:}}"
HTTP_PORT="${HTTP_PORT:-8080}"
PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-}"
DOMAIN="${PUBLIC_BASE_URL#https://}"
DOMAIN="${DOMAIN#http://}"
DOMAIN="${DOMAIN%%/*}"
DOMAIN="${DOMAIN%%:*}"

echo "=== Hodhod UI diagnostics ==="
echo ""
echo "HTTP_PORT=${HTTP_PORT}"
echo "PUBLIC_BASE_URL=${PUBLIC_BASE_URL}"
echo "DOMAIN=${DOMAIN:-<unset>}"
echo ""

echo "1) App health (direct, no Host header)"
if curl -sf --max-time 5 "http://127.0.0.1:${HTTP_PORT}/healthz" >/dev/null; then
  echo "   OK — healthz on 127.0.0.1:${HTTP_PORT}"
else
  echo "   FAIL — nothing on 127.0.0.1:${HTTP_PORT} (check HTTP_PORT / docker compose ps)"
fi
echo ""

echo "2) Login page (direct, no Host — needs loopback or host fix)"
code="$(curl -sS --max-time 8 -o /tmp/hodhod-login.html -w '%{http_code}' "http://127.0.0.1:${HTTP_PORT}/login" 2>/dev/null || echo 000)"
bytes="$(wc -c < /tmp/hodhod-login.html 2>/dev/null || echo 0)"
echo "   HTTP ${code}, ${bytes} bytes"
if [[ "$bytes" -lt 200 ]]; then
  echo "   FAIL — login HTML too small (likely 404 host mismatch on old builds)"
  head -c 200 /tmp/hodhod-login.html 2>/dev/null | sed 's/^/   body: /'
else
  echo "   OK — login HTML looks present"
fi
echo ""

if [[ -n "$DOMAIN" ]]; then
  echo "3) Login page (with Host: ${DOMAIN})"
  code="$(curl -sS --max-time 8 -o /tmp/hodhod-login-host.html -w '%{http_code}' \
    -H "Host: ${DOMAIN}" "http://127.0.0.1:${HTTP_PORT}/login" 2>/dev/null || echo 000)"
  bytes="$(wc -c < /tmp/hodhod-login-host.html 2>/dev/null || echo 0)"
  echo "   HTTP ${code}, ${bytes} bytes"
  if [[ "$bytes" -lt 200 ]]; then
    echo "   FAIL — host routing blocked this domain"
  else
    echo "   OK"
  fi
  echo ""
fi

echo "4) Static CSS"
css_code="$(curl -sS --max-time 8 -o /tmp/hodhod.css -w '%{http_code}' \
  "http://127.0.0.1:${HTTP_PORT}/static/app.css" 2>/dev/null || echo 000)"
css_bytes="$(wc -c < /tmp/hodhod.css 2>/dev/null || echo 0)"
echo "   HTTP ${css_code}, ${css_bytes} bytes"
if [[ "$css_bytes" -lt 1000 ]]; then
  echo "   FAIL — app.css missing or empty in image"
else
  echo "   OK"
fi
echo ""

echo "5) Nginx upstream port"
if command -v nginx >/dev/null 2>&1 && [[ -d /etc/nginx/sites-enabled ]]; then
  found="$(grep -rh "127.0.0.1:" /etc/nginx/sites-enabled/ 2>/dev/null | grep -oE '127\.0\.0\.1:[0-9]+' | sort -u | tr '\n' ' ' || true)"
  if [[ -z "$found" ]]; then
    echo "   WARN — no 127.0.0.1 upstream in nginx sites-enabled"
  else
    echo "   nginx upstream(s): ${found}"
    if echo "$found" | grep -q "127.0.0.1:${HTTP_PORT}"; then
      echo "   OK — matches HTTP_PORT=${HTTP_PORT}"
    else
      echo "   FAIL — nginx port ≠ HTTP_PORT=${HTTP_PORT}"
      echo "   Fix: install.sh → option 7 (Nginx + SSL) or edit sites-enabled and reload nginx"
    fi
  fi
else
  echo "   SKIP — nginx not installed or not sites-enabled"
fi
echo ""

echo "6) PUBLIC_BASE_URL vs browser"
echo "   Open exactly: ${PUBLIC_BASE_URL}/login"
echo "   www vs non-www must match .env (or use latest build with www normalization)"
echo ""

echo "=== Quick fixes ==="
echo "  unset HODHOD_IMAGE   # if compose keeps using old image"
echo "  grep -E '^(HTTP_PORT|PUBLIC_BASE_URL|HODHOD_IMAGE)=' .env"
echo "  docker compose up -d --force-recreate hodhod-app"
echo "  bash scripts/diagnose-ui.sh"
