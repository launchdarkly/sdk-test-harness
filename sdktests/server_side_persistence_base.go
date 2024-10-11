package sdktests

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

func doServerSidePersistentTests(t *ldtest.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	newServerSidePersistentTests(t, &RedisPersistentStore{redis: rdb}).Run(t)
}

type PersistentStore interface {
	DSN() string

	WriteData(key string, data map[string]string) error

	Reset() error
}

type ServerSidePersistentTests struct {
	persistentStore PersistentStore
	initialFlags    map[string]string
}

func newServerSidePersistentTests(t *ldtest.T, persistentStore PersistentStore) *ServerSidePersistentTests {
	flagKeyBytes, err :=
		ldbuilders.NewFlagBuilder("flag-key").Version(1).
			On(true).Variations(ldvalue.String("off"), ldvalue.String("match"), ldvalue.String("fallthrough")).
			OffVariation(0).
			FallthroughVariation(2).
			Build().MarshalJSON()
	require.NoError(t, err)

	initialFlags := map[string]string{"flag-key": string(flagKeyBytes)}

	uncachedFlagKeyBytes, err :=
		ldbuilders.NewFlagBuilder("uncached-flag-key").Version(1).
			On(true).Variations(ldvalue.String("off"), ldvalue.String("match"), ldvalue.String("fallthrough")).
			OffVariation(0).
			FallthroughVariation(2).
			Build().MarshalJSON()
	require.NoError(t, err)

	initialFlags["uncached-flag-key"] = string(uncachedFlagKeyBytes)

	return &ServerSidePersistentTests{
		persistentStore: persistentStore,
		initialFlags:    initialFlags,
	}
}

func (s *ServerSidePersistentTests) Run(t *ldtest.T) {
	t.Run("uses default prefix", s.usesDefaultPrefix)
	t.Run("uses custom prefix", s.usesCustomPrefix)

	t.Run("daemon mode", s.doDaemonModeTests)
}

func (s *ServerSidePersistentTests) usesDefaultPrefix(t *ldtest.T) {
	require.NoError(t, s.persistentStore.Reset())
	require.NoError(t, s.persistentStore.WriteData("launchdarkly:features", s.initialFlags))

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: servicedef.Redis,
		DSN:  s.persistentStore.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.Off,
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
		Type:   servicedef.Redis,
		DSN:    s.persistentStore.DSN(),
		Prefix: customPrefix,
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.Off,
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

	require.NoError(t, s.persistentStore.WriteData(customPrefix+":features", s.initialFlags))

	pollUntilFlagValueUpdated(t, client, "flag-key", ldcontext.New("user-key"),
		ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))
}
