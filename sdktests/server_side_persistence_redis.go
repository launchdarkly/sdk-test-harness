package sdktests

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type RedisPersistentStore struct {
	redis *redis.Client
}

// {{{ PersistentStore implementation

func (r RedisPersistentStore) DSN() string {
	return fmt.Sprintf("redis://%s", r.redis.Options().Addr)
}

func (r *RedisPersistentStore) Reset() error {
	var ctx = context.Background()
	return r.redis.FlushAll(ctx).Err()
}

func (r *RedisPersistentStore) WriteData(key string, data map[string]string) error {
	var ctx = context.Background()
	_, err := r.redis.HSet(ctx, key, data).Result()
	return err
}

// }}}
