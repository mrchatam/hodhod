#!/usr/bin/env bash
# Restore Docker bridge outbound (SNAT) when host curl works but containers time out.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APPLY=false
[[ "${1:-}" == "--apply" ]] && APPLY=true

container_egress_ok() {
  docker run --rm curlimages/curl:8.5.0 -sS --max-time 12 -o /dev/null \
    "https://api.telegram.org/bot123:fake/getMe" 2>/dev/null
}

need_root() {
  if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
    echo "Run as root: sudo bash scripts/fix-docker-egress.sh --apply"
    exit 1
  fi
}

echo "=== Fix Docker bridge egress ==="
echo ""

if container_egress_ok; then
  echo "OK — container egress already works."
  exit 0
fi

if ! $APPLY; then
  bash "$ROOT/scripts/diagnose-docker-egress.sh"
  echo ""
  echo "To apply automatic fixes: sudo bash scripts/fix-docker-egress.sh --apply"
  exit 1
fi

need_root

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

if container_egress_ok; then
  echo "OK — egress fixed after Docker restart."
  exit 0
fi

echo "→ Adding NAT MASQUERADE for Hodhod Docker network..."
IFACE="$(ip -4 route show default 2>/dev/null | awk '{print $5; exit}' || true)"
NET_NAME="$(docker network ls --format '{{.Name}}' 2>/dev/null | grep -E '^hodhod_' | head -1 || true)"
if [[ -n "$NET_NAME" && -n "$IFACE" ]] && command -v iptables >/dev/null 2>&1; then
  SUBNET="$(docker network inspect "$NET_NAME" -f '{{range .IPAM.Config}}{{.Subnet}}{{end}}' 2>/dev/null || true)"
  if [[ -n "$SUBNET" ]]; then
    if ! iptables -t nat -C POSTROUTING -s "$SUBNET" -o "$IFACE" -j MASQUERADE 2>/dev/null; then
      iptables -t nat -A POSTROUTING -s "$SUBNET" -o "$IFACE" -j MASQUERADE
      echo "   Added: MASQUERADE $SUBNET → $IFACE"
    fi
  fi
fi

if container_egress_ok; then
  echo "OK — egress fixed."
  if command -v netfilter-persistent >/dev/null 2>&1; then
    netfilter-persistent save 2>/dev/null || true
  fi
  exit 0
fi

echo "FAIL — egress still broken (VPN/nftables may override Docker rules)."
echo "Run: bash scripts/diagnose-docker-egress.sh"
exit 1
