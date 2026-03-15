-- name: GetAnimatedEmoji :one
SELECT * FROM animated_emoji WHERE emoji = $1;

-- name: GetAnimatedEmojiBatch :many
-- Получить несколько эмодзи за раз (для предзагрузки)
SELECT * FROM animated_emoji WHERE emoji = ANY($1::text[]);

-- name: UpsertAnimatedEmoji :exec
INSERT INTO animated_emoji (emoji, object_key, bucket)
VALUES ($1, $2, $3)
ON CONFLICT (emoji) DO UPDATE
    SET object_key = EXCLUDED.object_key,
        bucket     = EXCLUDED.bucket;

-- name: ListAnimatedEmoji :many
SELECT emoji, object_key FROM animated_emoji ORDER BY id;