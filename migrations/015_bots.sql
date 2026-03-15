-- +goose Up
CREATE TABLE IF NOT EXISTS bots (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token          TEXT        NOT NULL UNIQUE,
    username       TEXT        NOT NULL UNIQUE,
    name           TEXT        NOT NULL,
    description    TEXT        NOT NULL DEFAULT '',
    avatar_url     TEXT        NOT NULL DEFAULT '',
    is_active      BOOL        NOT NULL DEFAULT true,
    is_ai_enabled  BOOL        NOT NULL DEFAULT false,
    webhook_url    TEXT        NOT NULL DEFAULT '',
    webhook_secret TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS bot_commands (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id      UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    command     TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    UNIQUE(bot_id, command)
);

CREATE TABLE IF NOT EXISTS bot_updates (
    id         BIGSERIAL   PRIMARY KEY,
    bot_id     UUID        NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    update_id  BIGINT      NOT NULL,
    type       TEXT        NOT NULL DEFAULT 'message',
    payload    JSONB       NOT NULL DEFAULT '{}',
    processed  BOOL        NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(bot_id, update_id)
);

CREATE INDEX IF NOT EXISTS idx_bot_updates_unprocessed ON bot_updates(bot_id, processed)
    WHERE processed = false;
CREATE INDEX IF NOT EXISTS idx_bots_token ON bots(token);
CREATE INDEX IF NOT EXISTS idx_bots_owner ON bots(owner_id);

-- +goose Down
DROP TABLE IF EXISTS bot_updates;
DROP TABLE IF EXISTS bot_commands;
DROP TABLE IF EXISTS bots;