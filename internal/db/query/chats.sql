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

-- name: BanChatMember :exec
UPDATE chat_members SET is_banned = TRUE WHERE chat_id = $1 AND user_id = $2;

-- name: UnbanChatMember :exec
UPDATE chat_members SET is_banned = FALSE WHERE chat_id = $1 AND user_id = $2;

-- name: MuteChatMember :exec
UPDATE chat_members SET muted_until = $3 WHERE chat_id = $1 AND user_id = $2;

-- name: UnmuteChatMember :exec
UPDATE chat_members SET muted_until = NULL WHERE chat_id = $1 AND user_id = $2;

-- name: CreateInviteLink :one
INSERT INTO invite_links (code, chat_id, created_by, max_uses, expires_at)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetInviteByCode :one
SELECT * FROM invite_links WHERE code = $1 AND is_revoked = FALSE;

-- name: IncrementInviteUses :exec
UPDATE invite_links SET uses_count = uses_count + 1 WHERE code = $1;

-- name: RevokeInviteLink :exec
UPDATE invite_links SET is_revoked = TRUE WHERE code = $1 AND chat_id = $2;


-- name: GetLastMessageTime :one
SELECT created_at FROM messages
WHERE chat_id = $1 AND sender_id = $2 AND is_deleted = FALSE
ORDER BY created_at DESC
LIMIT 1;

-- name: GetChatSlowMode :one
SELECT slow_mode FROM chats WHERE id = $1;

-- name: CreateFolder :one
INSERT INTO chat_folders (user_id, name, emoji, position)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetMyFolders :many
SELECT * FROM chat_folders WHERE user_id = $1 ORDER BY position ASC;

-- name: DeleteFolder :exec
DELETE FROM chat_folders WHERE id = $1 AND user_id = $2;

-- name: AddChatToFolder :exec
INSERT INTO chat_folder_items (folder_id, chat_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveChatFromFolder :exec
DELETE FROM chat_folder_items WHERE folder_id = $1 AND chat_id = $2;

-- name: GetFolderChats :many
SELECT c.* FROM chats c
JOIN chat_folder_items cfi ON c.id = cfi.chat_id
WHERE cfi.folder_id = $1 AND c.is_deleted = FALSE;

-- name: ArchiveChat :exec
UPDATE chat_members SET is_archived = TRUE WHERE chat_id = $1 AND user_id = $2;

-- name: UnarchiveChat :exec
UPDATE chat_members SET is_archived = FALSE WHERE chat_id = $1 AND user_id = $2;

-- name: GetArchivedChats :many
SELECT c.* FROM chats c
JOIN chat_members cm ON c.id = cm.chat_id
WHERE cm.user_id = $1 AND cm.is_archived = TRUE AND c.is_deleted = FALSE;

-- name: CreateChannel :one
INSERT INTO chats (type, name, username, description, owner_id, is_public)
VALUES ('channel', $1, $2, $3, $4, $5) RETURNING *;

-- name: GetPublicChats :many
SELECT * FROM chats
WHERE is_public = TRUE AND is_deleted = FALSE AND type = $1
ORDER BY member_count DESC
LIMIT $2 OFFSET $3;

-- name: IncrementMessageViews :exec
UPDATE messages SET views_count = views_count + 1 WHERE id = $1;

-- name: UpdateChatVisibility :exec
UPDATE chats SET is_public = $2, updated_at = NOW() WHERE id = $1;


-- name: CreateTopic :one
INSERT INTO chat_topics (chat_id, name, icon_emoji, icon_color, created_by)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetChatTopics :many
SELECT * FROM chat_topics WHERE chat_id = $1 AND is_hidden = FALSE
ORDER BY created_at ASC;

-- name: CloseTopic :exec
UPDATE chat_topics SET is_closed = TRUE WHERE id = $1 AND chat_id = $2;

-- name: DeleteTopic :exec
DELETE FROM chat_topics WHERE id = $1 AND chat_id = $2;

-- name: VerifyChat :exec
UPDATE chats SET metadata = jsonb_set(metadata, '{is_verified}', 'true') WHERE id = $1;

-- name: UnverifyChat :exec
UPDATE chats SET metadata = jsonb_set(metadata, '{is_verified}', 'false') WHERE id = $1;

-- name: CreateCommunity :one
INSERT INTO chats (type, name, username, description, owner_id, is_public)
VALUES ('community', $1, $2, $3, $4, $5) RETURNING *;

-- name: AddChatToCommunity :exec
UPDATE chats SET metadata = jsonb_set(metadata, '{community_id}', to_jsonb($2::text))
WHERE id = $1;

-- name: GetCommunityChats :many
SELECT * FROM chats
WHERE metadata->>'community_id' = $1::text AND is_deleted = FALSE;

-- name: RemoveChatFromCommunity :exec
UPDATE chats SET metadata = metadata - 'community_id' WHERE id = $1;

-- name: GetUnreadCount :one
SELECT COUNT(*)::int FROM messages m
WHERE m.chat_id = $1
AND m.sender_id != $2
AND m.is_deleted = FALSE
AND m.created_at > (
    SELECT COALESCE(cm.last_read_at, '1970-01-01')
    FROM chat_members cm
    WHERE cm.chat_id = $1 AND cm.user_id = $2
);


-- name: GetMutedChats :many
SELECT cm.chat_id FROM chat_members cm
WHERE cm.user_id = $1
AND cm.muted_until IS NOT NULL
AND cm.muted_until > NOW();

-- name: SetSlowMode :exec
UPDATE chats SET slow_mode = $2 WHERE id = $1;