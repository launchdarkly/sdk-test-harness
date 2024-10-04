package sdktests

import (
	"context"
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
	"github.com/redis/go-redis/v9"
)

type RedisPersistentStore struct {
	redis *redis.Client
}

// {{{ PersistentStore implementation

func (r RedisPersistentStore) DSN() string {
	return fmt.Sprintf("redis://%s", r.redis.Options().Addr)
}

func (r *RedisPersistentStore) Type() servicedef.SDKConfigPersistentType {
	return servicedef.Redis
}

func (r *RedisPersistentStore) Reset() error {
	var ctx = context.Background()
	return r.redis.FlushAll(ctx).Err()
}

func (r *RedisPersistentStore) ReadField(key string) (string, error) {
	var ctx = context.Background()
	return r.redis.Get(ctx, key).Result()
}

func (r *RedisPersistentStore) ReadData(key string) (map[string]string, error) {
	var ctx = context.Background()
	return r.redis.HGetAll(ctx, key).Result()
}

func (r *RedisPersistentStore) WriteData(key string, data map[string]string) error {
	var ctx = context.Background()
	_, err := r.redis.HSet(ctx, key, data).Result()
	return err
}

// }}}
