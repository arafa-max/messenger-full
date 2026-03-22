

-- name: CreateBot :one
INSERT INTO bots (owner_id, user_id, token, username, name, description, is_ai_enabled)
VALUES ($1, $1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetBotByToken :one
SELECT * FROM bots
WHERE token = $1 AND is_active = true
LIMIT 1;

-- name: GetBotByID :one
SELECT * FROM bots
WHERE id = $1
LIMIT 1;

-- name: GetMyBots :many
SELECT * FROM bots
WHERE owner_id = $1
ORDER BY created_at DESC;

-- name: UpdateBotWebhook :exec
UPDATE bots
SET webhook_url = $2, webhook_secret = $3
WHERE id = $1;

-- name: DeactivateBot :exec
UPDATE bots
SET is_active = false
WHERE id = $1 AND owner_id = $2;

-- name: SaveBotUpdate :exec
INSERT INTO bot_updates (bot_id, update_id, type, payload)
VALUES ($1, $2, $3, $4)
ON CONFLICT (bot_id, update_id) DO NOTHING;

-- name: GetUnprocessedUpdates :many
SELECT * FROM bot_updates
WHERE bot_id = $1 AND processed = false
ORDER BY update_id ASC
LIMIT 100;

-- name: MarkUpdateProcessed :exec
UPDATE bot_updates
SET processed = true
WHERE id = $1;

-- name: CreateBotCommand :exec
INSERT INTO bot_commands (bot_id, command, description)
VALUES ($1, $2, $3)
ON CONFLICT (bot_id, command) DO UPDATE
SET description = EXCLUDED.description;

-- name: GetBotCommands :many
SELECT * FROM bot_commands
WHERE bot_id = $1
ORDER BY command ASC;

-- name: DeleteBotCommand :exec
DELETE FROM bot_commands
WHERE bot_id = $1 AND command = $2;