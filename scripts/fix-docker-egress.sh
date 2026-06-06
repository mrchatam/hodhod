#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/docker-net-common.sh
source "$ROOT/scripts/docker-net-common.sh"

APPLY=false
NAT_ONLY=false
[[ "${1:-}" == "--apply" ]] && APPLY=true
[[ "${1:-}" == "--nat-only" ]] && { APPLY=true; NAT_ONLY=true; }

need_root() {
  [[ "${EUID:-$(id -u)}" -eq 0 ]] || { echo "Run as root: sudo bash scripts/fix-docker-egress.sh --apply"; exit 1; }
}

hodhod_net="$(hodhod_compose_network "$ROOT")"

echo "=== Fix Docker bridge egress ==="
echo ""

if docker_egress_test "$hodhod_net"; then
  echo "OK — Hodhod compose network egress works (${hodhod_net:-default})."
  exit 0
fi

if ! $APPLY; then
  bash "$ROOT/scripts/diagnose-docker-egress.sh"
  echo ""
  echo "To apply fixes: sudo bash scripts/fix-docker-egress.sh --apply"
  exit 1
fi

need_root

if ! $NAT_ONLY; then
  echo "→ Enabling IPv4 forwarding..."
  sysctl -w net.ipv4.ip_forward=1
  mkdir -p /etc/sysctl.d
  echo 'net.ipv4.ip_forward=1' > /etc/sysctl.d/99-docker-forward.conf

  if [[ -f /etc/docker/daemon.json ]] && grep -q '"iptables"[[:space:]]*:[[:space:]]*false' /etc/docker/daemon.json 2>/dev/null; then
    cp -a /etc/docker/daemon.json "/etc/docker/daemon.json.bak.$(date +%s)"
    sed -i 's/"iptables"[[:space:]]*:[[:space:]]*false/"iptables": true/' /etc/docker/daemon.json
  fi

  echo "→ Restarting Docker..."
  systemctl restart docker
  sleep 4
  hodhod_net="$(hodhod_compose_network "$ROOT")"
fi

vpn="$(detect_vpn_hint || true)"
[[ -n "$vpn" ]] && echo "→ VPN/policy routing detected ($vpn)"

IFACE="$(default_outbound_iface)"
echo "→ Applying policy routing + NAT + FORWARD (outbound: ${IFACE:-unknown})..."
apply_all_docker_net_fixes

if docker_egress_test "$hodhod_net"; then
  echo "OK — direct bridge egress fixed (${hodhod_net})."
  exit 0
fi

echo ""
echo "Direct bridge egress still broken — starting host SOCKS relay (standard Docker, no host-network app)..."
bash "$ROOT/scripts/setup-host-socks-relay.sh"
