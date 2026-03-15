-- name: CreatePayment :one
INSERT INTO payments (bot_id, user_id, amount, currency, description, stripe_id)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetPayment :one
SELECT * FROM payments
WHERE id = $1;

-- name: GetPaymentByStripeID :one
SELECT * FROM payments
WHERE stripe_id = $1;

-- name: UpdatePaymentStatus :exec
UPDATE payments
SET status = $2, updated_at = now()
WHERE id = $1;

-- name: GetBotPayments :many
SELECT * FROM payments
WHERE bot_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetUserPayments :many
SELECT * FROM payments
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetSubscriptionByUserID :one
SELECT * FROM subscriptions WHERE user_id = $1;

-- name: UpsertSubscription :exec
INSERT INTO subscriptions (user_id, stripe_customer_id, stripe_sub_id, plan, status)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id) DO UPDATE SET
    stripe_customer_id   = EXCLUDED.stripe_customer_id,
    stripe_sub_id        = EXCLUDED.stripe_sub_id,
    plan                 = EXCLUDED.plan,
    status               = EXCLUDED.status,
    updated_at           = now();

-- name: UpdateSubscriptionByStripeID :exec
UPDATE subscriptions SET
    status               = $2,
    current_period_end   = $3,
    cancel_at_period_end = $4,
    updated_at           = now()
WHERE stripe_sub_id = $1;

-- name: UpdateSubscriptionCancelAtPeriodEnd :exec
UPDATE subscriptions SET
    cancel_at_period_end = $2,
    updated_at           = now()
WHERE user_id = $1;

-- name: GetPremiumSettings :one
SELECT * FROM premium_settings WHERE user_id = $1;

-- name: UpsertPremiumSettings :exec
INSERT INTO premium_settings (user_id, hide_phone, away_message, away_message_enabled)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE SET
    hide_phone           = EXCLUDED.hide_phone,
    away_message         = EXCLUDED.away_message,
    away_message_enabled = EXCLUDED.away_message_enabled,
    updated_at           = now();

-- name: AddChatLabel :one
INSERT INTO chat_labels (user_id, chat_id, label, color)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, chat_id, label) DO UPDATE SET color = EXCLUDED.color
RETURNING *;

-- name: GetChatLabels :many
SELECT * FROM chat_labels WHERE user_id = $1 ORDER BY created_at DESC;

-- name: DeleteChatLabel :exec
DELETE FROM chat_labels WHERE id = $1 AND user_id = $2;