# Hodhod

Multi-tenant Telegram VPN sales bot (Go + PostgreSQL + Docker).

One instance runs many bots for many agents. A **master admin** connects bots to Marzban / 3x-ui
panels and controls which agent may sell from which panel. Agents customize plans (price,
duration, volume) within master limits.

## Features

- **Unified sales panel + Telegram bots** — sellers manage VPN customers in the web GUI and via their bot
- **Manual panel accounts** — create users without Telegram (label/contact, sub-link copy)
- **Granular seller permissions** — master assigns create/modify/add-time/add-volume/reset/disable/delete/bot/plans per agent
- **Agent panel assignment** — scope inbounds, max users, expiry caps per seller
- **Seller custom domains** — master assigns a verified custom domain for branded panel + Mini App (webhooks stay on main URL)
- Multi-bot webhooks (`POST /wh/tg/{publicID}`)
- Wallet + manual receipt top-up with admin/agent approval (card numbers shown in bot)
- Shared provisioning via `internal/sales` (panel, bot, and Mini App)
- Usage warnings (per-bot `warn_percent`) and expiry notices
- Master + agent web GUI (HTMX + Tailwind, role-aware nav)
- Telegram Mini App JSON API
- Optional outbound SOCKS5 for Iran hosting
- **Live panel user management** — browse/create/edit users directly on Marzban / 3x-ui panels
- **3x-ui automatic backups** — scheduled `x-ui.db` snapshots with download, retention, and Telegram push

See [docs/PANEL_FEATURES.md](docs/PANEL_FEATURES.md) for panel capability coverage and known gaps.

## Sales panel (web GUI)

After login as **master**:

1. **Panels** — add Marzban / 3x-ui panels, test connection (works without a bot for manual services)
2. **Agents** — create sellers, set permissions, assign panels + quotas, create web login + reset password
3. **Bots** *(optional)* — attach Telegram bots to agents for automated sales (token validated via `getMe`)

After login as **agent** (seller):

1. **Services** — list/create VPN users on assigned panels (+ time/GB, disable, reset, delete — permission-gated)
2. **My Bots** — per-bot settings (welcome, support, card numbers, approver, force-join channel)
3. **Plans** — sell plans within master price floor/ceiling on assigned panels

Both channels share one tenant model: bot sales and manual panel creates go through the same sales service.

### Seller custom domain

Master assigns a seller-owned domain (e.g. `shop.example.com`) on the agent edit page:

1. Set domain → verify DNS (TXT `_hodhod-verify.{domain}` or CNAME to main host)
2. Enable domain after verification
3. Configure Nginx + TLS: `bash scripts/add-seller-domain.sh shop.example.com`
4. Seller logs in at `https://shop.example.com/login` (master routes blocked on seller host)

Telegram webhooks remain on `PUBLIC_BASE_URL` (`/wh/tg/{publicID}`).

## Quick start (recommended)

On a Linux server with Docker (Ubuntu/Debian recommended):

```bash
git clone https://github.com/mrchatam/hodhod.git
cd hodhod
bash install.sh
```

Choose **Install**. The installer will:

1. Verify prerequisites (Docker, OpenSSL, curl)
2. Let you pick a **deploy mode** (see below)
3. Prompt for your public URL — enter `your.domain` or `https://your.domain`; the installer normalizes it automatically
4. Generate secrets and write `.env`
5. Start services (pull prebuilt image by default — no compile wait)
6. Wait for `/healthz`
7. Optionally **configure host Nginx + Let's Encrypt SSL**

### Deploy modes

| Mode | What runs | Speed | When to use |
|------|-----------|-------|-------------|
| **Docker (prebuilt)** | App image from GHCR + Postgres container | Fastest | Production default |
| **Docker (build)** | Builds app image locally + Postgres | Slow | Development / before first release image |
| **Native binary** | `hodhod` binary via systemd + Postgres container only | Fast | Minimal app container overhead |

**Do you still need Docker?** For modes 1–2, yes (app + DB). For **native** mode, only Postgres runs in Docker — the Go app runs directly on the host. You still need PostgreSQL somewhere; Docker keeps that consistent. A pure non-Docker install (system Postgres + binary) is possible manually but not automated yet.

After install:

- **Admin GUI:** `https://your.domain/login`
- **Local debug:** `http://127.0.0.1:8080/login` (bound to localhost only)
- **Webhooks:** `https://your.domain/wh/tg/{publicID}`
- **Mini App:** `https://your.domain/miniapp/index.html?bot={publicID}`

Non-interactive install (CI / automation):

```bash
export PUBLIC_BASE_URL=your.domain          # https:// added automatically
export MASTER_PASSWORD='your-secure-password'
export DEPLOY_MODE=docker                   # docker | build | native
export SETUP_NGINX=1
export CERTBOT_EMAIL=you@example.com
bash install.sh --non-interactive
```

Prebuilt images are published to `ghcr.io/mrchatam/hodhod:latest` on each push to `main` and on `v*` tags (see `.github/workflows/release.yml`).

### Restricted networks (Iran / blocked CDNs)

Production installs should **pull the prebuilt GHCR image** — no local Docker build on the server.

| Registry | Arvan mirror helps? | If blocked |
|----------|---------------------|------------|
| `docker.io` (Postgres base image) | Yes — installer can enable Arvan mirror | Enable mirror in install prompt |
| `ghcr.io` (Hodhod app image) | No | Use **native** deploy mode (downloads GitHub release binary) |

If `docker pull ghcr.io/mrchatam/hodhod:latest` fails, re-run the installer and choose **native** mode, or set `DEPLOY_MODE=native` for non-interactive installs. Local **build** mode is developer-only and may fail when Alpine/Debian package mirrors are unreachable.

Installer menu options: update stack, remove stack, logs, status, regenerate secrets, **reconfigure Nginx + SSL**.

## Deployment

### Architecture

```
Internet → Nginx (443, TLS) → 127.0.0.1:8080 → hodhod-app (Docker)
                                      ↓
                               hodhod-db (Docker PostgreSQL)
```

Hodhod listens on **localhost only** (`127.0.0.1:8080`). Public HTTPS is handled by **host Nginx**.
The installer creates `/etc/nginx/sites-available/hodhod.conf` from
[deploy/nginx/hodhod.conf.example](deploy/nginx/hodhod.conf.example).

### Requirements

- Linux server (Ubuntu 22.04+ or similar)
- Docker + Docker Compose plugin
- Domain DNS **A/AAAA record** pointing to the server **before** running SSL setup
- Ports **80** and **443** open (Let's Encrypt HTTP-01 + HTTPS)

### Routes (proxied by Nginx)

| Path | Purpose |
|------|---------|
| `/` | Admin web GUI |
| `/wh/tg/{publicID}` | Telegram bot webhooks |
| `/api/miniapp/{publicID}/*` | Mini App JSON API |
| `/miniapp/` | Mini App static page |

### Manual Nginx + SSL

If you skipped SSL during install, run menu option **7** or:

```bash
# Edit placeholders, then:
sudo cp deploy/nginx/hodhod.conf.example /etc/nginx/sites-available/hodhod.conf
sudo sed -i 's/__DOMAIN__/your.domain/; s/__HTTP_PORT__/8080/' /etc/nginx/sites-available/hodhod.conf
sudo ln -sf /etc/nginx/sites-available/hodhod.conf /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
sudo certbot --nginx -d your.domain --redirect
```

### Iran hosting + SOCKS proxy

When Telegram or panel APIs are blocked, set an outbound SOCKS5 proxy in `.env`:

```env
OUTBOUND_SOCKS_PROXY=socks5h://host.docker.internal:10810
```

Docker Compose maps `host.docker.internal` to the host gateway so a local SOCKS listener works.

### Arvan CDN / reverse proxy

If you terminate TLS at Arvan (or another CDN) instead of on the server:

- Origin must forward **POST** bodies intact for `/wh/tg/*`
- Preserve header `X-Telegram-Bot-Api-Secret-Token`
- Set `PUBLIC_BASE_URL` to the **public HTTPS URL** Telegram will call (CDN URL or origin URL, whichever receives webhooks)

### Updates and backups

```bash
bash install.sh   # choose 2) Update
docker compose logs -f hodhod-app
./scripts/backup-db.sh   # if configured
```

## Advanced: manual `.env`

```bash
cp .env.example .env
# Edit .env (APP_ENCRYPTION_KEY, PUBLIC_BASE_URL, MASTER_PASSWORD, etc.)
docker compose up -d --build
# Then configure Nginx manually (see above) or: bash install.sh → option 7
```

See [AGENTS.md](AGENTS.md) for architecture and implementation rules.

## Development

```bash
go test ./...
go build -o bin/hodhod ./cmd/server
```

## License

Proprietary / your choice.
