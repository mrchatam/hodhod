#!/usr/bin/env bash
# Shared helpers for Docker egress tests (hodhod compose network, not default bridge).
set -euo pipefail

# Network name used by hodhod-app (compose project network).
hodhod_compose_network() {
  local root="${1:-.}"
  local cid net
  if [[ -f "$root/docker-compose.yml" ]]; then
    cid="$(docker compose -f "$root/docker-compose.yml" ps hodhod-app -q 2>/dev/null | head -1 || true)"
    if [[ -n "$cid" ]]; then
      net="$(docker inspect "$cid" --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}}{{"\n"}}{{end}}' 2>/dev/null | head -1 || true)"
      [[ -n "$net" ]] && { echo "$net"; return 0; }
    fi
  fi
  docker network ls --format '{{.Name}}' 2>/dev/null | grep -E '^hodhod_' | head -1 || true
}

# Outbound HTTPS test from a container on the given network (empty = default bridge).
docker_egress_test() {
  local net="${1:-}"
  local args=(docker run --rm)
  [[ -n "$net" ]] && args+=(--network "$net")
  args+=(curlimages/curl:8.5.0 -sS --max-time 12 -o /dev/null "https://api.telegram.org/bot123:fake/getMe")
  "${args[@]}" 2>/dev/null
}

default_outbound_iface() {
  ip -4 route show default 2>/dev/null | awk '{print $5; exit}'
}

detect_vpn_hint() {
  if ip link show mullvad >/dev/null 2>&1 || systemctl is-active mullvad-daemon >/dev/null 2>&1; then
    echo "mullvad"
  elif ip link show wg0 >/dev/null 2>&1; then
    echo "wireguard"
  elif ip link show tailscale0 >/dev/null 2>&1; then
    echo "tailscale"
  fi
}

# Insert SNAT + FORWARD rules so bridge containers can reach the internet (VPN-safe ordering).
apply_docker_nat_rules() {
  local iface="${1:-}"
  [[ -n "$iface" ]] || iface="$(default_outbound_iface)"
  [[ -n "$iface" ]] || return 1
  command -v iptables >/dev/null 2>&1 || return 1

  # Broad Docker RFC1918 range — insert first so VPN/nftables rules do not skip SNAT.
  if ! iptables -t nat -C POSTROUTING -s 172.16.0.0/12 ! -d 172.16.0.0/12 -o "$iface" -j MASQUERADE 2>/dev/null; then
    iptables -t nat -I POSTROUTING 1 -s 172.16.0.0/12 ! -d 172.16.0.0/12 -o "$iface" -j MASQUERADE
    echo "   Inserted: MASQUERADE 172.16.0.0/12 → $iface (priority 1)"
  fi

  # Per-network rules for compose + default bridge.
  local net subnet
  for net in bridge $(docker network ls --format '{{.Name}}' 2>/dev/null | grep -E '^hodhod_' || true); do
    [[ -n "$net" ]] || continue
    subnet="$(docker network inspect "$net" -f '{{range .IPAM.Config}}{{.Subnet}}{{end}}' 2>/dev/null || true)"
    [[ -n "$subnet" ]] || continue
    if ! iptables -t nat -C POSTROUTING -s "$subnet" -o "$iface" -j MASQUERADE 2>/dev/null; then
      iptables -t nat -I POSTROUTING 1 -s "$subnet" -o "$iface" -j MASQUERADE
      echo "   Inserted: MASQUERADE $subnet → $iface"
    fi
  done

  # Allow forwarded traffic from docker bridges to WAN.
  if iptables -nL DOCKER-USER >/dev/null 2>&1; then
    if ! iptables -C DOCKER-USER -s 172.16.0.0/12 -j ACCEPT 2>/dev/null; then
      iptables -I DOCKER-USER 1 -s 172.16.0.0/12 -j ACCEPT
      echo "   Inserted: DOCKER-USER ACCEPT 172.16.0.0/12"
    fi
  elif ! iptables -C FORWARD -s 172.16.0.0/12 -j ACCEPT 2>/dev/null; then
    iptables -I FORWARD 1 -s 172.16.0/12 -j ACCEPT
    echo "   Inserted: FORWARD ACCEPT 172.16.0.0/12"
  fi

  return 0
}
