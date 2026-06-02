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

```bash
bash install.sh
```

The interactive installer verifies Docker/OpenSSL, prompts for your public HTTPS URL and master
credentials, generates secrets, writes `.env`, starts the stack, and prints the admin login URL
plus an Nginx snippet.

## Advanced: manual `.env`

```bash
cp .env.example .env
# Edit .env (set APP_ENCRYPTION_KEY, PUBLIC_BASE_URL, MASTER_PASSWORD, etc.)
docker compose up -d --build
```

Admin GUI: `http://127.0.0.1:8080/login`

See [AGENTS.md](AGENTS.md) for architecture and implementation rules.

## Development

```bash
go test ./...
go build -o bin/hodhod ./cmd/server
```

## License

Proprietary / your choice.
