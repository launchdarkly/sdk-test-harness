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

func doServerSideDataSystemTests(t *ldtest.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	newServerSideDataSystemTests(t, RedisPersistenceStore{redis: rdb}).Run(t)
}

type ServerSideDataSystemTests struct {
	persistence  PersistenceStore
	initialFlags map[string]string
}

func newServerSideDataSystemTests(t *ldtest.T, persistence PersistenceStore) *ServerSideDataSystemTests {
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

	return &ServerSideDataSystemTests{
		persistence:  persistence,
		initialFlags: initialFlags,
	}
}

func (s *ServerSideDataSystemTests) Run(t *ldtest.T) {
	t.Run("uses default prefix", s.usesDefaultPrefix)
	t.Run("uses custom prefix", s.usesCustomPrefix)

	t.Run("read-only", s.doReadOnlyTests)
}

func (s *ServerSideDataSystemTests) usesDefaultPrefix(t *ldtest.T) {
	require.NoError(t, s.persistence.Reset())
	require.NoError(t, s.persistence.WriteData("launchdarkly:features", s.initialFlags))

	dataSystem := NewDataSystem()
	dataSystem.AddPersistence(servicedef.SDKConfigDataSystemPersistence{
		Store: servicedef.SDKConfigDataSystemPersistenceStore{
			Type: servicedef.Redis,
			DSN:  s.persistence.DSN(),
		},
		Cache: servicedef.SDKConfigDataSystemPersistenceCache{
			Mode: servicedef.Off,
		},
	})

	client := NewSDKClient(t, dataSystem)
	pollUntilFlagValueUpdated(t, client, "flag-key", ldcontext.New("user-key"),
		ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))
}

func (s *ServerSideDataSystemTests) usesCustomPrefix(t *ldtest.T) {
	require.NoError(t, s.persistence.Reset())
	customPrefix := "custom-prefix"

	dataSystem := NewDataSystem()
	dataSystem.AddPersistence(servicedef.SDKConfigDataSystemPersistence{
		Store: servicedef.SDKConfigDataSystemPersistenceStore{
			Type:   servicedef.Redis,
			Prefix: customPrefix,
			DSN:    s.persistence.DSN(),
		},
		Cache: servicedef.SDKConfigDataSystemPersistenceCache{
			Mode: servicedef.Off,
		},
	})

	client := NewSDKClient(t, dataSystem)

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
