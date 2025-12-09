package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	l1Cache *LRUCache
	l2Cache *redis.Client
	l2TTL   time.Duration
}

func NewMultiTierCache(l1Capacity int, redisClient *redis.Client, l2TTL time.Duration) *Cache {
	return &Cache{
		l1Cache: NewLRUCache(l1Capacity),
		l2Cache: redisClient,
		l2TTL:   l2TTL,
	}
}

func (c *Cache) Get(ctx context.Context, key string) (string, bool) {
	if val, found := c.l1Cache.Get(key); found {
		return val.(string), true
	}

	val, err := c.l2Cache.Get(ctx, key).Result()
	if err == nil {
		c.l1Cache.Set(key, val)
		return val, true
	}

	return "", false
}

func (c *Cache) Set(ctx context.Context, key string, value string) error {
	c.l1Cache.Set(key, value)
	return c.l2Cache.Set(ctx, key, value, c.l2TTL).Err()
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	c.l1Cache.Delete(key)
	return c.l2Cache.Del(ctx, key).Err()
}

func (c *Cache) GetJSON(ctx context.Context, key string, dest interface{}) (bool, error) {
	val, found := c.Get(ctx, key)
	if !found {
		return false, nil
	}

	err := json.Unmarshal([]byte(val), dest)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (c *Cache) SetJSON(ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return c.Set(ctx, key, string(data))
}
