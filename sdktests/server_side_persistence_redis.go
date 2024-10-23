package sdktests

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
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

func (r *RedisPersistentStore) Get(prefix, key string) (o.Maybe[string], error) {
	var ctx = context.Background()
	result, err := r.redis.Get(ctx, prefix+":"+key).Result()
	if err == redis.Nil || err != nil {
		return o.None[string](), err
	}

	return o.Some(result), nil
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
