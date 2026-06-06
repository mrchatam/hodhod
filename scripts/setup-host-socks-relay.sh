#!/usr/bin/env bash
# Host-network SOCKS relay: bridge containers reach the internet via host egress (works when SNAT/VPN breaks direct bridge egress).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/docker-net-common.sh
source "$ROOT/scripts/docker-net-common.sh"

need_root() {
  [[ "${EUID:-$(id -u)}" -eq 0 ]] || { echo "Run as root: sudo bash $0"; exit 1; }
}

start_socks_container() {
  echo "→ Starting host-network SOCKS relay on port ${HODHOD_SOCKS_PORT}..."
  docker rm -f "$HODHOD_SOCKS_CONTAINER" 2>/dev/null || true
  docker pull serjs/go-socks5-proxy:latest
  docker run -d --name "$HODHOD_SOCKS_CONTAINER" --network host --restart unless-stopped \
    -e PROXY_PORT="$HODHOD_SOCKS_PORT" \
    -e PROXY_BINDING=0.0.0.0 \
    serjs/go-socks5-proxy:latest

  sleep 2
  if ! docker ps --format '{{.Names}}' | grep -qx "$HODHOD_SOCKS_CONTAINER"; then
    echo "FAIL — SOCKS container did not start. Check: docker logs $HODHOD_SOCKS_CONTAINER"
    exit 1
  fi

  if command -v iptables >/dev/null 2>&1; then
    if ! iptables -C INPUT -p tcp --dport "$HODHOD_SOCKS_PORT" -s 172.16.0.0/12 -j ACCEPT 2>/dev/null; then
      iptables -I INPUT 1 -p tcp --dport "$HODHOD_SOCKS_PORT" -s 172.16.0.0/12 -j ACCEPT
    fi
    if ! iptables -C INPUT -p tcp --dport "$HODHOD_SOCKS_PORT" -j DROP 2>/dev/null; then
      iptables -A INPUT -p tcp --dport "$HODHOD_SOCKS_PORT" -j DROP
    fi
    echo "   Restricted port ${HODHOD_SOCKS_PORT} to Docker subnets (172.16.0.0/12)"
  fi
}

update_env_proxy() {
  local env_file="$ROOT/.env"
  local proxy
  proxy="$(socks_proxy_url)"
  [[ -f "$env_file" ]] || { echo "No .env at $env_file"; exit 1; }
  if grep -q '^OUTBOUND_SOCKS_PROXY=' "$env_file"; then
    sed -i "s|^OUTBOUND_SOCKS_PROXY=.*|OUTBOUND_SOCKS_PROXY=${proxy}|" "$env_file"
  else
    echo "OUTBOUND_SOCKS_PROXY=${proxy}" >> "$env_file"
  fi
  grep -q '^HODHOD_HOST_NETWORK=0' "$env_file" || {
    grep -q '^HODHOD_HOST_NETWORK=' "$env_file" && sed -i 's/^HODHOD_HOST_NETWORK=.*/HODHOD_HOST_NETWORK=0/' "$env_file" || echo 'HODHOD_HOST_NETWORK=0' >> "$env_file"
  }
  echo "→ Set OUTBOUND_SOCKS_PROXY=${proxy} in .env"
}

test_socks_from_compose() {
  local net
  net="$(hodhod_compose_network "$ROOT")"
  [[ -n "$net" ]] || { echo "WARN — hodhod compose network not found"; return 1; }
  local proxy
  proxy="$(socks_proxy_url)"
  docker run --rm --network "$net" curlimages/curl:8.5.0 -sS --max-time 15 -o /dev/null \
    --proxy "$proxy" "https://api.telegram.org/bot123:fake/getMe" 2>/dev/null
}

main() {
  need_root
  echo "=== Hodhod host SOCKS relay (Docker bridge → host egress) ==="
  echo ""
  start_socks_container
  update_env_proxy

  if test_socks_from_compose; then
    echo "OK — compose network reaches Telegram via host SOCKS relay."
    echo "Run: bash install.sh → Update  (or docker compose up -d --force-recreate hodhod-app)"
    exit 0
  fi

  echo "WARN — SOCKS relay started but curl test failed."
  echo "Check: docker logs $HODHOD_SOCKS_CONTAINER"
  echo "Proxy URL: $(socks_proxy_url)"
  exit 1
}

main "$@"
