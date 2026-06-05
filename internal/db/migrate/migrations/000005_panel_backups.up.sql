CREATE TABLE panel_backups (
    id BIGSERIAL PRIMARY KEY,
    panel_id BIGINT NOT NULL REFERENCES panels(id) ON DELETE CASCADE,
    filename TEXT NOT NULL,
    size_bytes BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'ok' CHECK (status IN ('ok', 'failed')),
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_panel_backups_panel ON panel_backups(panel_id, created_at DESC);
