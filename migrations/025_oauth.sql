-- +goose Up
-- ──────────────────────────────────────────────────────────────
-- 025_oauth.sql — OAuth2 привязка аккаунтов
-- Поддерживает: Google, GitHub (и любые будущие провайдеры)
-- ──────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS oauth_accounts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider    VARCHAR(32)  NOT NULL,  -- "google" | "github"
    provider_id VARCHAR(256) NOT NULL,  -- ID пользователя у провайдера
    email       VARCHAR(255),           -- email от провайдера (может отличаться)
    avatar_url  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- один провайдер-аккаунт → один пользователь
    CONSTRAINT uq_oauth_provider UNIQUE (provider, provider_id)
);

-- быстрый поиск всех OAuth аккаунтов пользователя
CREATE INDEX IF NOT EXISTS idx_oauth_accounts_user_id ON oauth_accounts(user_id);

-- разрешаем NULL пароль для OAuth-only пользователей
ALTER TABLE users ALTER COLUMN password DROP NOT NULL;
ALTER TABLE users ALTER COLUMN password SET DEFAULT '';

-- +goose Down
ALTER TABLE users ALTER COLUMN password SET NOT NULL;
DROP TABLE IF EXISTS oauth_accounts;