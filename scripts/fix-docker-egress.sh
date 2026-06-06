#!/usr/bin/env bash
# Restore Docker bridge outbound (SNAT) when host curl works but containers time out.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/docker-net-common.sh
source "$ROOT/scripts/docker-net-common.sh"

APPLY=false
NAT_ONLY=false
[[ "${1:-}" == "--apply" ]] && APPLY=true
[[ "${1:-}" == "--nat-only" ]] && { APPLY=true; NAT_ONLY=true; }

need_root() {
  if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    echo "Run as root: sudo bash scripts/fix-docker-egress.sh --apply"
    exit 1
  fi
}

hodhod_net="$(hodhod_compose_network "$ROOT")"

echo "=== Fix Docker bridge egress ==="
echo ""

if docker_egress_test "$hodhod_net"; then
  echo "OK — Hodhod compose network egress works (${hodhod_net:-default bridge})."
  exit 0
fi

if ! $APPLY; then
  bash "$ROOT/scripts/diagnose-docker-egress.sh"
  echo ""
  echo "To apply automatic fixes: sudo bash scripts/fix-docker-egress.sh --apply"
  exit 1
fi

need_root

if ! $NAT_ONLY; then
  echo "→ Enabling IPv4 forwarding..."
  sysctl -w net.ipv4.ip_forward=1
  mkdir -p /etc/sysctl.d
  echo 'net.ipv4.ip_forward=1' > /etc/sysctl.d/99-docker-forward.conf
  sysctl --system >/dev/null 2>&1 || true

  if [[ -f /etc/docker/daemon.json ]] && grep -q '"iptables"[[:space:]]*:[[:space:]]*false' /etc/docker/daemon.json 2>/dev/null; then
    echo "→ Fixing /etc/docker/daemon.json (iptables was false)..."
    cp -a /etc/docker/daemon.json "/etc/docker/daemon.json.bak.$(date +%s)"
    sed -i 's/"iptables"[[:space:]]*:[[:space:]]*false/"iptables": true/' /etc/docker/daemon.json
  fi

  echo "→ Restarting Docker..."
  systemctl restart docker
  sleep 4
  hodhod_net="$(hodhod_compose_network "$ROOT")"

  if docker_egress_test "$hodhod_net"; then
    echo "OK — egress fixed after Docker restart."
    exit 0
  fi
fi

vpn="$(detect_vpn_hint || true)"
if [[ -n "$vpn" ]]; then
  echo "→ VPN detected ($vpn) — inserting SNAT rules before VPN policy routing..."
fi

IFACE="$(default_outbound_iface)"
echo "→ Adding NAT/FORWARD rules (outbound interface: ${IFACE:-unknown})..."
apply_docker_nat_rules "$IFACE"

if docker_egress_test "$hodhod_net"; then
  echo "OK — Hodhod compose network egress fixed (${hodhod_net})."
  if command -v netfilter-persistent >/dev/null 2>&1; then
    netfilter-persistent save 2>/dev/null || true
  fi
  exit 0
fi

echo "FAIL — egress still broken on network: ${hodhod_net:-unknown}"
if [[ "$vpn" == "mullvad" ]]; then
  echo ""
  echo "Mullvad often blocks Docker SNAT. In Mullvad app: Settings → VPN settings →"
  echo "  enable Local network sharing, OR add split-tunnel exclusion for 172.16.0.0/12"
fi
echo "Run: bash scripts/diagnose-docker-egress.sh"
exit 1
