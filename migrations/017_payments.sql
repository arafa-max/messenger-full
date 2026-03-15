-- +goose Up
CREATE TABLE payments (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id          UUID        NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount          BIGINT      NOT NULL,  -- в копейках/центах
    currency        TEXT        NOT NULL DEFAULT 'usd',
    status          TEXT        NOT NULL DEFAULT 'pending',
    stripe_id       TEXT        NOT NULL DEFAULT '',
    description     TEXT        NOT NULL DEFAULT '',
    metadata        JSONB       NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_payments_bot    ON payments(bot_id);
CREATE INDEX idx_payments_user   ON payments(user_id);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_stripe ON payments(stripe_id);

-- +goose Down
DROP TABLE IF EXISTS payments;