package sdktests

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	c "github.com/hashicorp/consul/api"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

func doServerSidePersistentTests(t *ldtest.T) {
	if t.Capabilities().Has(servicedef.CapabilityPersistentDataStoreRedis) {
		rdb := redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "", // no password set
			DB:       0,  // use default DB
		})

		t.Run("redis", newServerSidePersistentTests(t, &RedisPersistentStore{redis: rdb}).Run)
	}

	if t.Capabilities().Has(servicedef.CapabilityPersistentDataStoreConsul) {
		config := c.DefaultConfig()
		config.Address = "localhost:8500"

		consul, err := c.NewClient(config)
		require.NoError(t, err)

		t.Run("consul", newServerSidePersistentTests(t, &ConsulPersistentStore{consul: consul}).Run)
	}
}

type PersistentStore interface {
	DSN() string

	Get(prefix, key string) (string, bool, error)
	GetMap(prefix, key string) (map[string]string, error)
	WriteMap(prefix, key string, data map[string]string) error

	Type() servicedef.SDKConfigPersistentType

	Reset() error
}

type ServerSidePersistentTests struct {
	CommonStreamingTests
	persistentStore PersistentStore
	initialFlags    map[string]string
}

func newServerSidePersistentTests(t *ldtest.T, persistentStore PersistentStore) *ServerSidePersistentTests {
	flagKeyBytes, err :=
		ldbuilders.NewFlagBuilder("flag-key").Version(100).
			On(true).Variations(ldvalue.String("fallthrough"), ldvalue.String("other")).
			OffVariation(1).
			FallthroughVariation(0).
			Build().MarshalJSON()
	require.NoError(t, err)

	initialFlags := map[string]string{"flag-key": string(flagKeyBytes)}

	uncachedFlagKeyBytes, err :=
		ldbuilders.NewFlagBuilder("uncached-flag-key").Version(100).
			On(true).Variations(ldvalue.String("fallthrough"), ldvalue.String("other")).
			OffVariation(1).
			FallthroughVariation(0).
			Build().MarshalJSON()
	require.NoError(t, err)

	initialFlags["uncached-flag-key"] = string(uncachedFlagKeyBytes)

	return &ServerSidePersistentTests{
		CommonStreamingTests: NewCommonStreamingTests(t, "serverSidePersistenceTests"),
		persistentStore:      persistentStore,
		initialFlags:         initialFlags,
	}
}

func (s *ServerSidePersistentTests) Run(t *ldtest.T) {
	t.Run("uses default prefix", s.usesDefaultPrefix)
	t.Run("uses custom prefix", s.usesCustomPrefix)

	t.Run("daemon mode", s.doDaemonModeTests)
	t.Run("read-write", s.doReadWriteTests)
}

func (s *ServerSidePersistentTests) usesDefaultPrefix(t *ldtest.T) {
	require.NoError(t, s.persistentStore.Reset())
	require.NoError(t, s.persistentStore.WriteMap("launchdarkly", "features", s.initialFlags))

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  s.persistentStore.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.CacheModeOff,
	})

	client := NewSDKClient(t, persistence)
	pollUntilFlagValueUpdated(t, client, "flag-key", ldcontext.New("user-key"),
		ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))
}

func (s *ServerSidePersistentTests) usesCustomPrefix(t *ldtest.T) {
	require.NoError(t, s.persistentStore.Reset())
	customPrefix := "custom-prefix"

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type:   s.persistentStore.Type(),
		DSN:    s.persistentStore.DSN(),
		Prefix: customPrefix,
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.CacheModeOff,
	})

	client := NewSDKClient(t, persistence)

	require.Never(
		t,
		checkForUpdatedValue(t, client, "flag-key", ldcontext.New("user-key"),
			ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
		time.Millisecond*100,
		time.Millisecond*20,
		"flag value was updated, but it should not have been",
	)

	require.NoError(t, s.persistentStore.WriteMap(customPrefix, "features", s.initialFlags))

	pollUntilFlagValueUpdated(t, client, "flag-key", ldcontext.New("user-key"),
		ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))
}
