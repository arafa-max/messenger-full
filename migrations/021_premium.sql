-- +goose Up

-- Расширяем существующую таблицу subscriptions
ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS stripe_customer_id  TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS stripe_sub_id       TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS current_period_end  TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS cancel_at_period_end BOOLEAN    NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS updated_at          TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_user     ON subscriptions(user_id);
CREATE INDEX        IF NOT EXISTS idx_subscriptions_stripe   ON subscriptions(stripe_sub_id);
CREATE INDEX        IF NOT EXISTS idx_subscriptions_customer ON subscriptions(stripe_customer_id);

CREATE TABLE IF NOT EXISTS premium_settings (
    user_id              UUID        PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    hide_phone           BOOLEAN     NOT NULL DEFAULT FALSE,
    away_message         TEXT        NOT NULL DEFAULT '',
    away_message_enabled BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS chat_labels (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id    UUID        NOT NULL,
    label      TEXT        NOT NULL,
    color      TEXT        NOT NULL DEFAULT '#6B7280',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX        IF NOT EXISTS idx_chat_labels_user   ON chat_labels(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_labels_unique ON chat_labels(user_id, chat_id, label);

-- +goose Down
ALTER TABLE subscriptions
    DROP COLUMN IF EXISTS stripe_customer_id,
    DROP COLUMN IF EXISTS stripe_sub_id,
    DROP COLUMN IF EXISTS current_period_end,
    DROP COLUMN IF EXISTS cancel_at_period_end,
    DROP COLUMN IF EXISTS updated_at;

DROP TABLE IF EXISTS chat_labels;
DROP TABLE IF EXISTS premium_settings;