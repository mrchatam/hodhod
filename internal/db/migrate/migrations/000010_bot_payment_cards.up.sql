CREATE TABLE IF NOT EXISTS bot_payment_cards (
    id          BIGSERIAL PRIMARY KEY,
    bot_id      BIGINT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    label       TEXT NOT NULL DEFAULT '',
    card_number TEXT NOT NULL,
    holder_name TEXT NOT NULL DEFAULT '',
    weight      INT NOT NULL DEFAULT 1,
    sort_order  INT NOT NULL DEFAULT 0,
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bot_payment_cards_bot ON bot_payment_cards(bot_id);

ALTER TABLE bots ADD COLUMN IF NOT EXISTS card_display_mode TEXT NOT NULL DEFAULT 'random';
ALTER TABLE bots ADD COLUMN IF NOT EXISTS card_rr_index INT NOT NULL DEFAULT 0;
