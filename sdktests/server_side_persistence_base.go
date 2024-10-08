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

func doServerSidePersistenceTests(t *ldtest.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	newServerSidePersistenceTests(t, &RedisPersistenceStore{redis: rdb}).Run(t)
}

type ServerSidePersistenceTests struct {
	persistence  PersistenceStore
	initialFlags map[string]string
}

func newServerSidePersistenceTests(t *ldtest.T, persistence PersistenceStore) *ServerSidePersistenceTests {
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

	return &ServerSidePersistenceTests{
		persistence:  persistence,
		initialFlags: initialFlags,
	}
}

func (s *ServerSidePersistenceTests) Run(t *ldtest.T) {
	t.Run("uses default prefix", s.usesDefaultPrefix)
	t.Run("uses custom prefix", s.usesCustomPrefix)

	t.Run("daemon mode", s.doDaemonModeTests)
}

func (s *ServerSidePersistenceTests) usesDefaultPrefix(t *ldtest.T) {
	require.NoError(t, s.persistence.Reset())
	require.NoError(t, s.persistence.WriteData("launchdarkly:features", s.initialFlags))

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistenceStore{
		Type: servicedef.Redis,
		DSN:  s.persistence.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigPersistenceCache{
		Mode: servicedef.Off,
	})

	client := NewSDKClient(t, persistence)
	pollUntilFlagValueUpdated(t, client, "flag-key", ldcontext.New("user-key"),
		ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))
}

func (s *ServerSidePersistenceTests) usesCustomPrefix(t *ldtest.T) {
	require.NoError(t, s.persistence.Reset())
	customPrefix := "custom-prefix"

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistenceStore{
		Type:   servicedef.Redis,
		DSN:    s.persistence.DSN(),
		Prefix: customPrefix,
	})
	persistence.SetCache(servicedef.SDKConfigPersistenceCache{
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

	require.NoError(t, s.persistence.WriteData(customPrefix+":features", s.initialFlags))

	pollUntilFlagValueUpdated(t, client, "flag-key", ldcontext.New("user-key"),
		ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))
}
