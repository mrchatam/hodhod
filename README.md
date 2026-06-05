# Hodhod

Multi-tenant Telegram VPN sales bot (Go + PostgreSQL + Docker).

One instance runs many bots for many agents. A **master admin** connects bots to Marzban / 3x-ui
panels and controls which agent may sell from which panel. Agents customize plans (price,
duration, volume) within master limits.

## Features

- Multi-bot webhooks (`POST /wh/tg/{publicID}`)
- Wallet + manual receipt top-up with admin/agent approval
- Automated provisioning on purchase
- Usage warnings (default 80%) and expiry notices
- Master + agent web GUI (HTMX + Tailwind)
- Telegram Mini App JSON API
- Optional outbound SOCKS5 for Iran hosting

## Quick start (recommended)

On a Linux server with Docker (Ubuntu/Debian recommended):

```bash
git clone https://github.com/mrchatam/hodhod.git
cd hodhod
bash install.sh
```

Choose **Install**. The installer will:

1. Verify Docker, OpenSSL, and curl
2. Prompt for your public **HTTPS URL** (`https://your.domain`), master credentials, optional SOCKS proxy, and local app port (default `8080`)
3. Generate secrets and write `.env`
4. Build and start the Docker stack (`hodhod-app` + PostgreSQL)
5. Wait for `/healthz`
6. **Configure host Nginx + Let's Encrypt SSL** (Certbot) — installs `nginx` and `certbot` if needed, writes the site config, obtains a certificate, and enables HTTPS redirect

After install:

- **Admin GUI:** `https://your.domain/login`
- **Local debug:** `http://127.0.0.1:8080/login` (bound to localhost only)
- **Webhooks:** `https://your.domain/wh/tg/{publicID}`
- **Mini App:** `https://your.domain/miniapp/index.html?bot={publicID}`

Non-interactive install (CI / automation):

```bash
export PUBLIC_BASE_URL=https://your.domain
export MASTER_PASSWORD='your-secure-password'
export SETUP_NGINX=1
export CERTBOT_EMAIL=you@example.com
bash install.sh --non-interactive
```

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
