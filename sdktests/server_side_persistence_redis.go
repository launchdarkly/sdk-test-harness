package sdktests

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type RedisPersistenceStore struct {
	redis *redis.Client
}

// {{{ PersistenceStore implementation

func (r RedisPersistenceStore) DSN() string {
	return fmt.Sprintf("redis://%s", r.redis.Options().Addr)
}

func (r *RedisPersistenceStore) Reset() error {
	var ctx = context.Background()
	return r.redis.FlushAll(ctx).Err()
}

func (r *RedisPersistenceStore) WriteData(key string, data map[string]string) error {
	var ctx = context.Background()
	_, err := r.redis.HSet(ctx, key, data).Result()
	return err
}

// }}}
