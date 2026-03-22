package redis

// Добавить эти методы в internal/redis/redis.go
// к существующему Client

import (
	"context"
	"time"
)

// SetOAuthState сохраняет state для CSRF защиты OAuth флоу
func (c *Client) SetOAuthState(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

// GetOAuthState получает и возвращает state
func (c *Client) GetOAuthState(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

// DelOAuthState удаляет state после использования (one-time use)
func (c *Client) DelOAuthState(ctx context.Context, key string) {
	c.rdb.Del(ctx, key)
}