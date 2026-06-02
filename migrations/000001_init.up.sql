CREATE TABLE agents (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    tg_admin_id BIGINT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    max_bots INT NOT NULL DEFAULT 5,
    price_floor_toman BIGINT NOT NULL DEFAULT 0,
    price_ceiling_toman BIGINT NOT NULL DEFAULT 999999999,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE admins (
    id BIGSERIAL PRIMARY KEY,
    agent_id BIGINT REFERENCES agents(id) ON DELETE SET NULL,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('master', 'agent')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE panels (
    id BIGSERIAL PRIMARY KEY,
    type TEXT NOT NULL CHECK (type IN ('marzban', 'xui')),
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    base_path TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL,
    password_enc TEXT NOT NULL,
    extra_json JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE bots (
    id BIGSERIAL PRIMARY KEY,
    agent_id BIGINT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    public_id TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL DEFAULT '',
    token_enc TEXT NOT NULL,
    webhook_secret TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    settings_json JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE bot_panels (
    id BIGSERIAL PRIMARY KEY,
    bot_id BIGINT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    panel_id BIGINT NOT NULL REFERENCES panels(id) ON DELETE CASCADE,
    scope_json JSONB NOT NULL DEFAULT '{}',
    UNIQUE (bot_id, panel_id)
);
CREATE INDEX idx_bot_panels_bot ON bot_panels(bot_id);

CREATE TABLE plans (
    id BIGSERIAL PRIMARY KEY,
    bot_id BIGINT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    agent_id BIGINT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    panel_id BIGINT REFERENCES panels(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    duration_days INT NOT NULL,
    volume_gb INT NOT NULL,
    price_toman BIGINT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_plans_bot ON plans(bot_id);

CREATE TABLE end_users (
    id BIGSERIAL PRIMARY KEY,
    bot_id BIGINT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    telegram_id BIGINT NOT NULL,
    lang TEXT NOT NULL DEFAULT 'fa',
    balance_toman BIGINT NOT NULL DEFAULT 0,
    warn_percent INT NOT NULL DEFAULT 80,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (bot_id, telegram_id)
);

CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    bot_id BIGINT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    end_user_id BIGINT NOT NULL REFERENCES end_users(id) ON DELETE CASCADE,
    plan_id BIGINT NOT NULL REFERENCES plans(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'pending_payment' CHECK (status IN (
        'pending_payment', 'awaiting_approval', 'approved', 'provisioned', 'rejected', 'expired'
    )),
    price_toman BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_by BIGINT REFERENCES admins(id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ
);
CREATE INDEX idx_orders_bot_status ON orders(bot_id, status);

CREATE TABLE payments (
    id BIGSERIAL PRIMARY KEY,
    bot_id BIGINT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    end_user_id BIGINT NOT NULL REFERENCES end_users(id) ON DELETE CASCADE,
    order_id BIGINT REFERENCES orders(id) ON DELETE SET NULL,
    amount_toman BIGINT NOT NULL,
    method TEXT NOT NULL CHECK (method IN ('card_receipt', 'wallet')),
    receipt_ref TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewed_by BIGINT REFERENCES admins(id) ON DELETE SET NULL,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_payments_bot_status ON payments(bot_id, status);

CREATE TABLE wallet_tx (
    id BIGSERIAL PRIMARY KEY,
    bot_id BIGINT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    end_user_id BIGINT NOT NULL REFERENCES end_users(id) ON DELETE CASCADE,
    delta_toman BIGINT NOT NULL,
    reason TEXT NOT NULL,
    ref_type TEXT NOT NULL DEFAULT '',
    ref_id BIGINT NOT NULL DEFAULT 0,
    balance_after BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_wallet_tx_user ON wallet_tx(bot_id, end_user_id);

CREATE TABLE services (
    id BIGSERIAL PRIMARY KEY,
    bot_id BIGINT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    end_user_id BIGINT NOT NULL REFERENCES end_users(id) ON DELETE CASCADE,
    order_id BIGINT REFERENCES orders(id) ON DELETE SET NULL,
    panel_id BIGINT NOT NULL REFERENCES panels(id) ON DELETE RESTRICT,
    panel_username TEXT NOT NULL,
    sub_link TEXT NOT NULL DEFAULT '',
    data_limit_bytes BIGINT NOT NULL DEFAULT 0,
    used_bytes BIGINT NOT NULL DEFAULT 0,
    expire_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'expired')),
    last_warned_percent INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_services_panel_status ON services(panel_id, status);
CREATE INDEX idx_services_expire ON services(expire_at);

CREATE TABLE settings (
    id BIGSERIAL PRIMARY KEY,
    scope TEXT NOT NULL CHECK (scope IN ('master', 'agent', 'bot')),
    scope_id BIGINT NOT NULL DEFAULT 0,
    key TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    UNIQUE (scope, scope_id, key)
);

CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    admin_id BIGINT REFERENCES admins(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id BIGINT NOT NULL DEFAULT 0,
    detail_json JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    admin_id BIGINT NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
