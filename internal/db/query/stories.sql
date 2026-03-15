-- name: CreateStory :one
INSERT INTO stories (user_id, media_url, thumbnail_url, type, caption, audience, media_id, sticker_data, music_data)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetActiveStories :many
SELECT s.*, u.username, u.avatar_url
FROM stories s
JOIN users u ON u.id = s.user_id
WHERE s.expires_at > NOW()
AND s.is_archived = FALSE
AND s.user_id = $1
ORDER BY s.created_at DESC;

-- name: GetStoriesFeed :many
SELECT s.*, u.username, u.avatar_url
FROM stories s
JOIN users u ON u.id = s.user_id
WHERE s.expires_at > NOW()
AND s.is_archived = FALSE
AND (
    s.audience = 'everyone'
    OR (s.audience = 'close_friends' AND EXISTS (
        SELECT 1 FROM close_friends cf
        WHERE cf.user_id = s.user_id AND cf.friend_id = sqlc.arg(viewer_id)
    ))
)
ORDER BY s.created_at DESC
LIMIT 100;

-- name: ViewStory :exec
INSERT INTO story_views (story_id, user_id)
VALUES ($1, $2)
ON CONFLICT (story_id, user_id) DO NOTHING;

-- name: GetStoryViewers :many
SELECT u.id, u.username, u.avatar_url, sv.viewed_at
FROM story_views sv
JOIN users u ON u.id = sv.user_id
WHERE sv.story_id = $1
ORDER BY sv.viewed_at DESC;

-- name: IncrementStoryViews :exec
UPDATE stories SET views_count = views_count + 1 WHERE id = $1;

-- name: ReactToStory :exec
INSERT INTO story_reactions (story_id, user_id, emoji)
VALUES ($1, $2, $3)
ON CONFLICT (story_id, user_id) DO UPDATE SET emoji = $3;

-- name: GetStoryReactions :many
SELECT sr.emoji, COUNT(*) as count
FROM story_reactions sr
WHERE sr.story_id = $1
GROUP BY sr.emoji
ORDER BY count DESC;

-- name: ArchiveStory :exec
UPDATE stories SET is_archived = TRUE WHERE id = $1 AND user_id = $2;

-- name: GetArchivedStories :many
SELECT * FROM stories
WHERE user_id = $1 AND is_archived = TRUE
ORDER BY created_at DESC;

-- name: DeleteStory :exec
DELETE FROM stories WHERE id = $1 AND user_id = $2;

-- name: AddCloseFriend :exec
INSERT INTO close_friends (user_id, friend_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveCloseFriend :exec
DELETE FROM close_friends WHERE user_id = $1 AND friend_id = $2;

-- name: GetCloseFriends :many
SELECT u.id, u.username, u.avatar_url
FROM close_friends cf
JOIN users u ON u.id = cf.friend_id
WHERE cf.user_id = $1
ORDER BY cf.created_at DESC;

-- name: CleanupExpiredStories :exec
UPDATE stories
SET is_archived = TRUE
WHERE expires_at < NOW() AND is_archived = FALSE;