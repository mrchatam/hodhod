# AGENTS.md - Read this first

This repository is a **dockerized, multi-tenant Telegram VPN-sales bot written in Go**.
One running instance serves **many Telegram bots** for **many agents**. A **master admin**
connects bots to VPN panels (Marzban + 3x-ui) and decides which agent/bot may sell from
which panel. Agents only customize sale parameters (price, duration, volume) within
master-defined limits.

You (the implementing AI) MUST follow the documents in `docs/guidelines/` and build in the
order defined in `docs/TASKS.md`. Do not improvise architecture. When something is
ambiguous, prefer the simplest option that satisfies the guidelines, leave a `// TODO(plan):`
comment, and keep going.

## Project name & renaming (do this in task 0.1, before any other code)
The official project name is **Hodhod** (هدهد - the hoopoe, the messenger bird; a fitting name
for a bot that delivers configs and messages). `mirza-go` was only a working title.

Apply this rename consistently everywhere as the FIRST step of Phase 0. After renaming, nothing
in the codebase should reference the old working title `mirza-go` or "Mirza" (renamed to Hodhod).

Canonical values:
- Display/product name: `Hodhod`
- Repo/folder name: `hodhod`
- Go module path: `github.com/mrchatam/hodhod` (replace `yourorg` with the real org/user)
- Binary name: `hodhod`
- Docker image: `hodhod`; compose service names: `hodhod-app`, `hodhod-db`
- Default DB name / role: `hodhod`
- Env prefix (optional): keep keys as documented (e.g. `APP_ENCRYPTION_KEY`); app-branded keys
  may use `HODHOD_` if introduced.

Rename checklist:
1. Rename the working directory `mirza-go/` -> `hodhod/` (if not already done).
2. `go mod init github.com/mrchatam/hodhod` (or edit `go.mod` module + fix imports).
3. Update all import paths and references from `mirza-go` to `hodhod`.
4. Update `Dockerfile`, `docker-compose.yml` (service/image/db names), `.env.example`, and
   `install.sh` strings.
5. Update doc references (this file, `docs/*`, README) - replace `mirza-go`/`Mirza` with
   `Hodhod`/`hodhod`.
6. Verify with a repo-wide search: there must be ZERO remaining matches for `mirza-go` or
   `Mirza` (case-insensitive) except in historical changelog notes, if any.

If you (the user) dislike `Hodhod`, change only the canonical values above and re-run the
checklist; the rest of the guidelines are name-agnostic.

## Golden rules (do not violate)
1. **Multi-tenancy is mandatory on every query.** Every data access for bot/agent data MUST
   be scoped by `bot_id` (and `agent_id` where relevant). There is no such thing as a global
   "users" or "orders" read. See `docs/guidelines/database.md` and `multi-tenancy` section.
2. **All outbound HTTP goes through the shared client** in `internal/httpx` which supports an
   optional SOCKS5 proxy. Never call `http.Get`/`http.DefaultClient` directly for Telegram or
   panel traffic. See `docs/guidelines/security.md`.
3. **Secrets are encrypted at rest** (bot tokens, panel passwords) using `internal/crypto`.
   Never log a token, password, receipt image bytes, or `initData`.
4. **Panels are accessed only through the `panels.Client` interface.** No panel-specific HTTP
   anywhere else. See `docs/guidelines/panel-adapters.md`.
5. **Webhook-only Telegram.** No long polling. Each bot has a unique webhook path and secret.
6. **Migrations are explicit and forward-only** (`golang-migrate`). Never rely on GORM
   `AutoMigrate` in production code paths.
7. **Every phase ends green:** `go build ./...`, `go vet ./...`, `golangci-lint run`, and
   `go test ./...` must pass before moving on. See `docs/guidelines/testing-and-dod.md`.
8. **Small, reviewable commits**, one logical change each, conventional-commit messages.

## Document map
- `docs/guidelines/architecture.md` - layers, packages, dependency direction.
- `docs/guidelines/go-style.md` - language conventions, errors, context, logging.
- `docs/guidelines/database.md` - schema, models, migrations, tenant scoping.
- `docs/guidelines/panel-adapters.md` - the `panels.Client` contract + Marzban/3x-ui specifics.
- `docs/guidelines/telegram-and-miniapp.md` - multi-bot manager, handlers, Mini App auth.
- `docs/guidelines/web-gui.md` - master/agent web GUI, auth, scoping.
- `docs/guidelines/security.md` - proxy client, crypto, authz, input validation.
- `docs/guidelines/testing-and-dod.md` - tests + Definition of Done per task.
- `docs/TASKS.md` - the ordered, very detailed task checklist. **Build in this order.**

## Tech stack (fixed)
- Go 1.22+, module path `github.com/mrchatam/hodhod` (see "Project name & renaming"; rename
  consistently if changed).
- PostgreSQL 16. GORM for models/queries, `golang-migrate` for migrations.
- Router: `github.com/go-chi/chi/v5`.
- Telegram: `github.com/go-telegram/bot` (webhook mode).
- SOCKS5: `golang.org/x/net/proxy`.
- Scheduler: `github.com/robfig/cron/v3`.
- Config: `github.com/caarlos0/env/v11` + `.env`.
- Web GUI: `html/template` + HTMX + Tailwind (CDN or prebuilt CSS; no Node build step).
- Logging: `log/slog` (structured JSON in prod).

## Definition of a tenant
- `master` = root operator of the instance.
- `agent` = a reseller; owns one or more `bots`; can manage only their own data.
- `bot` = one Telegram bot (token), owned by one agent; sells from its assigned panels only.
- `end_user` = a Telegram user of a specific bot; identity is `(bot_id, telegram_id)`.
