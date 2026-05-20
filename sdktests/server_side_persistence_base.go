package sdktests

// NOTE: You may note that these tests do not follow the same pattern as the
// other tests in this repository.
//
// Historically, tests in this repository have suffered from onion-like
// nesting. The further you have to dig, the more you cry.
//
// In the time honored tradition of every other language on the planet, we are
// going to write these tests in a flat manner where the setups and
// dependencies are explicitly passed in.

import (
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	consul "github.com/hashicorp/consul/api"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

const (
	persistenceInitedKey = "$inited"
)

// PS 1.3, 1.4: dispatches persistence tests across Redis, Consul, and DynamoDB backends
func doServerSidePersistentTests(t *ldtest.T) {
	ranAtLeastOnce := false

	if t.Capabilities().Has(servicedef.CapabilityPersistentDataStoreRedis) {
		ranAtLeastOnce = true
		rdb := redis.NewClient(&redis.Options{
			Addr:     "localhost:6379",
			Password: "", // no password set
			DB:       0,  // use default DB
		})

		t.Run("redis", newServerSidePersistentTests(t, &RedisPersistentStore{redis: rdb}, "launchdarkly").Run)
	}

	if t.Capabilities().Has(servicedef.CapabilityPersistentDataStoreConsul) {
		ranAtLeastOnce = true
		config := consul.DefaultConfig()
		config.Address = "localhost:8500"

		consul, err := consul.NewClient(config)
		require.NoError(t, err)

		t.Run("consul", newServerSidePersistentTests(t, &ConsulPersistentStore{consul: consul}, "launchdarkly").Run)
	}

	if t.Capabilities().Has(servicedef.CapabilityPersistentDataStoreDynamoDB) {
		ranAtLeastOnce = true
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
		err := store.Reset()
		require.NoError(t, err)

		t.Run("dynamodb", newServerSidePersistentTests(t, &store, "").Run)
	}

	if !ranAtLeastOnce {
		t.Skip()
	}
}

type PersistentStore interface {
	DSN() string

	Get(prefix, key string) (o.Maybe[string], error)
	GetMap(prefix, key string) (map[string]string, error)

	Write(prefix, key, data string) error
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

func newServerSidePersistentTests(
	t *ldtest.T, persistentStore PersistentStore, defaultPrefix string,
) *ServerSidePersistentTests {
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
			On(true).Variations(ldvalue.String("uncached-fallthrough"), ldvalue.String("other")).
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

// PS 1.1.2: cache TTL modes (off, positive, infinite)
// PS 1.1.3: store initialized with Init() containing all flag data
// PS 1.1.4: Get retrieves items from persistence when not cached
// PS 1.1.6: Upsert writes items to persistent store with version checking
// PS 1.1.6.1: successful upsert updates item cache (non-infinite TTL)
// PS 1.1.6.5: successful upsert updates item and all-items cache (infinite TTL)
// PS 1.1.7: IsInitialized gates reads until store is initialized
// PS 1.2.1: cached items expire after TTL elapses
func (s *ServerSidePersistentTests) Run(t *ldtest.T) {
	s.runWithEmptyStore(t, "uses default prefix", func(t *ldtest.T) {
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
	})

	s.runWithEmptyStore(t, "uses custom prefix", func(t *ldtest.T) {
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

		h.RequireNever(
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
	})

	databaseModes := map[string]servicedef.DataStoreMode{
		"read-write": servicedef.DataStoreModeReadWrite,
		"read":       servicedef.DataStoreModeRead,
	}

	updatedFlagKeyBytes, err :=
		ldbuilders.NewFlagBuilder("flag-key").Version(200).
			On(true).Variations(ldvalue.String("updated"), ldvalue.String("other")).
			OffVariation(1).
			FallthroughVariation(0).
			Build().MarshalJSON()
	require.NoError(t, err)
	updatedFlags := map[string]string{"flag-key": string(updatedFlagKeyBytes)}

	for desc, mode := range databaseModes {
		context := ldcontext.New("user-key")

		persistence := NewPersistence()
		persistence.SetStore(servicedef.SDKConfigPersistentStore{
			Type: s.persistentStore.Type(),
			DSN:  s.persistentStore.DSN(),
		})
		persistence.SetStoreMode(mode)

		t.Run(fmt.Sprintf("store mode %s - no data source", desc), func(t *ldtest.T) {
			s.runWithEmptyStore(t, "no cache - shows changes immediately", func(t *ldtest.T) {
				persistence.SetCache(servicedef.SDKConfigPersistentCache{
					Mode: servicedef.CacheModeOff,
				})

				client := NewSDKClient(t, persistence)

				require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))
				response := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      "flag-key",
					Context:      o.Some(context),
					ValueType:    servicedef.ValueTypeAny,
					DefaultValue: ldvalue.String("default"),
				})
				m.In(t).Assert(response.Value, m.Equal(ldvalue.String("fallthrough")))

				require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", updatedFlags))
				response = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      "flag-key",
					Context:      o.Some(context),
					ValueType:    servicedef.ValueTypeAny,
					DefaultValue: ldvalue.String("default"),
				})
				m.In(t).Assert(response.Value, m.Equal(ldvalue.String("updated")))
			})

			s.runWithEmptyStore(t, "ttl cache - shows changes eventually", func(t *ldtest.T) {
				persistence.SetCache(servicedef.SDKConfigPersistentCache{
					Mode: servicedef.CacheModeTTL,
					TTL:  o.Some(1),
				})

				client := NewSDKClient(t, persistence)

				require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))
				response := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      "flag-key",
					Context:      o.Some(context),
					ValueType:    servicedef.ValueTypeAny,
					DefaultValue: ldvalue.String("default"),
				})
				m.In(t).Assert(response.Value, m.Equal(ldvalue.String("fallthrough")))

				require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", updatedFlags))
				h.RequireNever(t,
					checkForUpdatedValue(t, client, "flag-key", context,
						ldvalue.String("fallthrough"), ldvalue.String("updated"), ldvalue.String("default")),
					time.Millisecond*250, time.Millisecond*20, "flag was updated before ttl expired")

				h.RequireEventually(t,
					checkForUpdatedValue(t, client, "flag-key", context,
						ldvalue.String("fallthrough"), ldvalue.String("updated"), ldvalue.String("default")),
					time.Second*1, time.Millisecond*20, "flag was not updated after ttl expired")
			})

			s.runWithEmptyStore(t, "infinite cache - shows changes never", func(t *ldtest.T) {
				persistence.SetCache(servicedef.SDKConfigPersistentCache{
					Mode: servicedef.CacheModeInfinite,
				})

				client := NewSDKClient(t, persistence)

				require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))
				response := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      "flag-key",
					Context:      o.Some(context),
					ValueType:    servicedef.ValueTypeAny,
					DefaultValue: ldvalue.String("default"),
				})
				m.In(t).Assert(response.Value, m.Equal(ldvalue.String("fallthrough")))

				require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", updatedFlags))
				h.RequireNever(t,
					checkForUpdatedValue(t, client, "flag-key", context,
						ldvalue.String("fallthrough"), ldvalue.String("updated"), ldvalue.String("default")),
					time.Millisecond*1_250, time.Millisecond*20, "flag was updated despite infinite cache")
			})
		})

		blockingEndpoint := func(closeWhenReady <-chan bool, handler http.Handler) *harness.MockEndpoint {
			return requireContext(t).harness.NewMockEndpoint(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					<-closeWhenReady
					handler.ServeHTTP(w, r)
				}),
				t.DebugLogger(),
				harness.MockEndpointDescription("blocking endpoint"),
			)
		}

		t.Run(fmt.Sprintf("store mode %s - with data source", desc), func(t *ldtest.T) {
			cacheConfigs := map[string]servicedef.SDKConfigPersistentCache{
				"no cache":       {Mode: servicedef.CacheModeOff},
				"ttl cache":      {Mode: servicedef.CacheModeTTL, TTL: o.Some(1)},
				"infinite cache": {Mode: servicedef.CacheModeInfinite},
			}
			for cacheDesc, cacheConfig := range cacheConfigs {
				s.runWithEmptyStore(t, fmt.Sprintf("%s - ignores database until init key is set", cacheDesc), func(t *ldtest.T) {
					data := mockld.NewServerSDKDataBuilder().Flag(s.makeServerSideFlag("flag-key", 1, updatedValue)).Build()
					dataSystem := NewSDKDataSystem(t, data)

					persistence.SetCache(cacheConfig)

					closeWhenReady := make(chan bool)
					endpoint := blockingEndpoint(closeWhenReady, dataSystem.Synchronizers[0].streaming)

					client := NewSDKClient(t,
						persistence,
						WithWaitToStart(time.Millisecond, true),
						WithStreamingSynchronizer(baseStreamConfig(endpoint)))

					require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

					// Since we never wrote the database init key, this
					// evaluation returns "default" instead of "fallthrough"
					response := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
						FlagKey:      "flag-key",
						Context:      o.Some(context),
						ValueType:    servicedef.ValueTypeAny,
						DefaultValue: ldvalue.String("default"),
					})
					m.In(t).Assert(response.Value, m.Equal(ldvalue.String("default")))

					require.NoError(t, s.persistentStore.Write(s.defaultPrefix, persistenceInitedKey, "1"))

					switch cacheConfig.Mode {
					case servicedef.CacheModeOff:
						response = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
							FlagKey:      "flag-key",
							Context:      o.Some(context),
							ValueType:    servicedef.ValueTypeAny,
							DefaultValue: ldvalue.String("default"),
						})
						m.In(t).Assert(response.Value, m.Equal(ldvalue.String("fallthrough")))
					case servicedef.CacheModeTTL:
						h.RequireNever(t,
							checkForUpdatedValue(t, client, "flag-key", context,
								ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
							time.Millisecond*500, time.Millisecond*20, "flag was incorrectly updated")
						h.RequireEventually(t,
							checkForUpdatedValue(t, client, "flag-key", context,
								ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
							time.Millisecond*750, time.Millisecond*20, "flag was never updated")
					case servicedef.CacheModeInfinite:
						h.RequireNever(t,
							checkForUpdatedValue(t, client, "flag-key", context,
								ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
							time.Millisecond*1_250, time.Millisecond*20, "flag was never updated")
					}

					close(closeWhenReady)
				})

				s.runWithEmptyStore(t, fmt.Sprintf("%s - ignores database when ds sends data", cacheDesc), func(t *ldtest.T) {
					data := mockld.NewServerSDKDataBuilder().Flag(s.makeServerSideFlag("flag-key", 1, updatedValue)).Build()
					dataSystem := NewSDKDataSystem(t, data)

					persistence.SetCache(cacheConfig)

					closeWhenReady := make(chan bool)
					endpoint := blockingEndpoint(closeWhenReady, dataSystem.Synchronizers[0].streaming)

					require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))
					require.NoError(t, s.persistentStore.Write(s.defaultPrefix, persistenceInitedKey, "1"))

					client := NewSDKClient(t,
						persistence,
						WithWaitToStart(time.Millisecond, true),
						WithStreamingSynchronizer(baseStreamConfig(endpoint)))

					response := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
						FlagKey:      "flag-key",
						Context:      o.Some(context),
						ValueType:    servicedef.ValueTypeAny,
						DefaultValue: ldvalue.String("default"),
					})
					m.In(t).Assert(response.Value, m.Equal(ldvalue.String("fallthrough")))

					close(closeWhenReady)

					pollUntilFlagValueUpdated(t, client, "flag-key", context,
						ldvalue.String("fallthrough"), updatedValue, ldvalue.String("default"))

					require.NoError(t, s.persistentStore.Reset())
					h.RequireNever(t,
						checkForUpdatedValue(t, client, "flag-key", context,
							updatedValue, ldvalue.String("default"), ldvalue.String("default")),
						time.Millisecond*250, time.Millisecond*20, "flag was lost on db reset")
				})
			}
		})
	}

	t.Run("read-write", func(t *ldtest.T) {
		persistence := NewPersistence()
		persistence.SetStoreMode(servicedef.DataStoreModeReadWrite)
		persistence.SetStore(servicedef.SDKConfigPersistentStore{
			Type: s.persistentStore.Type(),
			DSN:  s.persistentStore.DSN(),
		})
		persistence.SetCache(servicedef.SDKConfigPersistentCache{
			Mode: servicedef.CacheModeOff,
		})

		s.runWithEmptyStore(t, "initializes store when data received", func(t *ldtest.T) {
			sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
			_, configurers := s.setupDataSystems(t, sdkData)
			configurers = append(configurers, persistence)

			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

			value, _ := s.persistentStore.Get(s.defaultPrefix, persistenceInitedKey)
			require.False(t, value.IsDefined()) // should not exist

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

			// Overrides the existing flag definitions
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key": basicFlagValidationMatcher("flag-key", 1, "value"),
			})
		})

		s.runWithEmptyStore(t, "applies updates to store", func(t *ldtest.T) {
			sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
			dataSystem, configurers := s.setupDataSystems(t, sdkData)
			configurers = append(configurers, persistence)

			value, _ := s.persistentStore.Get(s.defaultPrefix, persistenceInitedKey)
			require.False(t, value.IsDefined()) // should not exist

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key": basicFlagValidationMatcher("flag-key", 1, "value"),
			})

			updateData := s.makeFlagData("flag-key", 2, ldvalue.String("new-value"))
			dataSystem.Synchronizers[0].streaming.PushUpdate("flag", "flag-key", 2, updateData)
			dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key": basicFlagValidationMatcher("flag-key", 2, "new-value"),
			})
		})

		// Companion to "applies updates to store": that test verifies the SDK writes
		// updates through to the persistent store; this one verifies the SDK serves
		// the new value from its in-memory state on subsequent evaluations.
		s.runWithEmptyStore(t, "evaluation reflects streaming updates", func(t *ldtest.T) {
			sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
			dataSystem, configurers := s.setupDataSystems(t, sdkData)
			configurers = append(configurers, persistence)

			client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			context := ldcontext.New("user-key")
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

			pollUntilFlagValueUpdated(t, client, "flag-key", context,
				ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

			updateData := s.makeFlagData("flag-key", 2, ldvalue.String("new-value"))
			dataSystem.Synchronizers[0].streaming.PushUpdate("flag", "flag-key", 2, updateData)
			dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)

			pollUntilFlagValueUpdated(t, client, "flag-key", context,
				ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default"))
		})

		s.runWithEmptyStore(t, "data source updates respect versioning", func(t *ldtest.T) {
			sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
			dataSystem, configurers := s.setupDataSystems(t, sdkData)
			configurers = append(configurers, persistence)

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

			// Lower versioned updates are ignored
			updateData := s.makeFlagData("flag-key", 1, ldvalue.String("new-value"))
			dataSystem.Synchronizers[0].streaming.PushUpdate("flag", "flag-key", 1, updateData)
			dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)
			s.neverValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicFlagValidationMatcher("flag-key", 1, "new-value"),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "uncached-fallthrough"),
			})

			// Same versioned updates are ignored
			updateData = s.makeFlagData("flag-key", 100, ldvalue.String("new-value"))
			dataSystem.Synchronizers[0].streaming.PushUpdate("flag", "flag-key", 100, updateData)
			dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 3)
			s.neverValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicFlagValidationMatcher("flag-key", 1, "new-value"),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "uncached-fallthrough"),
			})

			// Higher versioned updates are applied
			updateData = s.makeFlagData("flag-key", 200, ldvalue.String("new-value"))
			dataSystem.Synchronizers[0].streaming.PushUpdate("flag", "flag-key", 200, updateData)
			dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 4)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicFlagValidationMatcher("flag-key", 200, "new-value"),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "uncached-fallthrough"),
			})
		})

		s.runWithEmptyStore(t, "data source deletions respect versioning", func(t *ldtest.T) {
			sdkData := s.makeSDKDataWithFlag(100, ldvalue.String("value"))
			dataSystem, configurers := s.setupDataSystems(t, sdkData)
			configurers = append(configurers, persistence)

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

			// Lower versioned deletes are ignored
			dataSystem.Synchronizers[0].streaming.PushDelete("flag", "flag-key", 1)
			dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 2)
			s.neverValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicDeletedFlagValidationMatcher("flag-key", 1),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "uncached-fallthrough"),
			})

			// Higher versioned deletes are applied
			dataSystem.Synchronizers[0].streaming.PushDelete("flag", "flag-key", 200)
			dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 3)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicDeletedFlagValidationMatcher("flag-key", 200),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "uncached-fallthrough"),
			})
		})

		// Once a synchronizer has delivered a basis, the in-memory store is the
		// authoritative source for read-write FDv2 evaluations; the persistent store
		// is write-through only. These tests verify that direct activity against the
		// persistent store has no effect on what evaluations return, regardless of
		// whether a key was previously evaluated.
		s.runWithEmptyStore(t, "ignores direct database modifications", func(t *ldtest.T) {
			sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
			_, configurers := s.setupDataSystems(t, sdkData)
			configurers = append(configurers, persistence)

			client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			context := ldcontext.New("user-key")
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

			pollUntilFlagValueUpdated(t, client, "flag-key", context,
				ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

			// initialFlags overwrites flag-key (already evaluated) and adds
			// uncached-flag-key (never evaluated). Both should be invisible to reads.
			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

			h.RequireNever(t,
				checkForUpdatedValue(t, client, "flag-key", context,
					ldvalue.String("value"), ldvalue.String("fallthrough"), ldvalue.String("default")),
				time.Millisecond*500, time.Millisecond*20,
				"flag-key reflected a direct database modification")

			h.RequireNever(t,
				checkForUpdatedValue(t, client, "uncached-flag-key", context,
					ldvalue.String("default"), ldvalue.String("uncached-fallthrough"), ldvalue.String("default")),
				time.Millisecond*500, time.Millisecond*20,
				"uncached-flag-key surfaced from a direct database write")
		})

		s.runWithEmptyStore(t, "ignores dropped flags", func(t *ldtest.T) {
			sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
			_, configurers := s.setupDataSystems(t, sdkData)
			configurers = append(configurers, persistence)

			client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			context := ldcontext.New("user-key")
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

			pollUntilFlagValueUpdated(t, client, "flag-key", context,
				ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

			require.NoError(t, s.persistentStore.Reset())

			h.RequireNever(t,
				checkForUpdatedValue(t, client, "flag-key", context,
					ldvalue.String("value"), ldvalue.String("default"), ldvalue.String("default")),
				time.Millisecond*500, time.Millisecond*20,
				"flag-key was dropped from the in-memory store after a database reset")
		})
	})
}

func (s *ServerSidePersistentTests) runWithEmptyStore(t *ldtest.T, testName string, action func(*ldtest.T)) {
	t.Run(testName, func(t *ldtest.T) {
		require.NoError(t, s.persistentStore.Reset())
		action(t)
	})
}

// PS 1.1.7: validates the $inited key is set in the persistent store
func (s *ServerSidePersistentTests) eventuallyRequireDataStoreInit(t *ldtest.T, prefix string) {
	h.RequireEventually(t, func() bool {
		value, _ := s.persistentStore.Get(prefix, persistenceInitedKey)
		return value.IsDefined()
	}, time.Second, time.Millisecond*20, persistenceInitedKey+" key was not set")
}

// PS 1.1.3, 1.1.6: validates flag data written to persistent store matches expectations
func (s *ServerSidePersistentTests) eventuallyValidateFlagData(
	t *ldtest.T, prefix string, matchers map[string]m.Matcher) {
	h.RequireEventually(t, func() bool {
		data, err := s.persistentStore.GetMap(prefix, "features")
		if err != nil {
			return false
		}

		return validateFlagData(data, matchers)
	}, time.Second, time.Millisecond*20, "flag data did not match")
}

// PS 1.1.6: validates flag data is NOT written when version check rejects update
func (s *ServerSidePersistentTests) neverValidateFlagData(t *ldtest.T, prefix string, matchers map[string]m.Matcher) {
	h.RequireNever(t, func() bool {
		data, err := s.persistentStore.GetMap(prefix, "features")
		if err != nil {
			return false
		}

		return validateFlagData(data, matchers)
	}, time.Second, time.Millisecond*20, "flag data did not match")
}

// PS 1.1.3: builds matcher for flag key, version, and variations in store
func basicFlagValidationMatcher(key string, version int, value string) m.Matcher {
	return m.AllOf(
		m.JSONProperty("key").Should(m.Equal(key)),
		m.JSONProperty("version").Should(m.Equal(version)),
		m.JSONProperty("variations").Should(m.Equal([]interface{}{value, "other"})),
	)
}

// PS 1.1.6: builds matcher for deleted flag tombstone in store
func basicDeletedFlagValidationMatcher(key string, version int) m.Matcher {
	return m.AllOf(
		m.AnyOf(
			m.JSONProperty("key").Should(m.Equal(key)),
			m.JSONProperty("key").Should(m.Equal("$deleted")),
		),
		m.JSONProperty("version").Should(m.Equal(version)),
		m.JSONProperty("deleted").Should(m.Equal(true)),
	)
}

func validateFlagData(data map[string]string, matchers map[string]m.Matcher) bool {
	if len(data) != len(matchers) {
		return false
	}

	for key, matcher := range matchers {
		flag, ok := data[key]
		if !ok {
			return false
		}

		result, _ := matcher.Test(flag)
		if !result {
			return false
		}
	}

	return true
}
