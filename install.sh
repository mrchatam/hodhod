#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"
ENV_FILE="$ROOT/.env"
ENV_BACKUP="$ROOT/.env.bak.$(date +%Y%m%d%H%M%S)"
NGINX_TEMPLATE="$ROOT/deploy/nginx/hodhod.conf.example"
SYSTEMD_TEMPLATE="$ROOT/deploy/systemd/hodhod.service"
NGINX_SITE_NAME="hodhod"
NGINX_AVAILABLE="/etc/nginx/sites-available/${NGINX_SITE_NAME}.conf"
NGINX_ENABLED="/etc/nginx/sites-enabled/${NGINX_SITE_NAME}.conf"
HODHOD_IMAGE_DEFAULT="ghcr.io/mrchatam/hodhod:latest"
HODHOD_RELEASE_BASE="https://github.com/mrchatam/hodhod/releases/latest/download"

NON_INTERACTIVE=false
DEPLOY_MODE="${DEPLOY_MODE:-docker}"   # docker | build | native
HODHOD_IMAGE="${HODHOD_IMAGE:-$HODHOD_IMAGE_DEFAULT}"
MENU_CHOICE=""
USE_COLOR=true

for arg in "$@"; do
  case "$arg" in
    --non-interactive) NON_INTERACTIVE=true ;;
    --no-color) USE_COLOR=false ;;
    --help|-h)
      cat <<'HELP'
Usage: bash install.sh [--non-interactive] [--no-color]

Deploy modes (DEPLOY_MODE):
  docker  Pull prebuilt image + Postgres in Docker (default, fastest)
  build   Build app image from source + Postgres in Docker
  native  Run hodhod binary on host + Postgres in Docker only

Non-interactive env vars:
  PUBLIC_BASE_URL       your.domain or https://your.domain
  MASTER_USERNAME       default: admin
  MASTER_PASSWORD       required
  OUTBOUND_SOCKS_PROXY  optional
  DB_PASSWORD           auto-generated if empty
  HTTP_PORT             default: 8080
  DEPLOY_MODE           docker | build | native
  HODHOD_IMAGE          override prebuilt image
  HODHOD_USE_DOCKER_MIRROR  1 to enable Arvan docker.io mirror
  HODHOD_SKIP_DOCKER_MIRROR 1 to skip mirror prompt
  HODHOD_BUILD_NO_CACHE 1 to rebuild Docker image without cache (default 0; build mode only)
  HODHOD_BUILD_LOCAL    1 to build on server (default: pull prebuilt image from GHCR)
  SETUP_NGINX           1 to configure nginx + certbot
  CERTBOT_EMAIL         required when SETUP_NGINX=1
HELP
      exit 0
      ;;
  esac
done

# ── UI helpers ──────────────────────────────────────────────────────────────
# tput queries the terminal and can hang indefinitely when TERM is missing or
# wrong (common over SSH). Never call bare tput without a timeout.
safe_tput() {
  if command -v timeout >/dev/null 2>&1; then
    timeout 1 tput "$@" 2>/dev/null || true
  else
    tput "$@" 2>/dev/null || true
  fi
}

ui_init() {
  UI_BOLD="" UI_DIM="" UI_GREEN="" UI_YELLOW="" UI_RED="" UI_CYAN="" UI_RESET=""
  if ! $USE_COLOR || [[ ! -t 1 ]]; then
    return
  fi
  # Broken TERM causes tput to block — skip colors instead of hanging.
  if [[ -z "${TERM:-}" || "${TERM:-}" == "dumb" ]]; then
    USE_COLOR=false
    return
  fi
  if ! safe_tput sgr0 >/dev/null 2>&1; then
    USE_COLOR=false
    return
  fi
  UI_BOLD="$(safe_tput bold)"
  UI_DIM="$(safe_tput dim)"
  UI_GREEN="$(safe_tput setaf 2)"
  UI_YELLOW="$(safe_tput setaf 3)"
  UI_RED="$(safe_tput setaf 1)"
  UI_CYAN="$(safe_tput setaf 6)"
  UI_RESET="$(safe_tput sgr0)"
}

ui_banner() {
  echo ""
  echo "${UI_CYAN}${UI_BOLD}╔══════════════════════════════════════════════════╗${UI_RESET}"
  echo "${UI_CYAN}${UI_BOLD}║${UI_RESET}  ${UI_BOLD}Hodhod${UI_RESET} — VPN sales bot installer              ${UI_CYAN}${UI_BOLD}║${UI_RESET}"
  echo "${UI_CYAN}${UI_BOLD}╚══════════════════════════════════════════════════╝${UI_RESET}"
  echo ""
}

ui_line() { echo "${UI_DIM}────────────────────────────────────────────────────${UI_RESET}"; }
ui_step() { echo ""; echo "${UI_CYAN}${UI_BOLD}▸ Step $1/$2${UI_RESET}  $3"; ui_line; }
ui_ok()   { echo "  ${UI_GREEN}✔${UI_RESET} $*"; }
ui_warn() { echo "  ${UI_YELLOW}!${UI_RESET} $*"; }
ui_err()  { echo "  ✖ $*"; }
ui_hint() { echo "  ${UI_DIM}→ $*${UI_RESET}"; }

# Read from controlling terminal — SSH often has stdout tty but not stdin.
safe_read() {
  local var="$1"
  local line=""
  if [[ -r /dev/tty ]]; then
    IFS= read -r line </dev/tty || return 1
  else
    IFS= read -r line || return 1
  fi
  printf -v "$var" '%s' "$line"
}

safe_read_secret() {
  local var="$1"
  local line=""
  if [[ -r /dev/tty ]]; then
    IFS= read -rs line </dev/tty || return 1
  else
    IFS= read -rs line || return 1
  fi
  echo ""
  printf -v "$var" '%s' "$line"
}

# ── prerequisites ─────────────────────────────────────────────────────────────
need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    ui_err "'$1' is required but not installed."
    return 1
  fi
}

maybe_sudo() {
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    ui_err "root or sudo required for: $*"
    return 1
  fi
}

docker_compose_ok() {
  if command -v timeout >/dev/null 2>&1; then
    timeout 15 docker compose version >/dev/null 2>&1
  else
    docker compose version >/dev/null 2>&1
  fi
}

check_prereqs() {
  local ok=true
  need_cmd openssl || ok=false
  need_cmd curl    || ok=false
  if [[ "$DEPLOY_MODE" != "native" ]]; then
    need_cmd docker || ok=false
    docker_compose_ok || { ui_err "'docker compose' plugin is required (or Docker daemon not responding)."; ok=false; }
  else
    need_cmd docker || ok=false
    docker_compose_ok || { ui_err "'docker compose' plugin is required for Postgres (or Docker daemon not responding)."; ok=false; }
  fi
  if [[ "$ok" != true ]]; then
    return 1
  fi
}

# ── input normalization (fix user input instead of rejecting) ─────────────────
normalize_public_url() {
  local raw="$1"
  raw="${raw#"${raw%%[![:space:]]*}"}"
  raw="${raw%"${raw##*[![:space:]]}"}"
  raw="${raw%/}"
  # bare domain → https://domain
  if [[ "$raw" != *"://"* ]]; then
    raw="https://${raw}"
  fi
  # http → https (Telegram webhooks need HTTPS)
  if [[ "$raw" == http://* ]]; then
    ui_hint "Upgraded http:// to https://"
    raw="https://${raw#http://}"
  fi
  printf '%s' "$raw"
}

normalize_proxy() {
  local raw="$1"
  raw="${raw#"${raw%%[![:space:]]*}"}"
  raw="${raw%"${raw##*[![:space:]]}"}"
  [[ -z "$raw" ]] && { printf '%s' ""; return; }
  if [[ "$raw" != *"://"* ]]; then
    raw="socks5h://${raw}"
    ui_hint "Added socks5h:// prefix to proxy."
  fi
  printf '%s' "$raw"
}

normalize_email() {
  local raw="$1"
  raw="${raw#"${raw%%[![:space:]]*}"}"
  raw="${raw%"${raw##*[![:space:]]}"}"
  printf '%s' "$raw"
}

domain_from_url() {
  local url="$1"
  url="${url#https://}"
  url="${url#http://}"
  url="${url%%/*}"
  url="${url%%:*}"
  printf '%s' "$url"
}

is_valid_domain() {
  [[ "$1" =~ ^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$ ]]
}

is_valid_email() {
  [[ "$1" =~ ^[^@]+@[^@]+\.[^@]+$ ]]
}

is_valid_port() {
  [[ "$1" =~ ^[0-9]+$ ]] && (( 10#"$1" >= 1 && 10#"$1" <= 65535 ))
}

gen_secret() { openssl rand -base64 32 | tr -d '\n'; }
gen_hex()    { openssl rand -hex 32; }

load_env() {
  HTTP_PORT=8080
  PUBLIC_BASE_URL=""
  DEPLOY_MODE="docker"
  DB_PASSWORD="hodhod"
  if [[ -f "$ENV_FILE" ]]; then
    # shellcheck disable=SC1090
    set -a; source "$ENV_FILE"; set +a
    HTTP_PORT="${HTTP_ADDR#:}"
    HTTP_PORT="${HTTP_PORT:-8080}"
    DEPLOY_MODE="${DEPLOY_MODE:-docker}"
  fi
}

backup_env() {
  if [[ -f "$ENV_FILE" ]]; then
    cp "$ENV_FILE" "$ENV_BACKUP"
    ui_hint "Backed up .env → $(basename "$ENV_BACKUP")"
  fi
}

# ── graceful prompts (retry on mistake, never hard-exit in interactive) ───────
prompt_loop() {
  local var="$1" label="$2" default="${3:-}"
  local val hint=""
  while true; do
    if $NON_INTERACTIVE; then
      printf -v "$var" '%s' "${!var:-$default}"
      return 0
    fi
    if [[ -n "$default" ]]; then
      echo -n "  ${UI_BOLD}${label}${UI_RESET} ${UI_DIM}[${default}]${UI_RESET}: "
      safe_read val || { ui_err "Could not read input."; return 1; }
      val="${val:-$default}"
    else
      echo -n "  ${UI_BOLD}${label}${UI_RESET}: "
      safe_read val || { ui_err "Could not read input."; return 1; }
      val="${val:-$default}"
    fi
    printf -v "$var" '%s' "$val"
    return 0
  done
}

prompt_url() {
  local var="$1" label="$2" default="${3:-}"
  local raw norm domain
  while true; do
    prompt_loop raw "$label" "$default"
    norm="$(normalize_public_url "$raw")"
    domain="$(domain_from_url "$norm")"
    if is_valid_domain "$domain"; then
      if [[ "$norm" != "$raw" ]]; then
        ui_hint "Saved as: ${norm}"
      fi
      printf -v "$var" '%s' "$norm"
      return 0
    fi
    ui_err "Could not parse a valid domain. Try: your.domain or https://your.domain"
  done
}

prompt_proxy() {
  local var="$1" label="$2" default="${3:-}"
  local raw norm
  while true; do
    prompt_loop raw "$label" "$default"
    norm="$(normalize_proxy "$raw")"
    if [[ -z "$norm" ]] || [[ "$norm" =~ ^socks5h?:// ]]; then
      printf -v "$var" '%s' "$norm"
      return 0
    fi
    ui_err "Proxy should look like socks5h://host:port (or leave empty)."
  done
}

prompt_port() {
  local var="$1" label="$2" default="${3:-8080}"
  local raw
  while true; do
    prompt_loop raw "$label" "$default"
    if is_valid_port "$raw"; then
      printf -v "$var" '%s' "$raw"
      return 0
    fi
    ui_err "Port must be a number between 1 and 65535."
  done
}

prompt_email() {
  local var="$1" label="$2"
  local raw norm
  while true; do
    if $NON_INTERACTIVE; then
      printf -v "$var" '%s' "${!var:-}"
      [[ -n "${!var}" ]] && is_valid_email "${!var}" && return 0
      ui_err "CERTBOT_EMAIL required in non-interactive mode."; exit 1
    fi
    echo -n "  ${UI_BOLD}${label}${UI_RESET}: "
    safe_read raw || { ui_err "Could not read input."; return 1; }
    norm="$(normalize_email "$raw")"
    if is_valid_email "$norm"; then
      printf -v "$var" '%s' "$norm"
      return 0
    fi
    ui_err "That doesn't look like an email address. Try again."
  done
}

prompt_secret() {
  local var="$1" label="$2"
  local val val2
  while true; do
    if $NON_INTERACTIVE; then
      printf -v "$var" '%s' "${!var:-}"
      [[ -n "${!var}" ]] || { ui_err "MASTER_PASSWORD required in non-interactive mode."; exit 1; }
      return 0
    fi
    echo -n "  ${UI_BOLD}${label}${UI_RESET}: "
    safe_read_secret val || { ui_err "Could not read input."; return 1; }
    echo -n "  ${UI_BOLD}Confirm password${UI_RESET}: "
    safe_read_secret val2 || { ui_err "Could not read input."; return 1; }
    if [[ "$val" == "$val2" && -n "$val" ]]; then
      printf -v "$var" '%s' "$val"
      return 0
    fi
    ui_err "Passwords empty or don't match. Try again."
  done
}

prompt_yes_no() {
  local var="$1" label="$2" default="${3:-y}"
  if $NON_INTERACTIVE; then
    printf -v "$var" '%s' "${default}"
    return 0
  fi
  local hint="Y/n"; [[ "$default" == "n" ]] && hint="y/N"
  local ans
  while true; do
    echo -n "  ${UI_BOLD}${label}${UI_RESET} ${UI_DIM}[${hint}]${UI_RESET}: "
    safe_read ans || { ui_err "Could not read input."; return 1; }
    ans="${ans:-$default}"
    case "$ans" in
      [Yy]*) printf -v "$var" '%s' "yes"; return 0 ;;
      [Nn]*) printf -v "$var" '%s' "no";  return 0 ;;
      *) ui_err "Please answer y or n." ;;
    esac
  done
}

choose_deploy_mode() {
  if $NON_INTERACTIVE; then
    DEPLOY_MODE="${DEPLOY_MODE:-docker}"
    return 0
  fi
  echo ""
  ui_line
  echo "  ${UI_BOLD}How should Hodhod run?${UI_RESET}"
  echo "    ${UI_CYAN}1)${UI_RESET} Docker — pull prebuilt image ${UI_DIM}(fastest, recommended)${UI_RESET}"
  echo "    ${UI_CYAN}2)${UI_RESET} Docker — build from source ${UI_DIM}(dev only; set HODHOD_BUILD_LOCAL=1 on Update)${UI_RESET}"
  echo "    ${UI_CYAN}3)${UI_RESET} Native binary on host + Postgres in Docker"
  echo "       ${UI_DIM}(no app container; still needs Docker for the database)${UI_RESET}"
  ui_line
  local choice
  while true; do
    echo -n "  ${UI_BOLD}Choice${UI_RESET} ${UI_DIM}[1]${UI_RESET}: "
    safe_read choice || { ui_err "Could not read input."; return 1; }
    choice="${choice:-1}"
    case "$choice" in
      1) DEPLOY_MODE=docker; ui_ok "Docker (prebuilt image)"; return 0 ;;
      2) DEPLOY_MODE=build;  ui_ok "Docker (build from source)"; return 0 ;;
      3) DEPLOY_MODE=native; ui_ok "Native binary + Docker Postgres"; return 0 ;;
      *) ui_err "Pick 1, 2, or 3." ;;
    esac
  done
}

choose_menu() {
  local choice
  ui_banner
  echo "  ${UI_BOLD}What would you like to do?${UI_RESET}"
  echo ""
  echo "    ${UI_CYAN}1)${UI_RESET} Install"
  echo "    ${UI_CYAN}2)${UI_RESET} Update"
  echo "    ${UI_CYAN}3)${UI_RESET} Remove"
  echo "    ${UI_CYAN}4)${UI_RESET} Show logs"
  echo "    ${UI_CYAN}5)${UI_RESET} Show status"
  echo "    ${UI_CYAN}6)${UI_RESET} Re-generate secrets"
  echo "    ${UI_CYAN}7)${UI_RESET} Configure / renew Nginx + SSL"
  echo ""
  while true; do
    echo -n "  ${UI_BOLD}Choice${UI_RESET} ${UI_DIM}[1]${UI_RESET}: "
    safe_read choice || { ui_err "Could not read input. Try: bash install.sh --non-interactive"; return 1; }
    choice="${choice:-1}"
    case "$choice" in
      1|2|3|4|5|6|7) MENU_CHOICE="$choice"; return 0 ;;
      *) ui_err "Pick a number from 1 to 7." ;;
    esac
  done
}

# ── env / stack ───────────────────────────────────────────────────────────────
write_env() {
  backup_env
  local dsn
  if [[ "$DEPLOY_MODE" == "native" ]]; then
    dsn="postgres://hodhod:${DB_PASSWORD}@127.0.0.1:5432/hodhod?sslmode=disable"
  else
    dsn="postgres://hodhod:${DB_PASSWORD}@hodhod-db:5432/hodhod?sslmode=disable"
  fi
  cat > "$ENV_FILE" <<EOF
ENV=production
DEPLOY_MODE=${DEPLOY_MODE}
HTTP_ADDR=:${HTTP_PORT}
PUBLIC_BASE_URL=${PUBLIC_BASE_URL}
DATABASE_DSN=${dsn}
DB_PASSWORD=${DB_PASSWORD}
RUN_MIGRATIONS=true
APP_ENCRYPTION_KEY=${APP_ENCRYPTION_KEY}
OUTBOUND_SOCKS_PROXY=${OUTBOUND_SOCKS_PROXY}
SESSION_SECRET=${SESSION_SECRET}
LOG_LEVEL=info
MASTER_USERNAME=${MASTER_USERNAME}
MASTER_PASSWORD=${MASTER_PASSWORD}
PANEL_POLL_WORKERS=4
HODHOD_IMAGE=${HODHOD_IMAGE}
EOF
  chmod 600 "$ENV_FILE"
}

# ── Docker registry mirror (optional, helps docker.io in Iran) ────────────────
prompt_docker_registry_mirror() {
  if [[ "${HODHOD_SKIP_DOCKER_MIRROR:-0}" == "1" ]]; then
    HODHOD_USE_DOCKER_MIRROR=0
    ui_hint "Docker mirror skipped (HODHOD_SKIP_DOCKER_MIRROR=1)."
    return 0
  fi
  if [[ "${HODHOD_USE_DOCKER_MIRROR:-}" == "1" ]]; then
    ui_hint "Docker mirror enabled via HODHOD_USE_DOCKER_MIRROR=1."
    return 0
  fi
  if [[ "${HODHOD_USE_DOCKER_MIRROR:-}" == "0" ]]; then
    ui_hint "Docker mirror disabled via HODHOD_USE_DOCKER_MIRROR=0."
    return 0
  fi
  if $NON_INTERACTIVE; then
    HODHOD_USE_DOCKER_MIRROR=0
    return 0
  fi
  echo ""
  ui_line
  echo "  ${UI_BOLD}Docker registry mirror (optional)${UI_RESET}"
  ui_hint "Arvan Cloud mirror can speed up docker.io pulls in Iran."
  ui_hint "GHCR (ghcr.io) is not mirrored — use native mode if GHCR is blocked."
  local mirror_choice
  prompt_yes_no mirror_choice "Use Arvan Cloud Docker registry mirror?" "n"
  if [[ "$mirror_choice" == "yes" ]]; then
    HODHOD_USE_DOCKER_MIRROR=1
    ui_ok "Docker mirror enabled."
  else
    HODHOD_USE_DOCKER_MIRROR=0
    ui_hint "Using default Docker registry (no mirror)."
  fi
}

configure_docker_registry_mirrors() {
  prompt_docker_registry_mirror
  [[ "${HODHOD_USE_DOCKER_MIRROR:-0}" == "1" ]] || return 0

  ui_hint "Configuring Docker registry mirrors (Arvan Cloud)..."
  maybe_sudo mkdir -p /etc/docker

  local mirror_backup=""
  if [[ -f /etc/docker/daemon.json ]]; then
    mirror_backup="/etc/docker/daemon.json.bak.$(date +%s)"
    maybe_sudo cp /etc/docker/daemon.json "$mirror_backup" || true
  fi

  maybe_sudo bash -c 'cat > /etc/docker/daemon.json <<EOF
{
  "insecure-registries": ["https://docker.arvancloud.ir"],
  "registry-mirrors": ["https://docker.arvancloud.ir"]
}
EOF'

  docker logout >/dev/null 2>&1 || true
  if ! maybe_sudo systemctl restart docker; then
    ui_warn "Failed to restart Docker after mirror setup."
    if [[ -n "${mirror_backup:-}" && -f "${mirror_backup}" ]]; then
      maybe_sudo cp "${mirror_backup}" /etc/docker/daemon.json
      maybe_sudo systemctl restart docker 2>/dev/null || true
    fi
    return 0
  fi
  sleep 2
  if ! docker info >/dev/null 2>&1; then
    ui_warn "Docker not running after mirror configuration — restoring previous config."
    if [[ -n "${mirror_backup:-}" && -f "${mirror_backup}" ]]; then
      maybe_sudo cp "${mirror_backup}" /etc/docker/daemon.json
      maybe_sudo systemctl restart docker 2>/dev/null || true
    fi
    return 0
  fi
  ui_ok "Docker registry mirrors configured."
}

pull_hodhod_image() {
  export DB_PASSWORD HTTP_PORT HODHOD_IMAGE
  ui_hint "Pulling ${HODHOD_IMAGE} ..."
  if command -v timeout >/dev/null 2>&1; then
    timeout 600 docker compose pull hodhod-app
  else
    docker compose pull hodhod-app
  fi
  docker image inspect "$HODHOD_IMAGE" >/dev/null 2>&1
}

handle_image_unavailable() {
  ui_warn "Prebuilt image unavailable or GHCR unreachable: ${HODHOD_IMAGE}"
  ui_hint "Publish with: git tag v0.1.0 && git push origin v0.1.0"
  if $NON_INTERACTIVE; then
    ui_err "Pull failed. Set DEPLOY_MODE=native or ensure the image is pullable."
    return 1
  fi
  echo ""
  echo "  ${UI_BOLD}What next?${UI_RESET}"
  echo "    ${UI_CYAN}1)${UI_RESET} Retry pull"
  echo "    ${UI_CYAN}2)${UI_RESET} Switch to native mode (binary + Postgres container)"
  echo "    ${UI_CYAN}3)${UI_RESET} Build from source ${UI_DIM}(slow; may fail on restricted networks)${UI_RESET}"
  local choice
  while true; do
    echo -n "  ${UI_BOLD}Choice${UI_RESET} ${UI_DIM}[1]${UI_RESET}: "
    safe_read choice || { ui_err "Could not read input."; return 1; }
    choice="${choice:-1}"
    case "$choice" in
      1)
        ui_hint "Retrying pull..."
        if pull_hodhod_image; then
          compose_pull_and_recreate_app
          return 0
        fi
        ui_err "Pull still failed."
        ;;
      2)
        DEPLOY_MODE=native
        write_env
        start_native
        return 0
        ;;
      3)
        ui_warn "Building locally — requires network access to Debian/Alpine package mirrors."
        DEPLOY_MODE=build
        write_env
        compose_rebuild_app 0
        return 0
        ;;
      *) ui_err "Pick 1, 2, or 3." ;;
    esac
  done
}

wait_health() {
  local target="http://127.0.0.1:${HTTP_PORT}/healthz"
  ui_hint "Waiting for Hodhod at ${target} ..."
  local i
  for i in $(seq 1 45); do
    if curl -sf "$target" >/dev/null 2>&1; then
      ui_ok "Hodhod is healthy."
      return 0
    fi
    printf "  ${UI_DIM}%s${UI_RESET}\r" "$(printf '%*s' "$i" '' | tr ' ' '.')"
    sleep 2
  done
  echo ""
  ui_warn "Health check timed out. Run: docker compose logs -f hodhod-app"
  return 1
}

compose_up_db() {
  export DB_PASSWORD HTTP_PORT
  if [[ "$DEPLOY_MODE" == "native" ]]; then
    docker compose -f docker-compose.yml -f docker-compose.native.yml up -d hodhod-db
  else
    docker compose up -d hodhod-db
  fi
}

# Compose file args for hodhod-app (build overlay adds Dockerfile build context).
compose_app_files() {
  COMPOSE_APP_FILES=(-f docker-compose.yml)
  if [[ "$DEPLOY_MODE" == "build" ]]; then
    COMPOSE_APP_FILES+=(-f docker-compose.build.yml)
  fi
}

# Rebuild from source and replace the running app container (database volume untouched).
compose_rebuild_app() {
  local no_cache="${1:-0}"
  compose_app_files
  if [[ "$no_cache" == "1" ]]; then
    ui_hint "Building image without cache (this may take a few minutes)..."
    ui_warn "Local builds need network access to package registries."
    docker compose "${COMPOSE_APP_FILES[@]}" build --pull --no-cache hodhod-app
    docker compose "${COMPOSE_APP_FILES[@]}" up -d --force-recreate --no-deps hodhod-app
  else
    ui_hint "Rebuilding image and recreating app container..."
    docker compose "${COMPOSE_APP_FILES[@]}" up -d --build --force-recreate --no-deps hodhod-app
  fi
}

# Pull prebuilt image and replace the running app container (database volume untouched).
compose_pull_and_recreate_app() {
  ui_hint "Pulling prebuilt image ${HODHOD_IMAGE} (built by GitHub Actions)..."
  pull_hodhod_image || return 1
  ui_hint "Recreating app container with pulled image..."
  docker compose up -d --pull always --force-recreate --no-deps hodhod-app
}

# Update uses GHCR by default; local build only when DEPLOY_MODE=build and HODHOD_BUILD_LOCAL=1.
ensure_pull_deploy_mode() {
  if [[ "$DEPLOY_MODE" == "build" && "${HODHOD_BUILD_LOCAL:-0}" != "1" ]]; then
    ui_hint "Using prebuilt image (set DEPLOY_MODE=build and HODHOD_BUILD_LOCAL=1 to compile on server)."
    DEPLOY_MODE=docker
  fi
}

compose_up_build_app() {
  compose_rebuild_app 0
}

start_docker_app() {
  export DB_PASSWORD HTTP_PORT HODHOD_IMAGE
  if [[ "$DEPLOY_MODE" == "build" ]]; then
    compose_rebuild_app 0
    return
  fi
  ui_hint "Pulling prebuilt image: ${HODHOD_IMAGE}"
  if pull_hodhod_image; then
    docker compose up -d --pull always --force-recreate --no-deps hodhod-app
    return
  fi
  handle_image_unavailable
}

install_native_binary() {
  local force="${1:-0}"
  local bindir="$ROOT/bin"
  mkdir -p "$bindir"
  local bin="$bindir/hodhod"
  if [[ -f "$bin" && -x "$bin" && "$force" != "1" ]]; then
    ui_ok "Binary already present: $bin"
    return 0
  fi
  ui_hint "Downloading release binary..."
  local tmp
  tmp="$(mktemp -d)"
  if curl -fsSL "${HODHOD_RELEASE_BASE}/hodhod-linux-amd64.tar.gz" -o "$tmp/hodhod.tar.gz" 2>/dev/null; then
    tar xzf "$tmp/hodhod.tar.gz" -C "$tmp"
    install -m 755 "$tmp/hodhod-linux-amd64" "$bin"
    rm -rf "$tmp"
    ui_ok "Installed binary from GitHub release."
    return 0
  fi
  rm -rf "$tmp"
  if command -v go >/dev/null 2>&1; then
    ui_hint "Release not found — compiling locally with Go..."
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "$bin" ./cmd/server
    ui_ok "Built binary locally."
    return 0
  fi
  ui_err "No release binary and Go not installed. Use DEPLOY_MODE=docker or install Go."
  return 1
}

install_systemd_unit() {
  local unit="/etc/systemd/system/hodhod.service"
  local run_user; run_user="$(whoami)"
  local tmp
  tmp="$(mktemp)"
  sed -e "s|__INSTALL_DIR__|${ROOT}|g" -e "s|__RUN_USER__|${run_user}|g" "$SYSTEMD_TEMPLATE" > "$tmp"
  maybe_sudo cp "$tmp" "$unit"
  rm -f "$tmp"
  maybe_sudo systemctl daemon-reload
  maybe_sudo systemctl enable hodhod
  maybe_sudo systemctl restart hodhod
  ui_ok "systemd service enabled (hodhod.service)"
}

start_native() {
  compose_up_db
  install_native_binary || return 1
  install_systemd_unit
  ui_hint "Migrations run automatically on first service start."
}

start_stack() {
  if [[ "$DEPLOY_MODE" == "native" ]]; then
    start_native
  else
    compose_up_db
    start_docker_app
  fi
}

# ── nginx / ssl ───────────────────────────────────────────────────────────────
install_nginx_packages() {
  if command -v nginx >/dev/null 2>&1 && command -v certbot >/dev/null 2>&1; then
    return 0
  fi
  ui_hint "Installing nginx + certbot..."
  if command -v apt-get >/dev/null 2>&1; then
    maybe_sudo apt-get update -qq
    maybe_sudo apt-get install -y nginx certbot python3-certbot-nginx
  elif command -v dnf >/dev/null 2>&1; then
    maybe_sudo dnf install -y nginx certbot python3-certbot-nginx
  elif command -v yum >/dev/null 2>&1; then
    maybe_sudo yum install -y nginx certbot python3-certbot-nginx
  else
    ui_err "Could not install nginx/certbot automatically."
    return 1
  fi
  maybe_sudo systemctl enable --now nginx
}

render_nginx_config() {
  local domain="$1" port="$2" dest="$3"
  sed -e "s/__DOMAIN__/${domain}/g" -e "s/__HTTP_PORT__/${port}/g" "$NGINX_TEMPLATE" > "$dest"
}

setup_nginx_ssl() {
  local domain="$1" email="$2"
  [[ -f "$NGINX_TEMPLATE" ]] || { ui_err "Missing $NGINX_TEMPLATE"; return 1; }

  ui_step "SSL" 1 "Configuring Nginx + Let's Encrypt for ${domain}"
  install_nginx_packages || return 1

  local tmp; tmp="$(mktemp)"
  render_nginx_config "$domain" "$HTTP_PORT" "$tmp"
  maybe_sudo mkdir -p /etc/nginx/sites-available /etc/nginx/sites-enabled
  maybe_sudo cp "$tmp" "$NGINX_AVAILABLE"
  rm -f "$tmp"
  maybe_sudo ln -sf "$NGINX_AVAILABLE" "$NGINX_ENABLED"
  [[ -f /etc/nginx/sites-enabled/default ]] && maybe_sudo rm -f /etc/nginx/sites-enabled/default

  maybe_sudo nginx -t
  maybe_sudo systemctl reload nginx

  ui_hint "Requesting certificate from Let's Encrypt..."
  if $NON_INTERACTIVE; then
    maybe_sudo certbot --nginx -d "$domain" -m "$email" --agree-tos --no-eff-email --redirect --non-interactive
  else
    maybe_sudo certbot --nginx -d "$domain" -m "$email" --agree-tos --no-eff-email --redirect
  fi
  maybe_sudo systemctl reload nginx
  ui_ok "HTTPS ready: https://${domain}"
}

# ── actions ───────────────────────────────────────────────────────────────────
do_install() {
  check_prereqs || return 1
  ui_banner

  PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-}"
  MASTER_USERNAME="${MASTER_USERNAME:-admin}"
  MASTER_PASSWORD="${MASTER_PASSWORD:-}"
  OUTBOUND_SOCKS_PROXY="${OUTBOUND_SOCKS_PROXY:-}"
  DB_PASSWORD="${DB_PASSWORD:-$(openssl rand -hex 16)}"
  HTTP_PORT="${HTTP_PORT:-8080}"
  CERTBOT_EMAIL="${CERTBOT_EMAIL:-}"
  SETUP_NGINX="${SETUP_NGINX:-0}"

  ui_step 1 4 "Deploy method"
  choose_deploy_mode

  ui_step 2 4 "Configuration"
  prompt_url        PUBLIC_BASE_URL "Public URL (domain or https://domain)"
  prompt_loop       MASTER_USERNAME "Master username" "$MASTER_USERNAME"
  prompt_secret     MASTER_PASSWORD "Master password"
  prompt_proxy      OUTBOUND_SOCKS_PROXY "SOCKS proxy (optional, empty to skip)" ""
  prompt_port       HTTP_PORT "Local app port" "$HTTP_PORT"

  APP_ENCRYPTION_KEY="${APP_ENCRYPTION_KEY:-$(gen_secret)}"
  SESSION_SECRET="${SESSION_SECRET:-$(gen_hex)}"

  write_env

  ui_step 3 4 "Starting services"
  configure_docker_registry_mirrors
  start_stack
  wait_health || true

  DOMAIN="$(domain_from_url "$PUBLIC_BASE_URL")"
  local setup_nginx_ans=""
  if $NON_INTERACTIVE; then
    [[ "${SETUP_NGINX:-0}" == "1" ]] && setup_nginx_ans=yes || setup_nginx_ans=no
  else
    ui_step 4 4 "TLS / Nginx"
    prompt_yes_no setup_nginx_ans "Configure host Nginx + Let's Encrypt SSL?" "y"
  fi

  if [[ "$setup_nginx_ans" == "yes" ]]; then
    prompt_email CERTBOT_EMAIL "Let's Encrypt email"
    setup_nginx_ssl "$DOMAIN" "$CERTBOT_EMAIL" || ui_warn "Nginx/SSL setup failed — re-run option 7 later."
  else
    ui_hint "Skipped Nginx. Example: deploy/nginx/hodhod.conf.example (menu option 7 later)."
  fi

  echo ""
  ui_line
  echo "  ${UI_GREEN}${UI_BOLD}Installation complete${UI_RESET}"
  ui_line
  echo ""
  ui_ok "Public URL   ${PUBLIC_BASE_URL}"
  ui_ok "Admin login  ${PUBLIC_BASE_URL}/login  (or http://127.0.0.1:${HTTP_PORT}/login)"
  ui_ok "Master user  ${MASTER_USERNAME}"
  ui_ok "Deploy mode  ${DEPLOY_MODE}"
  echo ""
  ui_hint "Webhooks  ${PUBLIC_BASE_URL}/wh/tg/{publicID}"
  ui_hint "Mini App  ${PUBLIC_BASE_URL}/miniapp/index.html?bot={publicID}"
  echo ""
}

do_update() {
  check_prereqs || return 1
  load_env
  export DB_PASSWORD HTTP_PORT HODHOD_IMAGE
  local build_no_cache="${HODHOD_BUILD_NO_CACHE:-0}"

  if [[ -d "$ROOT/.git" ]] && command -v git >/dev/null 2>&1; then
    ui_hint "Pulling install/compose updates from git (app comes from ${HODHOD_IMAGE:-$HODHOD_IMAGE_DEFAULT})..."
    if ! git -C "$ROOT" pull --ff-only 2>/dev/null; then
      ui_warn "git pull failed or not fast-forward — continuing with current tree."
    fi
  fi

  ensure_pull_deploy_mode
  if [[ "$DEPLOY_MODE" == "native" ]]; then
    compose_up_db
    install_native_binary 1
    maybe_sudo systemctl restart hodhod
  elif [[ "$DEPLOY_MODE" == "build" ]]; then
    compose_up_db
    compose_rebuild_app "$build_no_cache"
  else
    compose_up_db
    compose_pull_and_recreate_app || { ui_err "Failed to pull ${HODHOD_IMAGE}"; return 1; }
  fi
  wait_health || true
  ui_ok "Update complete (database volume preserved)."
}

do_remove() {
  local ans ans2
  prompt_yes_no ans "Remove Docker containers and database volume?" "n"
  if [[ "$ans" == "yes" ]]; then
    docker compose down -v 2>/dev/null || true
    ui_ok "Docker stack removed."
  fi
  prompt_yes_no ans2 "Remove Nginx site and SSL certificate?" "n"
  if [[ "$ans2" == "yes" ]]; then
    maybe_sudo rm -f "$NGINX_ENABLED" "$NGINX_AVAILABLE"
    load_env
    local domain; domain="$(domain_from_url "${PUBLIC_BASE_URL:-}")"
    command -v certbot >/dev/null 2>&1 && [[ -n "$domain" ]] && \
      maybe_sudo certbot delete --cert-name "$domain" --non-interactive 2>/dev/null || true
    maybe_sudo nginx -t && maybe_sudo systemctl reload nginx
    ui_ok "Nginx site removed."
  fi
  if systemctl is-enabled hodhod >/dev/null 2>&1; then
    prompt_yes_no ans "Stop and disable native systemd service?" "n"
    [[ "$ans" == "yes" ]] && maybe_sudo systemctl disable --now hodhod
  fi
}

do_logs() {
  load_env
  if [[ "$DEPLOY_MODE" == "native" ]]; then
    maybe_sudo journalctl -u hodhod -f
  else
    docker compose logs -f --tail=100 hodhod-app
  fi
}

do_status() {
  load_env
  echo ""
  if [[ "$DEPLOY_MODE" == "native" ]]; then
    systemctl is-active hodhod 2>/dev/null && ui_ok "hodhod.service: active" || ui_warn "hodhod.service: inactive"
  fi
  docker compose ps 2>/dev/null || true
  curl -sf "http://127.0.0.1:${HTTP_PORT}/healthz" >/dev/null && ui_ok "healthz: ok" || ui_warn "healthz: down"
  [[ -f "$NGINX_ENABLED" ]] && ui_ok "nginx: ${NGINX_ENABLED}"
  echo ""
}

do_regen_secrets() {
  check_prereqs || return 1
  [[ -f "$ENV_FILE" ]] || { ui_err "No .env — run Install first."; return 1; }
  load_env
  APP_ENCRYPTION_KEY=$(gen_secret)
  SESSION_SECRET=$(gen_hex)
  write_env
  ui_ok "Secrets regenerated. Restart: bash install.sh (menu Update) or docker compose up -d --force-recreate hodhod-app"
}

do_nginx_ssl() {
  check_prereqs || return 1
  [[ -f "$ENV_FILE" ]] || { ui_err "No .env — run Install first."; return 1; }
  load_env
  DOMAIN="$(domain_from_url "$PUBLIC_BASE_URL")"
  prompt_email CERTBOT_EMAIL "Let's Encrypt email"
  setup_nginx_ssl "$DOMAIN" "$CERTBOT_EMAIL"
}

# ── main ──────────────────────────────────────────────────────────────────────
# Immediate feedback — ui_init must not block silently on broken terminals.
echo ""
echo "Hodhod installer"
ui_init

if $NON_INTERACTIVE; then
  do_install
  exit $?
fi

ui_hint "Checking prerequisites (openssl, curl, docker)..."
if ! check_prereqs; then
  echo ""
  ui_err "Prerequisites check failed. Fix the issues above and try again."
  exit 1
fi

if ! choose_menu; then
  exit 1
fi
choice="$MENU_CHOICE"
case "$choice" in
  1) do_install ;;
  2) do_update ;;
  3) do_remove ;;
  4) do_logs ;;
  5) do_status ;;
  6) do_regen_secrets ;;
  7) do_nginx_ssl ;;
esac
