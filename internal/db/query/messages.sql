-- name: CreateMessage :one
INSERT INTO messages(chat_id, sender_id, type, content, reply_to_id, format, is_spoiler, scheduled_at, expires_at, topic_id,media_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,$11)
RETURNING *;

-- name: GetMessageByID :one
SELECT * FROM messages WHERE id = $1 AND is_deleted = FALSE;

-- name: GetChatMessages :many
SELECT m.* FROM messages m
LEFT JOIN deleted_messages dm ON dm.message_id = m.id AND dm.user_id = $4
WHERE m.chat_id = $1
  AND m.is_deleted = FALSE
  AND m.scheduled_at IS NULL
  AND dm.message_id IS NULL
ORDER BY m.created_at DESC
LIMIT $2 OFFSET $3;

-- name: EditMessage :one
UPDATE messages SET content = $2, format = $3, is_edited = TRUE, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteMessageForAll :exec
UPDATE messages SET is_deleted = TRUE, updated_at = NOW()
WHERE id = $1;

-- name: DeleteMessageForMe :exec
INSERT INTO deleted_messages (message_id, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ForwardMessage :one
INSERT INTO messages (
    chat_id, sender_id, type, content, format,
    forwarded_from_id, forward_sender_id, forward_chat_id, forward_date
)
SELECT $2, $3, src.type, src.content, src.format, src.id, src.sender_id, src.chat_id, src.created_at
FROM messages src WHERE src.id = $1
RETURNING *;

-- name: PinMessage :exec
UPDATE messages SET is_pinned = TRUE WHERE id = $1;

-- name: UnpinMessage :exec
UPDATE messages SET is_pinned = FALSE WHERE id = $1;

-- name: GetPinnedMessages :many
SELECT * FROM messages
WHERE chat_id = $1 AND is_pinned = TRUE AND is_deleted = FALSE
ORDER BY updated_at DESC;

-- name: AddReaction :exec
INSERT INTO reactions (message_id, user_id, emoji)
VALUES ($1, $2, $3)
ON CONFLICT (message_id, user_id, emoji) DO NOTHING;

-- name: RemoveReaction :exec
DELETE FROM reactions
WHERE message_id = $1 AND user_id = $2 AND emoji = $3;

-- name: GetMessageReactions :many
SELECT emoji, COUNT(*) as count
FROM reactions
WHERE message_id = $1
GROUP BY emoji
ORDER BY count DESC;

-- name: GetUserReaction :one
SELECT emoji FROM reactions
WHERE message_id = $1 AND user_id = $2 AND emoji = $3;

-- name: CreateMessageStatus :exec
INSERT INTO message_status (message_id, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: UpdateMessageDelivered :exec
UPDATE message_status
SET delivered = TRUE, delivered_at = NOW()
WHERE message_id = $1 AND user_id = $2;

-- name: UpdateMessageRead :exec
UPDATE message_status
SET read = TRUE, read_at = NOW()
WHERE message_id = $1 AND user_id = $2;

-- name: MarkChatMessagesRead :exec
UPDATE message_status ms
SET read = TRUE, read_at = NOW()
FROM messages m
WHERE ms.message_id = m.id
  AND m.chat_id = $1
  AND ms.user_id = $2
  AND ms.read = FALSE;

-- name: SaveMessage :exec
INSERT INTO saved_messages (user_id, message_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: UnsaveMessage :exec
DELETE FROM saved_messages
WHERE user_id = $1 AND message_id = $2;

-- name: GetSavedMessages :many
SELECT m.* FROM messages m
JOIN saved_messages sm ON sm.message_id = m.id
WHERE sm.user_id = $1 AND m.is_deleted = FALSE
ORDER BY sm.saved_at DESC
LIMIT $2 OFFSET $3;

-- name: SetMessageReminder :one
INSERT INTO message_reminders (user_id, message_id, remind_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, message_id)
DO UPDATE SET remind_at = EXCLUDED.remind_at, is_sent = FALSE
RETURNING *;

-- name: GetPendingReminders :many
SELECT * FROM message_reminders
WHERE remind_at <= NOW() AND is_sent = FALSE;

-- name: MarkReminderSent :exec
UPDATE message_reminders SET is_sent = TRUE WHERE id = $1;

-- name: GetScheduledMessages :many
SELECT * FROM messages
WHERE scheduled_at <= NOW()
  AND scheduled_at IS NOT NULL
  AND is_deleted = FALSE;

-- name: SendScheduledMessage :exec
UPDATE messages
SET scheduled_at = NULL, created_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: SearchMessages :many
SELECT * FROM messages
WHERE chat_id = @chat_id
  AND is_deleted = FALSE
  AND content ILIKE '%' || @query::text || '%'
ORDER BY created_at DESC
LIMIT @page_size OFFSET @page_offset;

-- name: CreateQuickReply :one
INSERT INTO quick_replies (user_id, shortcut, text)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, shortcut)
DO UPDATE SET text = EXCLUDED.text
RETURNING *;

-- name: GetQuickReplies :many
SELECT * FROM quick_replies
WHERE user_id = $1
ORDER BY shortcut;

-- name: DeleteQuickReply :exec
DELETE FROM quick_replies WHERE id = $1 AND user_id = $2;

-- name: DeleteExpiredMessages :exec
UPDATE messages
SET is_deleted = TRUE, updated_at = NOW()
WHERE expires_at IS NOT NULL
  AND expires_at <= NOW()
  AND is_deleted = FALSE;