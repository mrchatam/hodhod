#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"
ENV_FILE="$ROOT/.env"
ENV_BACKUP="$ROOT/.env.bak.$(date +%Y%m%d%H%M%S)"

NON_INTERACTIVE=false
for arg in "$@"; do
  case "$arg" in
    --non-interactive) NON_INTERACTIVE=true ;;
    --help|-h)
      echo "Usage: bash install.sh [--non-interactive]"
      echo "Env vars for non-interactive: PUBLIC_BASE_URL, MASTER_USERNAME, MASTER_PASSWORD,"
      echo "OUTBOUND_SOCKS_PROXY, DB_PASSWORD, HTTP_PORT"
      exit 0
      ;;
  esac
done

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Error: '$1' is required but not installed." >&2
    exit 1
  fi
}

check_prereqs() {
  need_cmd docker
  need_cmd openssl
  if ! docker compose version >/dev/null 2>&1; then
    echo "Error: 'docker compose' plugin is required." >&2
    exit 1
  fi
}

validate_url() {
  local url="$1"
  [[ "$url" =~ ^https:// ]] || { echo "PUBLIC_BASE_URL must start with https://"; return 1; }
}

validate_proxy() {
  local p="$1"
  [[ -z "$p" ]] && return 0
  [[ "$p" =~ ^socks5h?:// ]] || { echo "Proxy must be socks5:// or socks5h://"; return 1; }
}

gen_secret() { openssl rand -base64 32 | tr -d '\n'; }
gen_hex() { openssl rand -hex 32; }

backup_env() {
  if [[ -f "$ENV_FILE" ]]; then
    cp "$ENV_FILE" "$ENV_BACKUP"
    echo "Backed up existing .env to $ENV_BACKUP"
  fi
}

prompt() {
  local var="$1" prompt="$2" default="${3:-}"
  if $NON_INTERACTIVE; then
    printf -v "$var" '%s' "${!var:-$default}"
    return
  fi
  if [[ -n "$default" ]]; then
    read -rp "$prompt [$default]: " val
    val="${val:-$default}"
  else
    read -rp "$prompt: " val
  fi
  printf -v "$var" '%s' "$val"
}

prompt_secret() {
  local var="$1" prompt="$2"
  if $NON_INTERACTIVE; then
    printf -v "$var" '%s' "${!var:-}"
    [[ -n "${!var}" ]] || { echo "Error: $var required in non-interactive mode"; exit 1; }
    return
  fi
  read -rsp "$prompt: " val
  echo
  read -rsp "Confirm: " val2
  echo
  if [[ "$val" != "$val2" ]]; then
    echo "Passwords do not match." >&2
    exit 1
  fi
  printf -v "$var" '%s' "$val"
}

write_env() {
  backup_env
  cat > "$ENV_FILE" <<EOF
ENV=production
HTTP_ADDR=:${HTTP_PORT}
PUBLIC_BASE_URL=${PUBLIC_BASE_URL}
DATABASE_DSN=postgres://hodhod:${DB_PASSWORD}@hodhod-db:5432/hodhod?sslmode=disable
RUN_MIGRATIONS=true
APP_ENCRYPTION_KEY=${APP_ENCRYPTION_KEY}
OUTBOUND_SOCKS_PROXY=${OUTBOUND_SOCKS_PROXY}
SESSION_SECRET=${SESSION_SECRET}
LOG_LEVEL=info
MASTER_USERNAME=${MASTER_USERNAME}
MASTER_PASSWORD=${MASTER_PASSWORD}
PANEL_POLL_WORKERS=4
EOF
  chmod 600 "$ENV_FILE"
}

wait_health() {
  echo "Waiting for Hodhod to become healthy..."
  for i in $(seq 1 60); do
    if curl -sf "http://127.0.0.1:${HTTP_PORT}/healthz" >/dev/null 2>&1; then
      echo "Hodhod is up."
      return 0
    fi
    sleep 2
  done
  echo "Warning: health check timed out. Check: docker compose logs -f hodhod-app"
  return 1
}

do_install() {
  check_prereqs
  PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-}"
  MASTER_USERNAME="${MASTER_USERNAME:-admin}"
  MASTER_PASSWORD="${MASTER_PASSWORD:-}"
  OUTBOUND_SOCKS_PROXY="${OUTBOUND_SOCKS_PROXY:-}"
  DB_PASSWORD="${DB_PASSWORD:-$(openssl rand -hex 16)}"
  HTTP_PORT="${HTTP_PORT:-8080}"

  prompt PUBLIC_BASE_URL "Public base URL (https://your.domain)"
  validate_url "$PUBLIC_BASE_URL"
  prompt MASTER_USERNAME "Master username" "$MASTER_USERNAME"
  prompt_secret MASTER_PASSWORD "Master password"
  prompt OUTBOUND_SOCKS_PROXY "Optional SOCKS proxy (empty=none)" "$OUTBOUND_SOCKS_PROXY"
  validate_proxy "$OUTBOUND_SOCKS_PROXY"
  prompt HTTP_PORT "Local HTTP port" "$HTTP_PORT"

  APP_ENCRYPTION_KEY="${APP_ENCRYPTION_KEY:-$(gen_secret)}"
  SESSION_SECRET="${SESSION_SECRET:-$(gen_hex)}"

  write_env

  if [[ -f docker-compose.yml ]]; then
    sed -i "s/POSTGRES_PASSWORD: hodhod/POSTGRES_PASSWORD: ${DB_PASSWORD}/" docker-compose.yml 2>/dev/null || true
  fi

  docker compose up -d --build
  wait_health || true

  echo ""
  echo "=== Hodhod installed ==="
  echo "Admin URL: http://127.0.0.1:${HTTP_PORT}/login"
  echo "Master user: ${MASTER_USERNAME}"
  echo ""
  echo "Host Nginx snippet (proxy to 127.0.0.1:${HTTP_PORT}):"
  cat <<NGINX

location / {
    proxy_pass http://127.0.0.1:${HTTP_PORT};
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
}
location /wh/tg/ {
    proxy_pass http://127.0.0.1:${HTTP_PORT};
    proxy_set_header Host \$host;
}
location /api/miniapp/ {
    proxy_pass http://127.0.0.1:${HTTP_PORT};
    proxy_set_header Host \$host;
}
NGINX
}

do_update() {
  check_prereqs
  docker compose up -d --build
  wait_health || true
  echo "Update complete."
}

do_remove() {
  read -rp "Remove containers and volumes? [y/N]: " ans
  if [[ "$ans" =~ ^[Yy]$ ]]; then
    docker compose down -v
    echo "Removed."
  fi
}

do_logs() {
  docker compose logs -f --tail=100 hodhod-app
}

do_status() {
  docker compose ps
  curl -sf "http://127.0.0.1:${HTTP_PORT:-8080}/healthz" && echo " healthz: ok" || echo " healthz: down"
}

do_regen_secrets() {
  check_prereqs
  [[ -f "$ENV_FILE" ]] || { echo "No .env found. Run Install first."; exit 1; }
  backup_env
  APP_ENCRYPTION_KEY=$(gen_secret)
  SESSION_SECRET=$(gen_hex)
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  write_env
  echo "Secrets regenerated. Restart the stack: docker compose up -d"
}

if $NON_INTERACTIVE; then
  do_install
  exit 0
fi

check_prereqs
echo "=== Hodhod installer ==="
echo "1) Install"
echo "2) Update"
echo "3) Remove"
echo "4) Show logs"
echo "5) Show status"
echo "6) Re-generate secrets"
read -rp "Choice [1]: " choice
choice="${choice:-1}"
case "$choice" in
  1) do_install ;;
  2) do_update ;;
  3) do_remove ;;
  4) do_logs ;;
  5) do_status ;;
  6) do_regen_secrets ;;
  *) echo "Invalid choice"; exit 1 ;;
esac
