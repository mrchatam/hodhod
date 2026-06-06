CREATE TABLE IF NOT EXISTS agent_inbound_grants (
    id BIGSERIAL PRIMARY KEY,
    agent_id BIGINT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    panel_id BIGINT NOT NULL REFERENCES panels(id) ON DELETE CASCADE,
    inbound_id INT NOT NULL,
    allow_create BOOLEAN NOT NULL DEFAULT FALSE,
    allow_view_users BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (agent_id, panel_id, inbound_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_inbound_grants_agent_panel
    ON agent_inbound_grants (agent_id, panel_id);

CREATE TABLE IF NOT EXISTS agent_user_grants (
    id BIGSERIAL PRIMARY KEY,
    agent_id BIGINT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    panel_id BIGINT NOT NULL REFERENCES panels(id) ON DELETE CASCADE,
    panel_username TEXT NOT NULL,
    allow_view BOOLEAN NOT NULL DEFAULT FALSE,
    allow_modify BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (agent_id, panel_id, panel_username)
);

CREATE INDEX IF NOT EXISTS idx_agent_user_grants_agent_panel
    ON agent_user_grants (agent_id, panel_id);
