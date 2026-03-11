-- +goose Up
-- +goose StatementBegin
ALTER TABLE messages
    ALTER COLUMN content SET NOT NULL,
    ALTER COLUMN content SET DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE messages
    ALTER COLUMN content DROP NOT NULL,
    ALTER COLUMN content DROP DEFAULT;
-- +goose StatementEnd