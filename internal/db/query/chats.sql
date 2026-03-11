-- name: CreateChat :one
INSERT INTO chats(type,name,owner_id,is_public)
VALUES ($1,$2,$3,$4) RETURNING *;

-- name: GetChatByID :one
SELECT * FROM chats WHERE id=$1 AND is_deleted=FALSE;

-- name: GetUserChats :many
SELECT c.* FROM chats c JOIN chat_members cm ON c.id = cm.chat_id
WHERE cm.user_id =$1 AND c.is_deleted =FALSE ORDER BY c.updated_at DESC;

-- name: AddChatMember :exec
INSERT INTO chat_members (chat_id,user_id,role)VALUES ($1,$2,$3);

-- name: GetChatMembers :many
SELECT * FROM chat_members WHERE chat_id =$1 AND is_banned =FALSE;

-- name: UpdateChatMemberRole :exec
UPDATE chat_members SET role = $3 WHERE chat_id =$1 AND user_id =$2;

-- name: RemoveChatMember :exec
DELETE FROM chat_members WHERE chat_id = $1 AND user_id =$2;

-- name: GetPrivateChatBetweenUsers :one
SELECT c.* FROM chats c
JOIN chat_members cm1 ON c.id = cm1.chat_id AND cm1.user_id = $1
JOIN chat_members cm2 ON c.id = cm2.chat_id AND cm2.user_id = $2
WHERE c.type = 'private' AND c.is_deleted = FALSE
LIMIT 1;

-- name: UpdateChatUpdatedAt :exec
UPDATE chats SET updated_at = NOW() WHERE id = $1;

-- name: GetChatMember :one
SELECT * FROM chat_members WHERE chat_id = $1 AND user_id = $2;

-- name: DeleteChat :exec
UPDATE chats SET is_deleted = TRUE WHERE id = $1;





