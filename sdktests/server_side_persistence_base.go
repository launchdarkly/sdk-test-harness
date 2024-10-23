package sdktests

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	consul "github.com/hashicorp/consul/api"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

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

		t.Run("redis", newServerSidePersistentTests(t, &RedisPersistentStore{redis: rdb}, "launchdarkly").Run)
	}

	if t.Capabilities().Has(servicedef.CapabilityPersistentDataStoreConsul) {
		config := consul.DefaultConfig()
		config.Address = "localhost:8500"

		consul, err := consul.NewClient(config)
		require.NoError(t, err)

		t.Run("consul", newServerSidePersistentTests(t, &ConsulPersistentStore{consul: consul}, "launchdarkly").Run)
	}

	if t.Capabilities().Has(servicedef.CapabilityPersistentDataStoreDynamoDB) {
		mySession := session.Must(session.NewSession(
			aws.NewConfig().
				WithRegion("us-east-1").
				WithEndpoint("http://localhost:8000").
				WithCredentials(
					credentials.NewStaticCredentials(
						"dummy",
						"dummy",
						"dummy",
					),
				),
		))

		store := DynamoDBPersistentStore{dynamodb: dynamodb.New(mySession)}
		store.Reset()

		t.Run("dynamodb", newServerSidePersistentTests(t, &store, "").Run)
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
	defaultPrefix   string
	persistentStore PersistentStore
	initialFlags    map[string]string
}

func newServerSidePersistentTests(t *ldtest.T, persistentStore PersistentStore, defaultPrefix string) *ServerSidePersistentTests {
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
		defaultPrefix:        defaultPrefix,
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
	require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

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
		Prefix: o.Some(customPrefix),
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
