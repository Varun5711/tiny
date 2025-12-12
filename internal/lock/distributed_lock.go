package lock

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrLockNotAcquired = errors.New("failed to acquire lock")
	ErrLockNotHeld     = errors.New("lock is not held")
)

type DistributedLock struct {
	client *redis.Client
	key    string
	value  string
	ttl    time.Duration
}

func NewDistributedLock(client *redis.Client, key string, ttl time.Duration) *DistributedLock {
	return &DistributedLock{
		client: client,
		key:    key,
		value:  generateLockValue(),
		ttl:    ttl,
	}
}

func (l *DistributedLock) Acquire(ctx context.Context) (bool, error) {
	result, err := l.client.SetNX(ctx, l.key, l.value, l.ttl).Result()
	if err != nil {
		return false, err
	}
	return result, nil
}

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

func generateLockValue() string {
	return time.Now().Format("20060102150405.000000")
}
