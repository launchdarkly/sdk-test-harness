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
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	consul "github.com/hashicorp/consul/api"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

const (
	persistenceInitedKey = "$inited"
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
		err := store.Reset()
		require.NoError(t, err)

		t.Run("dynamodb", newServerSidePersistentTests(t, &store, "").Run)
	}
}

type PersistentStore interface {
	DSN() string

	Get(prefix, key string) (o.Maybe[string], error)
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
	t.Run("uses default prefix", func(t *ldtest.T) {
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

	t.Run("uses custom prefix", func(t *ldtest.T) {
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
	})

	t.Run("daemon mode", func(t *ldtest.T) {
		persistence := NewPersistence()
		persistence.SetStore(servicedef.SDKConfigPersistentStore{
			Type: s.persistentStore.Type(),
			DSN:  s.persistentStore.DSN(),
		})
		persistence.SetCache(servicedef.SDKConfigPersistentCache{
			Mode: servicedef.CacheModeOff,
		})
		context := ldcontext.New("user-key")

		s.runWithEmptyStore(t, "ignores database initialization flag", func(t *ldtest.T) {
			client := NewSDKClient(t, persistence)

			h.RequireEventually(t, func() bool {
				result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      "flag-key",
					Context:      o.Some(context),
					ValueType:    servicedef.ValueTypeAny,
					DefaultValue: ldvalue.String("default"),
					Detail:       true,
				})

				return result.Reason.IsDefined() &&
					result.Reason.Value().GetErrorKind() == ldreason.EvalErrorFlagNotFound
			}, time.Second, time.Millisecond*20, "flag was found before it should have been")

			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))
			pollUntilFlagValueUpdated(t, client, "flag-key", context,
				ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))
		})

		s.runWithEmptyStore(t, "can disable cache", func(t *ldtest.T) {
			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

			client := NewSDKClient(t, persistence)
			pollUntilFlagValueUpdated(t, client, "flag-key", context,
				ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))

			// Completely reset the database so there are no valid flag definitions
			require.NoError(t, s.persistentStore.Reset())

			h.RequireEventually(t,
				checkForUpdatedValue(t, client, "flag-key", context,
					ldvalue.String("fallthrough"), ldvalue.String("default"), ldvalue.String("default")),
				time.Second, time.Millisecond*20, "flag value was NOT updated after cache TTL")
		})

		t.Run("caches flag for duration", func(t *ldtest.T) {
			persistence := NewPersistence()
			persistence.SetStore(servicedef.SDKConfigPersistentStore{
				Type: s.persistentStore.Type(),
				DSN:  s.persistentStore.DSN(),
			})
			persistence.SetCache(servicedef.SDKConfigPersistentCache{
				Mode: servicedef.CacheModeTTL,
				TTL:  o.Some(1),
			})
			context := ldcontext.New("user-key")

			s.runWithEmptyStore(t, "cache hit persists for TTL", func(t *ldtest.T) {
				client := NewSDKClient(t, persistence)

				require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

				pollUntilFlagValueUpdated(t, client, "flag-key", context,
					ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))

				// Completely reset the database so there are no valid flag definitions
				require.NoError(t, s.persistentStore.Reset())

				h.RequireNever(t,
					checkForUpdatedValue(t, client, "flag-key", context,
						ldvalue.String("fallthrough"), ldvalue.String("default"), ldvalue.String("default")),
					time.Millisecond*500, time.Millisecond*20, "flag value was updated before cache TTL")

				h.RequireEventually(t,
					checkForUpdatedValue(t, client, "flag-key", context,
						ldvalue.String("fallthrough"), ldvalue.String("default"), ldvalue.String("default")),
					time.Second, time.Millisecond*20, "flag value was NOT updated after cache TTL")
			})

			s.runWithEmptyStore(t, "cache miss persists for TTL", func(t *ldtest.T) {
				client := NewSDKClient(t, persistence)

				result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      "flag-key",
					Context:      o.Some(context),
					ValueType:    servicedef.ValueTypeAny,
					DefaultValue: ldvalue.String("default"),
					Detail:       true,
				})

				m.In(t).Assert(result.Value, m.Equal(ldvalue.String("default")))
				m.In(t).Assert(result.Reason.Value().GetErrorKind(), m.Equal(ldreason.EvalErrorFlagNotFound))

				require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

				h.RequireNever(t,
					checkForUpdatedValue(t, client, "flag-key", context,
						ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
					time.Microsecond*500, time.Millisecond*20, "flag value was updated before cache TTL")

				h.RequireEventually(t,
					checkForUpdatedValue(t, client, "flag-key", context,
						ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
					time.Second, time.Millisecond*20, "flag value was NOT updated after cache TTL")
			})
		})

		s.runWithEmptyStore(t, "caches flag forever", func(t *ldtest.T) {
			persistence := NewPersistence()
			persistence.SetStore(servicedef.SDKConfigPersistentStore{
				Type: s.persistentStore.Type(),
				DSN:  s.persistentStore.DSN(),
			})
			persistence.SetCache(servicedef.SDKConfigPersistentCache{
				Mode: servicedef.CacheModeInfinite,
			})
			context := ldcontext.New("user-key")

			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

			client := NewSDKClient(t, persistence)

			h.RequireEventually(t,
				checkForUpdatedValue(t, client, "flag-key", context,
					ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
				time.Millisecond*500, time.Millisecond*20, "flag value was not changed")

			// Reset the store and verify that the flag value is still cached
			require.NoError(t, s.persistentStore.Reset())

			// Uncached key is gone, so we should NEVER see it evaluate as expected.
			h.RequireNever(t,
				checkForUpdatedValue(t, client, "uncached-flag-key", context,
					ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
				time.Millisecond*500, time.Millisecond*20, "uncached-flag-key was not determined to be missing")

			// We are caching the old flag version forever, so this should also never revert to the default.
			h.RequireNever(t,
				checkForUpdatedValue(t, client, "flag-key", context,
					ldvalue.String("fallthrough"), ldvalue.String("default"), ldvalue.String("default")),
				time.Millisecond*500, time.Millisecond*20, "flag value was not changed")
		})
	})

	t.Run("read-write", func(t *ldtest.T) {
		// No cache is enabled
		s.runWithEmptyStore(t, "initializes store when data received", func(t *ldtest.T) {
			persistence := NewPersistence()
			persistence.SetStore(servicedef.SDKConfigPersistentStore{
				Type: s.persistentStore.Type(),
				DSN:  s.persistentStore.DSN(),
			})
			persistence.SetCache(servicedef.SDKConfigPersistentCache{
				Mode: servicedef.CacheModeOff,
			})

			sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
			_, configurers := s.setupDataSources(t, sdkData)
			configurers = append(configurers, persistence)

			value, _ := s.persistentStore.Get(s.defaultPrefix, persistenceInitedKey)
			require.False(t, value.IsDefined()) // should not exist

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
		})

		s.runWithEmptyStore(t, "applies updates to store", func(t *ldtest.T) {
			persistence := NewPersistence()
			persistence.SetStore(servicedef.SDKConfigPersistentStore{
				Type: s.persistentStore.Type(),
				DSN:  s.persistentStore.DSN(),
			})
			persistence.SetCache(servicedef.SDKConfigPersistentCache{
				Mode: servicedef.CacheModeOff,
			})

			sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
			stream, configurers := s.setupDataSources(t, sdkData)
			configurers = append(configurers, persistence)

			value, _ := s.persistentStore.Get(s.defaultPrefix, persistenceInitedKey)
			require.False(t, value.IsDefined()) // should not exist

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key": basicFlagValidationMatcher("flag-key", 1, "value"),
			})

			updateData := s.makeFlagData("flag-key", 2, ldvalue.String("new-value"))
			stream.StreamingService().PushUpdate("flags", "flag-key", updateData)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key": basicFlagValidationMatcher("flag-key", 2, "new-value"),
			})
		})

		s.runWithEmptyStore(t, "data source updates respect versioning", func(t *ldtest.T) {
			persistence := NewPersistence()
			persistence.SetStore(servicedef.SDKConfigPersistentStore{
				Type: s.persistentStore.Type(),
				DSN:  s.persistentStore.DSN(),
			})
			persistence.SetCache(servicedef.SDKConfigPersistentCache{
				Mode: servicedef.CacheModeOff,
			})

			sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
			stream, configurers := s.setupDataSources(t, sdkData)
			configurers = append(configurers, persistence)

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

			// Lower versioned updates are ignored
			updateData := s.makeFlagData("flag-key", 1, ldvalue.String("new-value"))
			stream.StreamingService().PushUpdate("flags", "flag-key", updateData)
			s.neverValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicFlagValidationMatcher("flag-key", 1, "new-value"),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "value"),
			})

			// Same versioned updates are ignored
			updateData = s.makeFlagData("flag-key", 100, ldvalue.String("new-value"))
			stream.StreamingService().PushUpdate("flags", "flag-key", updateData)
			s.neverValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicFlagValidationMatcher("flag-key", 1, "new-value"),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "value"),
			})

			// Higher versioned updates are applied
			updateData = s.makeFlagData("flag-key", 200, ldvalue.String("new-value"))
			stream.StreamingService().PushUpdate("flags", "flag-key", updateData)
			s.neverValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicFlagValidationMatcher("flag-key", 200, "new-value"),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "value"),
			})
		})

		s.runWithEmptyStore(t, "data source deletions respect versioning", func(t *ldtest.T) {
			persistence := NewPersistence()
			persistence.SetStore(servicedef.SDKConfigPersistentStore{
				Type: s.persistentStore.Type(),
				DSN:  s.persistentStore.DSN(),
			})
			persistence.SetCache(servicedef.SDKConfigPersistentCache{
				Mode: servicedef.CacheModeOff,
			})

			sdkData := s.makeSDKDataWithFlag("flag-key", 100, ldvalue.String("value"))
			stream, configurers := s.setupDataSources(t, sdkData)
			configurers = append(configurers, persistence)

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

			// Lower versioned deletes are ignored
			stream.StreamingService().PushDelete("flags", "flag-key", 1)
			s.neverValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicDeletedFlagValidationMatcher(1),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "fallthrough"),
			})

			// Higher versioned deletes are applied
			stream.StreamingService().PushDelete("flags", "flag-key", 200)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicDeletedFlagValidationMatcher(200),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "fallthrough"),
			})
		})

		cacheConfigs := []servicedef.SDKConfigPersistentCache{
			{Mode: servicedef.CacheModeInfinite},
			{Mode: servicedef.CacheModeTTL, TTL: o.Some(1)},
		}

		for _, cacheConfig := range cacheConfigs {
			t.Run(fmt.Sprintf("cache mode %s", cacheConfig.Mode), func(t *ldtest.T) {
				s.runWithEmptyStore(t, "does not cache flag miss", func(t *ldtest.T) {
					persistence := NewPersistence()
					persistence.SetStore(servicedef.SDKConfigPersistentStore{
						Type: s.persistentStore.Type(),
						DSN:  s.persistentStore.DSN(),
					})
					persistence.SetCache(cacheConfig)

					stream, configurers := s.setupDataSources(t, mockld.NewServerSDKDataBuilder().Build())
					configurers = append(configurers, persistence)

					client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
					context := ldcontext.New("user-key")
					s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

					response := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
						FlagKey:      "flag-key",
						Context:      o.Some(context),
						ValueType:    servicedef.ValueTypeAny,
						DefaultValue: ldvalue.String("default"),
					})

					m.In(t).Assert(response.Value, m.Equal(ldvalue.String("default")))

					updateData := s.makeFlagData("flag-key", 2, ldvalue.String("new-value"))
					stream.StreamingService().PushUpdate("flags", "flag-key", updateData)

					h.RequireEventually(t,
						checkForUpdatedValue(t, client, "flag-key", context,
							ldvalue.String("default"), ldvalue.String("new-value"), ldvalue.String("default")),
						time.Millisecond*500, time.Millisecond*20, "flag was never updated")
				})
				s.runWithEmptyStore(t, "sdk reflects data source updates even with cache", func(t *ldtest.T) {
					persistence := NewPersistence()
					persistence.SetStore(servicedef.SDKConfigPersistentStore{
						Type: s.persistentStore.Type(),
						DSN:  s.persistentStore.DSN(),
					})
					persistence.SetCache(cacheConfig)

					sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
					stream, configurers := s.setupDataSources(t, sdkData)
					configurers = append(configurers, persistence)

					client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
					context := ldcontext.New("user-key")
					s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

					pollUntilFlagValueUpdated(t, client, "flag-key", context,
						ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

					updateData := s.makeFlagData("flag-key", 2, ldvalue.String("new-value"))
					stream.StreamingService().PushUpdate("flags", "flag-key", updateData)

					// This change is reflected in less time than the cache TTL. This should
					// prove it isn't caching that value.
					h.RequireEventually(t,
						checkForUpdatedValue(t, client, "flag-key", context,
							ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default")),
						time.Millisecond*500, time.Millisecond*20, "flag was updated")
				})
				s.runWithEmptyStore(t, "ignores direct database modifications", func(t *ldtest.T) {
					persistence := NewPersistence()
					persistence.SetStore(servicedef.SDKConfigPersistentStore{
						Type: s.persistentStore.Type(),
						DSN:  s.persistentStore.DSN(),
					})
					persistence.SetCache(cacheConfig)

					sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
					_, configurers := s.setupDataSources(t, sdkData)
					configurers = append(configurers, persistence)

					client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
					context := ldcontext.New("user-key")
					s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

					pollUntilFlagValueUpdated(t, client, "flag-key", context,
						ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

					require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

					if cacheConfig.Mode == servicedef.CacheModeInfinite {
						// This key was already cached, so it shouldn't see the change above.
						h.RequireNever(t,
							checkForUpdatedValue(t, client, "flag-key", context,
								ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default")),
							time.Millisecond*500, time.Millisecond*20, "flag-key was incorrectly updated")

						// But since we didn't evaluate this flag, this should actually be
						// reflected by directly changing the database.
						h.RequireEventually(t,
							checkForUpdatedValue(t, client, "uncached-flag-key", context,
								ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
							time.Millisecond*500, time.Millisecond*20, "uncached-flag-key was incorrectly cached")
					} else if cacheConfig.Mode == servicedef.CacheModeTTL {
						// This key was already cached, so it shouldn't see the change above.
						h.RequireNever(t,
							checkForUpdatedValue(t, client, "flag-key", context,
								ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default")),
							time.Duration(
								int(time.Second)*cacheConfig.TTL.Value()/2),
							time.Millisecond*20,
							"flag-key was incorrectly updated")

						// But eventually, it will expire and then we will fetch it from the database.
						h.RequireEventually(t,
							checkForUpdatedValue(t, client, "flag-key", context,
								ldvalue.String("value"), ldvalue.String("fallthrough"), ldvalue.String("default")),
							time.Duration(int(time.Second)*cacheConfig.TTL.Value()), time.Millisecond*20, "flag-key was incorrectly cached")
					}
				})

				s.runWithEmptyStore(t, "ignores dropped flags", func(t *ldtest.T) {
					persistence := NewPersistence()
					persistence.SetStore(servicedef.SDKConfigPersistentStore{
						Type: s.persistentStore.Type(),
						DSN:  s.persistentStore.DSN(),
					})
					persistence.SetCache(cacheConfig)

					sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
					_, configurers := s.setupDataSources(t, sdkData)
					configurers = append(configurers, persistence)

					client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
					context := ldcontext.New("user-key")
					s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

					pollUntilFlagValueUpdated(t, client, "flag-key", context,
						ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

					require.NoError(t, s.persistentStore.Reset())

					// This key was already cached, so it shouldn't see the change above.
					h.RequireNever(t,
						checkForUpdatedValue(t, client, "flag-key", context,
							ldvalue.String("value"), ldvalue.String("default"), ldvalue.String("default")),
						time.Millisecond*500, time.Millisecond*20, "flag was never updated")

					if cacheConfig.Mode == servicedef.CacheModeTTL {
						// But eventually, it will expire and then we will fetch it from the database.
						h.RequireEventually(t,
							checkForUpdatedValue(t, client, "flag-key", context,
								ldvalue.String("value"), ldvalue.String("default"), ldvalue.String("default")),
							time.Second, time.Millisecond*20, "flag-key was incorrectly cached")
					}
				})
			})
		}
	})
}

func (s *ServerSidePersistentTests) runWithEmptyStore(t *ldtest.T, testName string, action func(*ldtest.T)) {
	t.Run(testName, func(t *ldtest.T) {
		require.NoError(t, s.persistentStore.Reset())
		action(t)
	})
}

func (s *ServerSidePersistentTests) eventuallyRequireDataStoreInit(t *ldtest.T, prefix string) {
	h.RequireEventually(t, func() bool {
		value, _ := s.persistentStore.Get(prefix, persistenceInitedKey)
		return value.IsDefined()
	}, time.Second, time.Millisecond*20, persistenceInitedKey+" key was not set")
}

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

func (s *ServerSidePersistentTests) neverValidateFlagData(t *ldtest.T, prefix string, matchers map[string]m.Matcher) {
	h.RequireNever(t, func() bool {
		data, err := s.persistentStore.GetMap(prefix, "features")
		if err != nil {
			return false
		}

		return validateFlagData(data, matchers)
	}, time.Second, time.Millisecond*20, "flag data did not match")
}

func basicFlagValidationMatcher(key string, version int, value string) m.Matcher {
	return m.AllOf(
		m.JSONProperty("key").Should(m.Equal(key)),
		m.JSONProperty("version").Should(m.Equal(version)),
		m.JSONProperty("variations").Should(m.Equal([]interface{}{value, "other"})),
	)
}

func basicDeletedFlagValidationMatcher(version int) m.Matcher {
	return m.AllOf(
		m.JSONProperty("key").Should(m.Equal("$deleted")),
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
