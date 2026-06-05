CREATE TABLE customers (
    id BIGSERIAL PRIMARY KEY,
    agent_id BIGINT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    label TEXT NOT NULL DEFAULT '',
    contact TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_customers_agent ON customers(agent_id);

CREATE TABLE agent_permissions (
    agent_id BIGINT PRIMARY KEY REFERENCES agents(id) ON DELETE CASCADE,
    create_user BOOLEAN NOT NULL DEFAULT FALSE,
    modify_user BOOLEAN NOT NULL DEFAULT FALSE,
    add_time BOOLEAN NOT NULL DEFAULT FALSE,
    add_volume BOOLEAN NOT NULL DEFAULT FALSE,
    reset_usage BOOLEAN NOT NULL DEFAULT FALSE,
    disable_enable BOOLEAN NOT NULL DEFAULT FALSE,
    delete_user BOOLEAN NOT NULL DEFAULT FALSE,
    manage_bot BOOLEAN NOT NULL DEFAULT FALSE,
    manage_plans BOOLEAN NOT NULL DEFAULT FALSE,
    view_only BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE agent_panels (
    id BIGSERIAL PRIMARY KEY,
    agent_id BIGINT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    panel_id BIGINT NOT NULL REFERENCES panels(id) ON DELETE CASCADE,
    scope_json JSONB NOT NULL DEFAULT '{}',
    quota_bytes BIGINT NOT NULL DEFAULT 0,
    max_users INT NOT NULL DEFAULT 0,
    expiry_cap_days INT NOT NULL DEFAULT 0,
    UNIQUE (agent_id, panel_id)
);
CREATE INDEX idx_agent_panels_agent ON agent_panels(agent_id);

ALTER TABLE services ADD COLUMN IF NOT EXISTS agent_id BIGINT REFERENCES agents(id) ON DELETE CASCADE;
ALTER TABLE services ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'bot';
ALTER TABLE services ADD COLUMN IF NOT EXISTS label TEXT NOT NULL DEFAULT '';
ALTER TABLE services ADD COLUMN IF NOT EXISTS customer_id BIGINT REFERENCES customers(id) ON DELETE SET NULL;
ALTER TABLE services ADD COLUMN IF NOT EXISTS created_by_admin_id BIGINT REFERENCES admins(id) ON DELETE SET NULL;

ALTER TABLE services ALTER COLUMN bot_id DROP NOT NULL;
ALTER TABLE services ALTER COLUMN end_user_id DROP NOT NULL;

UPDATE services s SET agent_id = b.agent_id FROM bots b WHERE s.bot_id = b.id AND s.agent_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_services_agent ON services(agent_id);

ALTER TABLE services DROP CONSTRAINT IF EXISTS services_source_check;
ALTER TABLE services ADD CONSTRAINT services_source_check CHECK (source IN ('bot', 'panel'));
