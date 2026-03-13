-- name: CreateMedia :one
INSERT INTO media (
    uploader_id,
    type,
    status,
    bucket,
    object_key,
    original_name,
    mime_type
) VALUES (
    $1, $2, 'pending', $3, $4, $5, $6
)
RETURNING *;

-- name: GetMedia :one
SELECT * FROM media
WHERE id = $1;

-- name: UpdateMediaStatus :one
UPDATE media
SET status = $2
WHERE id = $1
RETURNING *;

-- name: UpdateMediaUploaded :one
UPDATE media
SET
    status = 'uploaded',
    size_bytes = $2,
    width = $3,
    height = $4,
    duration_sec = $5
WHERE id = $1
RETURNING *;

-- name: DeleteMedia :exec
DELETE FROM media
WHERE id = $1;

-- name: GetPendingExpiredMedia :many
SELECT * FROM media
WHERE status = 'pending'
AND created_at < NOW() - INTERVAL '24 hours';

-- name: GetPendingMedia :many
SELECT * FROM media WHERE status = 'uploaded' LIMIT 10;

-- name: SetMediaFailed :exec
UPDATE media SET status = 'failed' WHERE id = $1;

-- name: SetMediaProcessed :exec
UPDATE media SET status = 'processed' WHERE id = $1;

-- name: SetMediaProcessedWithThumb :exec
UPDATE media SET status = 'processed', thumb_key = $2 WHERE id = $1;

-- name: SetMediaProcessedWithWaveform :exec
UPDATE media
SET status = 'processed', waveform = $2
WHERE id = $1;
