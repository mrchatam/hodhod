#!/usr/bin/env bash
# Diagnose why host curl works but Docker bridge containers cannot reach the internet.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=scripts/docker-net-common.sh
source "$ROOT/scripts/docker-net-common.sh"

hodhod_net="$(hodhod_compose_network "$ROOT")"

echo "=== Hodhod Docker egress diagnostics ==="
echo ""

echo "1) Host reachability (api.telegram.org)"
if curl -sS --max-time 8 -o /dev/null "https://api.telegram.org/bot123:fake/getMe" 2>/dev/null; then
  echo "   OK — host can reach Telegram API"
else
  echo "   FAIL — host cannot reach Telegram (fix host routing/DNS first)"
fi
echo ""

echo "2) Container on Hodhod compose network (${hodhod_net:-not found})"
if [[ -z "$hodhod_net" ]]; then
  echo "   SKIP — start hodhod-app first (docker compose up -d)"
elif docker_egress_test "$hodhod_net"; then
  echo "   OK — compose network egress works (this is what hodhod-app uses)"
  echo ""
  echo "No Docker egress issue detected for Hodhod."
  exit 0
else
  echo "   FAIL — compose network egress broken"
fi
echo ""

echo "2b) Container on default bridge (docker0 — NOT what hodhod-app uses)"
if docker_egress_test ""; then
  echo "   OK — default bridge egress works"
else
  echo "   FAIL — default bridge egress broken (may differ from compose network)"
fi
echo ""

echo "3) IPv4 forwarding"
fwd="$(sysctl -n net.ipv4.ip_forward 2>/dev/null || echo '?')"
echo "   net.ipv4.ip_forward = $fwd"
[[ "$fwd" == "1" ]] || echo "   → enable: echo 'net.ipv4.ip_forward=1' | sudo tee /etc/sysctl.d/99-docker-forward.conf && sudo sysctl --system"
echo ""

echo "4) Docker iptables integration"
if [[ -f /etc/docker/daemon.json ]] && grep -q '"iptables"[[:space:]]*:[[:space:]]*false' /etc/docker/daemon.json 2>/dev/null; then
  echo "   FAIL — /etc/docker/daemon.json has \"iptables\": false"
else
  echo "   OK — docker iptables not explicitly disabled"
fi
echo ""

vpn="$(detect_vpn_hint || true)"
if [[ -n "$vpn" ]]; then
  echo "5) VPN detected: $vpn (often breaks Docker SNAT — rules must be inserted before VPN routing)"
  echo ""
fi

echo "6) NAT MASQUERADE rules"
if command -v iptables >/dev/null 2>&1; then
  sudo iptables -t nat -S POSTROUTING 2>/dev/null | grep -i masquerade | sed 's/^/   /' || echo "   (none)"
fi
echo ""

if [[ -n "$hodhod_net" ]]; then
  docker network inspect "$hodhod_net" --format '7) Compose network {{.Name}} subnet {{range .IPAM.Config}}{{.Subnet}}{{end}}' 2>/dev/null || true
  echo ""
fi

echo "=== Fix ==="
echo "  sudo bash scripts/fix-docker-egress.sh --apply"
echo "  # re-test on compose network:"
echo "  docker run --rm --network ${hodhod_net:-hodhod_default} curlimages/curl:8.5.0 -sS --max-time 12 https://api.telegram.org/bot123:fake/getMe"
