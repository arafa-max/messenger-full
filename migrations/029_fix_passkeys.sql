-- +goose Up
ALTER TABLE passkeys DROP COLUMN aaguid;
ALTER TABLE passkeys ADD COLUMN aaguid BYTEA NOT NULL DEFAULT '\x'::bytea;

-- +goose Down
ALTER TABLE passkeys DROP COLUMN aaguid;
ALTER TABLE passkeys ADD COLUMN aaguid UUID;