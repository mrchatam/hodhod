#!/usr/bin/env bash
# Diagnose why host curl works but Docker bridge containers cannot reach the internet.
set -euo pipefail

echo "=== Hodhod Docker egress diagnostics ==="
echo ""

echo "1) Host reachability (api.telegram.org)"
if curl -sS --max-time 8 -o /dev/null "https://api.telegram.org/bot123:fake/getMe" 2>/dev/null; then
  echo "   OK — host can reach Telegram API"
else
  echo "   FAIL — host cannot reach Telegram (fix host routing/DNS first)"
fi
echo ""

echo "2) Container reachability (bridge network)"
if docker run --rm curlimages/curl:8.5.0 -sS --max-time 12 -o /dev/null \
  "https://api.telegram.org/bot123:fake/getMe" 2>/dev/null; then
  echo "   OK — bridge egress works"
  echo ""
  echo "No Docker egress issue detected."
  exit 0
else
  echo "   FAIL — bridge egress broken (typical: missing SNAT/MASQUERADE for container traffic)"
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
  echo "   → remove that line or set true, then: sudo systemctl restart docker"
else
  echo "   OK — docker iptables not explicitly disabled (or no daemon.json)"
fi
echo ""

echo "5) NAT MASQUERADE rules (docker should manage these)"
if command -v iptables >/dev/null 2>&1; then
  masq="$(sudo iptables -t nat -S POSTROUTING 2>/dev/null | grep -i masquerade || true)"
  if [[ -n "$masq" ]]; then
    echo "$masq" | sed 's/^/   /'
  else
    echo "   WARN — no MASQUERADE rules in nat POSTROUTING"
    echo "   → sudo systemctl restart docker"
    echo "   → ensure Docker can manage iptables (no \"iptables\": false in daemon.json)"
  fi
else
  echo "   (iptables not installed — check nftables if using nft-only firewall)"
fi
echo ""

echo "6) Compose network subnet"
net_name="$(docker network ls --format '{{.Name}}' 2>/dev/null | grep -E '^hodhod_' | head -1 || true)"
if [[ -n "$net_name" ]]; then
  docker network inspect "$net_name" --format '   network {{.Name}} subnet {{range .IPAM.Config}}{{.Subnet}}{{end}}' 2>/dev/null || true
fi
echo ""

echo "=== Fix (standard Docker — recommended) ==="
echo ""
echo "  sudo bash scripts/fix-docker-egress.sh --apply"
echo "  # or: bash install.sh → 8) Fix Docker outbound networking"
echo ""
echo "Ensure .env: HODHOD_HOST_NETWORK=0  DEPLOY_MODE=docker"
echo "Then: bash install.sh → Update"
