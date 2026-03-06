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