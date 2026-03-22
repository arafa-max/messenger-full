-- name: CreateNotification :one
INSERT INTO notifications (user_id, type, title, body, reference_id, reference_type, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetUserNotifications :many
SELECT * FROM notifications
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: MarkNotificationRead :exec
UPDATE notifications SET is_read = TRUE WHERE id = $1 AND user_id = $2;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET is_read = TRUE WHERE user_id = $1;

-- name: GetUnreadNotificationsCount :one
SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = FALSE;