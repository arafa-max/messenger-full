-- name: CreateMessage :one
INSERT INTO messages(chat_id,sender_id,type,content,reply_to_id)
VALUES ($1,$2,$3,$4,$5) RETURNING *;

-- name: GetMessageByID :one
SELECT * FROM messages WHERE id=$1 AND is_deleted = FALSE;

-- name: GetChatMessages :many
SELECT * FROM messages WHERE chat_id =$1 AND is_deleted =FALSE
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: EditMessage :one
UPDATE messages SET content =$2,is_edited = TRUE, updated_at=NOW()
WHERE id=$1 RETURNING *;

-- name: DeleteMessageForAll :exec
UPDATE messages SET is_deleted =TRUE WHERE id =$1;

-- name: PinMessage :exec
UPDATE messages SET is_pinned = TRUE WHERE id =$1;

-- name: GetPinnedMessages :many
SELECT * FROM messages WHERE chat_id =$1 AND is_pinned =TRUE AND is_deleted =FALSE;

-- name: CreateMessageStatus :exec
INSERT INTO message_status (message_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING;

-- name: UpdateMessageDelivered :exec
UPDATE message_status SET delivered = TRUE,delivered_at = NOW() WHERE message_id =$1 AND user_id =$2;

-- name: UpdateMessageRead :exec
UPDATE message_status SET read = TRUE,read_at = NOW() WHERE message_id=$1 AND user_id=$2;