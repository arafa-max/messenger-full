-- name: LinkOAuthAccount :exec
INSERT INTO oauth_accounts (user_id, provider, provider_id, email, avatar_url)
VALUES (@user_id, @provider, @provider_id, @email, @avatar_url)
ON CONFLICT (provider, provider_id) DO UPDATE
    SET email      = EXCLUDED.email,
        avatar_url = EXCLUDED.avatar_url;

-- name: GetUserByOAuth :one
SELECT u.* FROM users u
JOIN oauth_accounts oa ON oa.user_id = u.id
WHERE oa.provider = @provider AND oa.provider_id = @provider_id
LIMIT 1;

-- name: GetOAuthAccounts :many
SELECT provider, provider_id, email, avatar_url, created_at
FROM oauth_accounts
WHERE user_id = @user_id
ORDER BY created_at;

-- name: UnlinkOAuthAccount :exec
DELETE FROM oauth_accounts
WHERE user_id = @user_id AND provider = @provider;