-- +goose Up
UPDATE bots SET webhook_url = '' WHERE webhook_url IS NULL;
UPDATE bots SET webhook_secret = '' WHERE webhook_secret IS NULL;

ALTER TABLE bots
    ALTER COLUMN webhook_url SET DEFAULT '',
    ALTER COLUMN webhook_url SET NOT NULL,
    ALTER COLUMN webhook_secret SET DEFAULT '',
    ALTER COLUMN webhook_secret SET NOT NULL;

-- +goose Down
ALTER TABLE bots
    ALTER COLUMN webhook_url DROP NOT NULL,
    ALTER COLUMN webhook_url DROP DEFAULT,
    ALTER COLUMN webhook_secret DROP NOT NULL,
    ALTER COLUMN webhook_secret DROP DEFAULT;