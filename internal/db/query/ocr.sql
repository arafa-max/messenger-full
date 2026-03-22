-- name: CreateMediaSearchIndex :exec
INSERT INTO media_search_index (media_id, message_id, content, source, lang)
VALUES (@media_id, @message_id, @content, @source, @lang)
ON CONFLICT DO NOTHING;

-- name: GetMediaSearchIndex :one
SELECT * FROM media_search_index
WHERE media_id = @media_id
LIMIT 1;

-- name: GetMediaByID :one
SELECT * FROM media WHERE id = @id;