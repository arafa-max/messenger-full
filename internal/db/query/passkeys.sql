-- name: CreatePasskey :one
INSERT INTO passkeys (user_id, credential_id, public_key, aaguid, sign_count, name)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetPasskeyByCredentialID :one
SELECT * FROM passkeys WHERE credential_id = $1;

-- name: GetPasskeysByUserID :many
SELECT * FROM passkeys WHERE user_id = $1 ORDER BY created_at DESC;

-- name: UpdatePasskeySignCount :exec
UPDATE passkeys
SET sign_count = $2, last_used_at = NOW()
WHERE credential_id = $1;

-- name: DeletePasskey :exec
DELETE FROM passkeys WHERE id = $1 AND user_id = $2;