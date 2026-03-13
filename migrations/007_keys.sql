-- +goose Up
-- +goose StatementBegin

-- Удаляем устаревшую упрощённую таблицу ключей
-- Заменена на: identity_keys, signed_prekeys, one_time_prekeys, key_bundles (миграция 002)
DROP TABLE IF EXISTS encryption_keys CASCADE;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Восстанавливаем encryption_keys если нужен rollback
CREATE TABLE IF NOT EXISTS encryption_keys (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       UUID REFERENCES devices(id) ON DELETE CASCADE,
    public_key      TEXT NOT NULL,
    key_type        VARCHAR(20) DEFAULT 'identity',
    is_used         BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- +goose StatementEnd