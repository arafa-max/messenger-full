package redis

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

// Connect parses Redis url, creates client and verifies connection 5s timeout
func Connect(url string) (*Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	rdb := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &Client{rdb: rdb}, nil
}
func (c *Client) Close() error {
	return c.rdb.Close()
}
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}
func (c *Client) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}
func (c *Client) Delete(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}
func (c *Client) Publish(ctx context.Context, channel string, message interface{}) error {
	return c.rdb.Publish(ctx, channel, message).Err()
}
func (c *Client) Subscribe(ctx context.Context, channel string) *redis.PubSub {
	return c.rdb.Subscribe(ctx, channel)
}
func (c *Client) LPush(ctx context.Context, key string, values ...interface{}) error {
	return c.rdb.LPush(ctx, key, values...).Err()
}
func (c *Client) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.rdb.LRange(ctx, key, start, stop).Result()
}
func (c *Client) LTrim(ctx context.Context, key string, start, stop int64) error {
	return c.rdb.LTrim(ctx, key, start, stop).Err()
}
func (c *Client) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return c.rdb.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err()
}
func (c *Client) ZScore(ctx context.Context, key, member string) (float64, error) {
	return c.rdb.ZScore(ctx, key, member).Result()
}
func (c *Client) ZRangeByScore(ctx context.Context, key string, min, max float64) ([]string, error) {
	return c.rdb.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min: strconv.FormatFloat(min, 'f', -1, 64),
		Max: strconv.FormatFloat(max, 'f', -1, 64),
	}).Result()
}
func (c *Client) ZRem(ctx context.Context, key string, members ...interface{}) error {
	return c.rdb.ZRem(ctx, key, members...).Err()
}
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.rdb.Exists(ctx, key).Result()
	return n > 0, err
}
func (c *Client) SetOnline(ctx context.Context, userID string) error {
	key := "online:" + userID
	now := time.Now().Unix()
	c.rdb.Set(ctx, "last_seen:"+userID, now, 30*24*time.Hour)
	return c.rdb.Set(ctx, key, now, 5*time.Minute).Err()
}
func (c *Client) SetOffline(ctx context.Context, userID string) error {
	key := "online:" + userID
	c.rdb.Set(ctx, "last_seen:"+userID, time.Now().Unix(), 30*24*time.Hour)
	return c.rdb.Del(ctx, key).Err()
}
func (c *Client) IsOnline(ctx context.Context, userID string) (bool, error) {
	return c.Exists(ctx, "online:"+userID)
}
func (c *Client) GetLastseen(ctx context.Context, userID string) (time.Time, error) {
	val, err := c.rdb.Get(ctx, "last_seen:"+userID).Int64()
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(val, 0), nil
}
func (c *Client) GetProfile(ctx context.Context, userID string) (string, error) {
	return c.rdb.Get(ctx, "profile:"+userID).Result()
}

func (c *Client) SetProfile(ctx context.Context, userID string, data string) error {
	return c.rdb.Set(ctx, "profile:"+userID, data, 10*time.Minute).Err()
}

func (c *Client) InvalidateProfile(ctx context.Context, userID string) error {
	return c.rdb.Del(ctx, "profile:"+userID).Err()
}
func (c *Client) PSubscribe(ctx context.Context, patterns ...string) *redis.PubSub {
    return c.rdb.PSubscribe(ctx, patterns...)
}