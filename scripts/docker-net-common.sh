#!/usr/bin/env bash
# Shared helpers for Docker egress tests (hodhod compose network, not default bridge).
set -euo pipefail

HODHOD_SOCKS_PORT="${HODHOD_SOCKS_PORT:-10810}"
HODHOD_SOCKS_CONTAINER="${HODHOD_SOCKS_CONTAINER:-hodhod-egress-socks}"

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
  elif ip link show wg0 >/dev/null 2>&1 || ip link show 'wg-*' >/dev/null 2>&1; then
    echo "wireguard"
  elif ip link show tailscale0 >/dev/null 2>&1; then
    echo "tailscale"
  elif ip rule list 2>/dev/null | grep -qE 'fwmark|0x[0-9a-f]+'; then
    echo "policy-routing"
  fi
}

bridge_for_subnet() {
  local subnet="$1"
  ip -4 route show "$subnet" 2>/dev/null | awk '{print $3; exit}'
}

apply_docker_policy_routing() {
  if ip rule show 2>/dev/null | grep -qE 'from 172\.16\.0\.0/12.*lookup main'; then
    return 0
  fi
  ip rule add from 172.16.0.0/12 lookup main priority 50 2>/dev/null || true
  echo "   Added: ip rule from 172.16.0.0/12 lookup main (bypass VPN routing tables)"
}

apply_docker_forward_rules() {
  local iface="${1:-}"
  [[ -n "$iface" ]] || iface="$(default_outbound_iface)"
  [[ -n "$iface" ]] || return 1
  command -v iptables >/dev/null 2>&1 || return 1

  local net subnet br
  for net in bridge $(docker network ls --format '{{.Name}}' 2>/dev/null | grep -E '^hodhod_' || true); do
    [[ -n "$net" ]] || continue
    subnet="$(docker network inspect "$net" -f '{{range .IPAM.Config}}{{.Subnet}}{{end}}' 2>/dev/null || true)"
    br="$(bridge_for_subnet "$subnet")"
    [[ -n "$br" ]] || continue
    if ! iptables -C FORWARD -i "$br" -o "$iface" -j ACCEPT 2>/dev/null; then
      iptables -I FORWARD 1 -i "$br" -o "$iface" -j ACCEPT
      echo "   Inserted: FORWARD $br → $iface ACCEPT"
    fi
    if ! iptables -C FORWARD -i "$iface" -o "$br" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT 2>/dev/null; then
      iptables -I FORWARD 1 -i "$iface" -o "$br" -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
      echo "   Inserted: FORWARD $iface → $br ESTABLISHED ACCEPT"
    fi
  done
}

apply_docker_nat_rules() {
  local iface="${1:-}"
  [[ -n "$iface" ]] || iface="$(default_outbound_iface)"
  [[ -n "$iface" ]] || return 1
  command -v iptables >/dev/null 2>&1 || return 1

  if ! iptables -t nat -C POSTROUTING -s 172.16.0.0/12 ! -d 172.16.0.0/12 -o "$iface" -j MASQUERADE 2>/dev/null; then
    iptables -t nat -I POSTROUTING 1 -s 172.16.0.0/12 ! -d 172.16.0.0/12 -o "$iface" -j MASQUERADE
    echo "   Inserted: MASQUERADE 172.16.0.0/12 → $iface (priority 1)"
  fi

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

  if iptables -nL DOCKER-USER >/dev/null 2>&1; then
    if ! iptables -C DOCKER-USER -s 172.16.0.0/12 -j ACCEPT 2>/dev/null; then
      iptables -I DOCKER-USER 1 -s 172.16.0.0/12 -j ACCEPT
      echo "   Inserted: DOCKER-USER ACCEPT 172.16.0.0/12"
    fi
  elif ! iptables -C FORWARD -s 172.16.0.0/12 -j ACCEPT 2>/dev/null; then
    iptables -I FORWARD 1 -s 172.16.0.0/12 -j ACCEPT
    echo "   Inserted: FORWARD ACCEPT 172.16.0.0/12"
  fi
}

apply_all_docker_net_fixes() {
  local iface
  iface="$(default_outbound_iface)"
  apply_docker_policy_routing
  apply_docker_nat_rules "$iface"
  apply_docker_forward_rules "$iface"
}

compose_network_gateway() {
  local root="${1:-.}"
  local net
  net="$(hodhod_compose_network "$root")"
  [[ -n "$net" ]] || return 1
  docker network inspect "$net" -f '{{range .IPAM.Config}}{{.Gateway}}{{end}}' 2>/dev/null || true
}

# Prefer the compose bridge gateway (e.g. 172.18.0.1). host.docker.internal often
# resolves to docker0 (172.17.0.1), which compose containers cannot reach.
socks_proxy_url() {
  local root="${1:-.}"
  local gw
  gw="$(compose_network_gateway "$root" || true)"
  if [[ -n "$gw" ]]; then
    echo "socks5h://${gw}:${HODHOD_SOCKS_PORT}"
  else
    echo "socks5h://host.docker.internal:${HODHOD_SOCKS_PORT}"
  fi
}
