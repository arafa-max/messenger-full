-- +goose Up
-- +goose StatementBegin
ALTER TABLE chat_members
    ADD COLUMN IF NOT EXISTS is_archived BOOLEAN DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE chat_members
    DROP COLUMN IF EXISTS is_archived;
-- +goose StatementEnd