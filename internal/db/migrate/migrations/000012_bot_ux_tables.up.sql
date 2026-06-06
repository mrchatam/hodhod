CREATE TABLE IF NOT EXISTS bot_channels (
    id          BIGSERIAL PRIMARY KEY,
    bot_id      BIGINT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    username    TEXT NOT NULL,
    label       TEXT NOT NULL DEFAULT '',
    join_url    TEXT NOT NULL DEFAULT '',
    mandatory   BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order  INT NOT NULL DEFAULT 0,
    active      BOOLEAN NOT NULL DEFAULT TRUE
);
CREATE INDEX IF NOT EXISTS idx_bot_channels_bot ON bot_channels(bot_id);

CREATE TABLE IF NOT EXISTS bot_menu_buttons (
    id           BIGSERIAL PRIMARY KEY,
    bot_id       BIGINT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    button_key   TEXT NOT NULL,
    label_fa     TEXT NOT NULL DEFAULT '',
    label_en     TEXT NOT NULL DEFAULT '',
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order   INT NOT NULL DEFAULT 0,
    url          TEXT NOT NULL DEFAULT '',
    UNIQUE (bot_id, button_key)
);

CREATE TABLE IF NOT EXISTS bot_notification_targets (
    id          BIGSERIAL PRIMARY KEY,
    bot_id      BIGINT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    telegram_id BIGINT NOT NULL,
    events      JSONB NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bot_notification_targets_bot ON bot_notification_targets(bot_id);

INSERT INTO bot_channels (bot_id, username, label, mandatory, active)
SELECT s.scope_id, trim(s.value), trim(s.value), true, true
FROM settings s
WHERE s.scope = 'bot' AND s.key = 'force_join_channel' AND trim(s.value) <> ''
  AND NOT EXISTS (SELECT 1 FROM bot_channels c WHERE c.bot_id = s.scope_id);

INSERT INTO settings (scope, scope_id, key, value)
SELECT 'bot', s.scope_id, 'welcome_text_fa', s.value
FROM settings s
WHERE s.scope = 'bot' AND s.key = 'welcome_text' AND trim(s.value) <> ''
ON CONFLICT (scope, scope_id, key) DO NOTHING;
