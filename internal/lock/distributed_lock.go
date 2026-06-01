// Package lock implements a Redis-backed distributed lock using the classic
// SETNX + Lua-script release pattern.
//
// Why a distributed lock?
// In a multi-replica deployment of the URL shortener, certain operations must
// be serialised across replicas -- for example, generating a unique short code
// without collisions, or running a one-shot migration. A local mutex only
// protects a single process; a Redis-based lock provides mutual exclusion
// across all replicas that share the same Redis instance.
//
// Acquire uses Redis SETNX ("SET if Not eXists") with a TTL. This is an
// atomic compare-and-set: if the key does not exist the caller wins the lock;
// if it already exists the caller loses. The TTL acts as a safety net so the
// lock is automatically released if the holder crashes without calling Release.
//
// Release uses a Lua script that atomically checks the lock value and deletes
// the key only if it still matches. This prevents a dangerous race:
//
//  1. Replica A acquires the lock with value "A".
//  2. Replica A stalls (GC pause, slow I/O) and the TTL expires.
//  3. Replica B acquires the same lock with value "B".
//  4. Replica A resumes and calls DEL -- without the value check it would
//     delete Replica B's lock, breaking mutual exclusion.
//
// The Lua script runs inside Redis as a single atomic operation, so the GET
// and DEL cannot be interleaved with other commands. A plain GET-then-DEL in
// two round-trips would be subject to the same race the script prevents.
package lock

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	// ErrLockNotAcquired is returned when SETNX fails because another holder
	// already owns the lock.
	ErrLockNotAcquired = errors.New("failed to acquire lock")

	// ErrLockNotHeld is returned by Release when the Lua script finds that the
	// lock key either does not exist (TTL expired) or belongs to a different
	// holder. This signals that the caller no longer owns the lock and should
	// not assume its critical section was protected.
	ErrLockNotHeld = errors.New("lock is not held")
)

// DistributedLock represents a single lock instance bound to a Redis key.
// Each instance carries a unique value so that Release can verify ownership
// before deleting the key (see the Lua script in Release).
type DistributedLock struct {
	client *redis.Client  // shared Redis connection (from the redis package)
	key    string         // Redis key used as the lock (e.g., "lock:shortcode:abc")
	value  string         // unique token written by Acquire, checked by Release
	ttl    time.Duration  // automatic expiry -- a safety net against holder crashes
}

// NewDistributedLock creates a lock for the given key with an automatic expiry
// of ttl. The unique lock value is generated at construction time so that
// Acquire and Release always agree on the ownership token.
func NewDistributedLock(client *redis.Client, key string, ttl time.Duration) *DistributedLock {
	return &DistributedLock{
		client: client,
		key:    key,
		value:  generateLockValue(),
		ttl:    ttl,
	}
}

// Acquire attempts to take the lock using Redis SETNX (SET if Not eXists).
// It returns (true, nil) if the lock was acquired, (false, nil) if another
// holder already owns it, or (false, err) on a Redis communication failure.
//
// The TTL ensures the lock auto-expires if the holder crashes or forgets to
// call Release. Callers should choose a TTL that is comfortably longer than
// the expected critical section but short enough that a crashed holder does
// not block others for an unreasonable time.
func (l *DistributedLock) Acquire(ctx context.Context) (bool, error) {
	result, err := l.client.SetNX(ctx, l.key, l.value, l.ttl).Result()
	if err != nil {
		return false, err
	}
	return result, nil
}

// Release frees the lock, but only if this instance still owns it.
//
// A Lua script is used instead of a simple DEL to make the check-and-delete
// atomic. Without the script, a race exists where another replica could
// acquire the lock between our GET and DEL, and our DEL would silently
// destroy the new holder's lock. Because Redis executes the entire Lua script
// as a single command, no other client can interleave between the GET and DEL.
//
// Returns nil on success, ErrLockNotHeld if the lock was already released or
// taken by another holder, or a Redis error on communication failure.
func (l *DistributedLock) Release(ctx context.Context) error {
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`

	result, err := l.client.Eval(ctx, script, []string{l.key}, l.value).Result()
	if err != nil {
		return err
	}

	if result.(int64) == 0 {
		return ErrLockNotHeld
	}

	return nil
}

// generateLockValue produces a timestamp-based token that is unique enough for
// single-Redis deployments. In a high-contention or multi-Redis (Redlock)
// scenario this should be replaced with a UUID or crypto-random string.
func generateLockValue() string {
	return time.Now().Format("20060102150405.000000")
}
