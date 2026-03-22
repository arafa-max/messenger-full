package redis

import (
	"context"
	"time"
)

// IncrExpire атомарно инкрементирует счётчик и устанавливает TTL если ключ новый.
// Возвращает новое значение счётчика.
func (c *Client) IncrExpire(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := c.rdb.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

// SetEx сохраняет значение с TTL (алиас для единообразия с antispam)
func (c *Client) SetEx(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

// TTL возвращает оставшееся время жизни ключа
func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.rdb.TTL(ctx, key).Result()
}