#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"
ENV_FILE="$ROOT/.env"
ENV_BACKUP="$ROOT/.env.bak.$(date +%Y%m%d%H%M%S)"
NGINX_TEMPLATE="$ROOT/deploy/nginx/hodhod.conf.example"
NGINX_SITE_NAME="hodhod"
NGINX_AVAILABLE="/etc/nginx/sites-available/${NGINX_SITE_NAME}.conf"
NGINX_ENABLED="/etc/nginx/sites-enabled/${NGINX_SITE_NAME}.conf"

NON_INTERACTIVE=false
for arg in "$@"; do
  case "$arg" in
    --non-interactive) NON_INTERACTIVE=true ;;
    --help|-h)
      echo "Usage: bash install.sh [--non-interactive]"
      echo ""
      echo "Non-interactive env vars:"
      echo "  PUBLIC_BASE_URL      https://your.domain (required)"
      echo "  MASTER_USERNAME      default: admin"
      echo "  MASTER_PASSWORD      required"
      echo "  OUTBOUND_SOCKS_PROXY optional socks5(h)://..."
      echo "  DB_PASSWORD          auto-generated if empty"
      echo "  HTTP_PORT            default: 8080"
      echo "  SETUP_NGINX          1 to configure host nginx + certbot SSL"
      echo "  CERTBOT_EMAIL        required when SETUP_NGINX=1"
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

maybe_sudo() {
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    echo "Error: root or sudo required for: $*" >&2
    return 1
  fi
}

check_prereqs() {
  need_cmd docker
  need_cmd openssl
  need_cmd curl
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

domain_from_url() {
  local url="$1"
  url="${url#https://}"
  url="${url#http://}"
  url="${url%%/*}"
  url="${url%%:*}"
  printf '%s' "$url"
}

gen_secret() { openssl rand -base64 32 | tr -d '\n'; }
gen_hex() { openssl rand -hex 32; }

load_env() {
  HTTP_PORT=8080
  PUBLIC_BASE_URL=""
  if [[ -f "$ENV_FILE" ]]; then
    # shellcheck disable=SC1090
    set -a
    source "$ENV_FILE"
    set +a
    HTTP_PORT="${HTTP_ADDR#:}"
    HTTP_PORT="${HTTP_PORT:-8080}"
  fi
}

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

prompt_yes_no() {
  local var="$1" prompt="$2" default="${3:-y}"
  if $NON_INTERACTIVE; then
    return 0
  fi
  local hint="Y/n"
  [[ "$default" == "n" ]] && hint="y/N"
  read -rp "$prompt [$hint]: " ans
  ans="${ans:-$default}"
  if [[ "$ans" =~ ^[Yy]$ ]]; then
    printf -v "$var" '%s' "yes"
  else
    printf -v "$var" '%s' "no"
  fi
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

install_nginx_packages() {
  if command -v nginx >/dev/null 2>&1 && command -v certbot >/dev/null 2>&1; then
    return 0
  fi
  echo "Installing nginx and certbot..."
  if command -v apt-get >/dev/null 2>&1; then
    maybe_sudo apt-get update -qq
    maybe_sudo apt-get install -y nginx certbot python3-certbot-nginx
  elif command -v dnf >/dev/null 2>&1; then
    maybe_sudo dnf install -y nginx certbot python3-certbot-nginx
  elif command -v yum >/dev/null 2>&1; then
    maybe_sudo yum install -y nginx certbot python3-certbot-nginx
  else
    echo "Error: could not install nginx/certbot automatically. Install them manually." >&2
    return 1
  fi
  maybe_sudo systemctl enable --now nginx
}

render_nginx_config() {
  local domain="$1" port="$2" dest="$3"
  [[ -f "$NGINX_TEMPLATE" ]] || { echo "Missing template: $NGINX_TEMPLATE"; return 1; }
  sed -e "s/__DOMAIN__/${domain}/g" -e "s/__HTTP_PORT__/${port}/g" "$NGINX_TEMPLATE" > "$dest"
}

setup_nginx_ssl() {
  local domain="$1"
  local email="$2"

  [[ -n "$domain" ]] || { echo "Could not parse domain from PUBLIC_BASE_URL"; return 1; }
  [[ -f "$NGINX_TEMPLATE" ]] || { echo "Missing $NGINX_TEMPLATE"; return 1; }

  echo "=== Configuring Nginx + SSL for ${domain} ==="
  install_nginx_packages

  local tmp
  tmp="$(mktemp)"
  render_nginx_config "$domain" "$HTTP_PORT" "$tmp"

  maybe_sudo mkdir -p /etc/nginx/sites-available /etc/nginx/sites-enabled
  maybe_sudo cp "$tmp" "$NGINX_AVAILABLE"
  rm -f "$tmp"
  maybe_sudo ln -sf "$NGINX_AVAILABLE" "$NGINX_ENABLED"

  # Drop default site if it conflicts on port 80
  if [[ -f /etc/nginx/sites-enabled/default ]]; then
    maybe_sudo rm -f /etc/nginx/sites-enabled/default
  fi

  maybe_sudo nginx -t
  maybe_sudo systemctl reload nginx

  echo "Requesting Let's Encrypt certificate..."
  if $NON_INTERACTIVE; then
    maybe_sudo certbot --nginx -d "$domain" -m "$email" --agree-tos --no-eff-email --redirect --non-interactive
  else
    maybe_sudo certbot --nginx -d "$domain" -m "$email" --agree-tos --no-eff-email --redirect
  fi

  maybe_sudo systemctl reload nginx
  echo "Nginx + SSL configured. Public URL: ${PUBLIC_BASE_URL}"
}

should_setup_nginx() {
  if $NON_INTERACTIVE; then
    [[ "${SETUP_NGINX:-0}" == "1" ]]
    return
  fi
  local ans=""
  prompt_yes_no ans "Configure host Nginx + Let's Encrypt SSL now?" "y"
  [[ "$ans" == "yes" ]]
}

do_install() {
  check_prereqs
  PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-}"
  MASTER_USERNAME="${MASTER_USERNAME:-admin}"
  MASTER_PASSWORD="${MASTER_PASSWORD:-}"
  OUTBOUND_SOCKS_PROXY="${OUTBOUND_SOCKS_PROXY:-}"
  DB_PASSWORD="${DB_PASSWORD:-$(openssl rand -hex 16)}"
  HTTP_PORT="${HTTP_PORT:-8080}"
  CERTBOT_EMAIL="${CERTBOT_EMAIL:-}"
  SETUP_NGINX="${SETUP_NGINX:-0}"

  prompt PUBLIC_BASE_URL "Public base URL (https://your.domain)"
  validate_url "$PUBLIC_BASE_URL"
  prompt MASTER_USERNAME "Master username" "$MASTER_USERNAME"
  prompt_secret MASTER_PASSWORD "Master password"
  prompt OUTBOUND_SOCKS_PROXY "Optional SOCKS proxy (empty=none)" "$OUTBOUND_SOCKS_PROXY"
  validate_proxy "$OUTBOUND_SOCKS_PROXY"
  prompt HTTP_PORT "Local HTTP port (Docker bind)" "$HTTP_PORT"

  APP_ENCRYPTION_KEY="${APP_ENCRYPTION_KEY:-$(gen_secret)}"
  SESSION_SECRET="${SESSION_SECRET:-$(gen_hex)}"

  write_env

  docker compose up -d --build
  wait_health || true

  DOMAIN="$(domain_from_url "$PUBLIC_BASE_URL")"

  if should_setup_nginx; then
    if [[ -z "$CERTBOT_EMAIL" ]]; then
      prompt CERTBOT_EMAIL "Email for Let's Encrypt notifications"
    fi
    [[ -n "$CERTBOT_EMAIL" ]] || { echo "CERTBOT_EMAIL is required for SSL"; exit 1; }
    setup_nginx_ssl "$DOMAIN" "$CERTBOT_EMAIL"
  else
    echo ""
    echo "Skipped Nginx/SSL. Example config: deploy/nginx/hodhod.conf.example"
    echo "Set SETUP_NGINX=1 and CERTBOT_EMAIL on a later run, or use menu option 7."
  fi

  echo ""
  echo "=== Hodhod installed ==="
  echo "Public URL: ${PUBLIC_BASE_URL}"
  echo "Local admin: http://127.0.0.1:${HTTP_PORT}/login"
  echo "Master user: ${MASTER_USERNAME}"
  echo ""
  echo "Telegram webhooks: ${PUBLIC_BASE_URL}/wh/tg/{publicID}"
  echo "Mini App: ${PUBLIC_BASE_URL}/miniapp/index.html?bot={publicID}"
}

do_update() {
  check_prereqs
  docker compose up -d --build
  load_env
  wait_health || true
  echo "Update complete."
}

do_remove() {
  read -rp "Remove containers and volumes? [y/N]: " ans
  if [[ "$ans" =~ ^[Yy]$ ]]; then
    docker compose down -v
    echo "Removed Docker stack."
  fi
  read -rp "Remove Nginx site + SSL config? [y/N]: " ans2
  if [[ "$ans2" =~ ^[Yy]$ ]]; then
    maybe_sudo rm -f "$NGINX_ENABLED" "$NGINX_AVAILABLE"
    load_env
    local domain
    domain="$(domain_from_url "${PUBLIC_BASE_URL:-}")"
    if [[ -n "$domain" ]] && command -v certbot >/dev/null 2>&1; then
      maybe_sudo certbot delete --cert-name "$domain" --non-interactive 2>/dev/null || true
    fi
    maybe_sudo nginx -t && maybe_sudo systemctl reload nginx
    echo "Removed Nginx site."
  fi
}

do_logs() {
  docker compose logs -f --tail=100 hodhod-app
}

do_status() {
  load_env
  docker compose ps
  curl -sf "http://127.0.0.1:${HTTP_PORT}/healthz" && echo " healthz: ok" || echo " healthz: down"
  if [[ -f "$NGINX_ENABLED" ]]; then
    echo "nginx site: enabled ($NGINX_ENABLED)"
  fi
}

do_regen_secrets() {
  check_prereqs
  [[ -f "$ENV_FILE" ]] || { echo "No .env found. Run Install first."; exit 1; }
  load_env
  backup_env
  APP_ENCRYPTION_KEY=$(gen_secret)
  SESSION_SECRET=$(gen_hex)
  write_env
  echo "Secrets regenerated. Restart the stack: docker compose up -d"
}

do_nginx_ssl() {
  check_prereqs
  [[ -f "$ENV_FILE" ]] || { echo "No .env found. Run Install first."; exit 1; }
  load_env
  [[ -n "$PUBLIC_BASE_URL" ]] || { echo "PUBLIC_BASE_URL missing in .env"; exit 1; }
  validate_url "$PUBLIC_BASE_URL"
  DOMAIN="$(domain_from_url "$PUBLIC_BASE_URL")"
  CERTBOT_EMAIL="${CERTBOT_EMAIL:-}"
  if [[ -z "$CERTBOT_EMAIL" ]]; then
    prompt CERTBOT_EMAIL "Email for Let's Encrypt notifications"
  fi
  setup_nginx_ssl "$DOMAIN" "$CERTBOT_EMAIL"
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
echo "7) Configure / renew Nginx + SSL"
read -rp "Choice [1]: " choice
choice="${choice:-1}"
case "$choice" in
  1) do_install ;;
  2) do_update ;;
  3) do_remove ;;
  4) do_logs ;;
  5) do_status ;;
  6) do_regen_secrets ;;
  7) do_nginx_ssl ;;
  *) echo "Invalid choice"; exit 1 ;;
esac
