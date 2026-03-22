-- name: SaveIdentityKey :one
INSERT INTO identity_keys (user_id, device_id, public_key, registration_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, device_id) DO UPDATE
    SET public_key = EXCLUDED.public_key,
        registration_id = EXCLUDED.registration_id
RETURNING *;

-- name: GetIdentityKey :one
SELECT * FROM identity_keys
WHERE user_id = $1 AND device_id IS NOT DISTINCT FROM $2;

-- name: GetIdentityKeysByUser :many
SELECT * FROM identity_keys
WHERE user_id = $1;

-- ============================================
-- SIGNED PREKEYS
-- ============================================

-- name: SaveSignedPreKey :one
INSERT INTO signed_prekeys (user_id, device_id, key_id, public_key, signature)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, device_id, key_id) DO UPDATE
    SET public_key = EXCLUDED.public_key,
        signature  = EXCLUDED.signature
RETURNING *;

-- name: GetSignedPreKey :one
SELECT * FROM signed_prekeys
WHERE user_id = $1 AND device_id IS NOT DISTINCT FROM $2
ORDER BY created_at DESC
LIMIT 1;

-- name: DeleteOldSignedPreKeys :exec
DELETE FROM signed_prekeys
WHERE user_id = $1 AND device_id IS NOT DISTINCT FROM $2
  AND key_id != $3;

-- ============================================
-- ONE-TIME PREKEYS
-- ============================================

-- name: SaveOneTimePreKey :exec
INSERT INTO one_time_prekeys (user_id, device_id, key_id, public_key)
VALUES ($1, $2, $3, $4);

-- name: GetAndUseOneTimePreKey :one
UPDATE one_time_prekeys
SET is_used = TRUE, used_at = NOW()
WHERE id = (
    SELECT otpk.id FROM one_time_prekeys otpk
    WHERE otpk.user_id = $1
      AND otpk.device_id IS NOT DISTINCT FROM $2
      AND otpk.is_used = FALSE
    ORDER BY otpk.created_at ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: CountAvailableOneTimePreKeys :one
SELECT COUNT(*) FROM one_time_prekeys
WHERE user_id = $1 AND device_id IS NOT DISTINCT FROM $2 AND is_used = FALSE;

-- ============================================
-- KEY BUNDLE
-- ============================================

-- name: SaveKeyBundle :one
INSERT INTO key_bundles (user_id, device_id, bundle, updated_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (user_id, device_id) DO UPDATE
    SET bundle     = EXCLUDED.bundle,
        updated_at = NOW()
RETURNING *;

-- name: GetKeyBundle :one
SELECT * FROM key_bundles
WHERE user_id = $1 AND device_id IS NOT DISTINCT FROM $2;

-- name: GetKeyBundlesByUser :many
SELECT * FROM key_bundles
WHERE user_id = $1;