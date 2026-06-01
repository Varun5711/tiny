// Package cache implements a multi-tier caching strategy for the URL shortener.
//
// The design uses two cache tiers to balance latency and durability:
//
//   - L1 (in-process): A bounded LRU cache that serves reads in nanoseconds
//     with zero network overhead. Because it lives in the process, it is lost on
//     restart, but that is acceptable since L2 provides persistence.
//
//   - L2 (Redis): A shared, TTL-based cache that survives process restarts and
//     is visible to every replica. Reads hit L2 only when L1 misses, keeping
//     Redis traffic proportional to the miss rate rather than total QPS.
//
// On a Get, the tiers are queried L1 then L2. An L2 hit automatically backfills
// L1, so repeated lookups for the same short code converge to L1 speed after
// the first access. Writes always update both tiers to keep them consistent.
//
// This two-tier approach is especially effective for URL shorteners because a
// small number of "hot" short codes (recently created or viral) account for the
// vast majority of redirect traffic, and LRU naturally retains those entries.
package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache is a multi-tier cache combining a fast in-process L1 (LRU) with a
// shared L2 (Redis). The two tiers are kept consistent on writes; reads
// cascade from L1 to L2, backfilling L1 on an L2 hit so subsequent reads
// are served from memory without a network round-trip.
type Cache struct {
	l1Cache *LRUCache    // in-process LRU -- nanosecond reads, bounded by capacity
	l2Cache *redis.Client // shared Redis -- millisecond reads, bounded by TTL
	l2TTL   time.Duration // expiry applied to every L2 entry
}

// NewMultiTierCache constructs a Cache with the given L1 capacity and L2 Redis
// backend. l1Capacity controls how many entries the in-process LRU retains;
// l2TTL controls how long entries survive in Redis before automatic expiry.
func NewMultiTierCache(l1Capacity int, redisClient *redis.Client, l2TTL time.Duration) *Cache {
	return &Cache{
		l1Cache: NewLRUCache(l1Capacity),
		l2Cache: redisClient,
		l2TTL:   l2TTL,
	}
}

// Get looks up a key, cascading through the tiers:
//
//  1. L1 (LRU) -- if found, return immediately (no network cost).
//  2. L2 (Redis) -- if found, backfill L1 so the next read is faster, then return.
//  3. Miss -- return ("", false) so the caller knows to fetch from the database.
//
// The backfill step is what makes the multi-tier strategy self-warming: the
// first request after a cold start pays the Redis RTT, but every subsequent
// request for the same key is served from process memory.
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

// Set writes a value to both tiers. L1 is updated first (in-process, cannot
// fail) so the entry is immediately available to subsequent in-process reads.
// The L2 write applies the configured TTL so stale entries are eventually
// reaped even if the application never explicitly deletes them.
func (c *Cache) Set(ctx context.Context, key string, value string) error {
	c.l1Cache.Set(key, value)
	return c.l2Cache.Set(ctx, key, value, c.l2TTL).Err()
}

// Delete removes a key from both tiers. Both tiers are invalidated even if one
// fails, because serving stale data after an explicit delete is worse than a
// cache miss.
func (c *Cache) Delete(ctx context.Context, key string) error {
	c.l1Cache.Delete(key)
	return c.l2Cache.Del(ctx, key).Err()
}

// GetJSON is a convenience wrapper around Get that deserializes the cached
// string as JSON into dest. It returns (true, nil) on a hit with successful
// deserialization, (false, nil) on a miss, or (false, err) if the cached value
// is not valid JSON for the target type.
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

// SetJSON is a convenience wrapper around Set that serializes value as JSON
// before storing it. This keeps the JSON encode/decode logic in one place and
// ensures that all structured objects are stored in a uniform format.
func (c *Cache) SetJSON(ctx context.Context, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return c.Set(ctx, key, string(data))
}
