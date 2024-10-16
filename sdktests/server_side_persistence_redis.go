package sdktests

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
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

func (r *RedisPersistentStore) Get(prefix, key string) (string, bool, error) {
	var ctx = context.Background()
	result, err := r.redis.Get(ctx, prefix+":"+key).Result()
	if err == redis.Nil {
		return result, false, err
	} else if err != nil {
		return "", false, err
	}

	return result, true, nil
}

func (r *RedisPersistentStore) GetMap(prefix, key string) (map[string]string, error) {
	var ctx = context.Background()
	return r.redis.HGetAll(ctx, prefix+":"+key).Result()
}

func (r *RedisPersistentStore) WriteMap(prefix, key string, data map[string]string) error {
	var ctx = context.Background()
	_, err := r.redis.HSet(ctx, prefix+":"+key, data).Result()
	return err
}

// }}}
