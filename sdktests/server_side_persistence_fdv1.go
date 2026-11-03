package sdktests

import (
	"fmt"
	"time"

	"github.com/stretchr/testify/require"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

// runFDv1 runs the persistent store tests for an FDv1 SDK. The SDK configuration has the top-level
// persistentDataStore property, and the SDK reads the store directly when it has no data source. With
// a cache, it reads the store again after the cache expires.
func (s *ServerSidePersistentTests) runFDv1(t *ldtest.T) {
	s.runWithEmptyStore(t, "uses default prefix", func(t *ldtest.T) {
		require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

		persistence := s.newPersistence()
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

		persistence := s.newPersistence()
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

	t.Run("daemon mode", func(t *ldtest.T) {
		persistence := s.newPersistence()
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
				time.Second, time.Millisecond*20, "failed to serve defaults after flag deletion")
		})

		t.Run("tombstones", s.doTombstoneTests)

		t.Run("caches flag for duration", func(t *ldtest.T) {
			persistence := s.newPersistence()
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
			persistence := s.newPersistence()
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
					ldvalue.String("default"), ldvalue.String("uncached-fallthrough"), ldvalue.String("default")),
				time.Millisecond*500, time.Millisecond*20, "uncached-flag-key was not determined to be missing")

			// We are caching the old flag version forever, so this should also never revert to the default.
			h.RequireNever(t,
				checkForUpdatedValue(t, client, "flag-key", context,
					ldvalue.String("fallthrough"), ldvalue.String("default"), ldvalue.String("default")),
				time.Millisecond*500, time.Millisecond*20, "flag value was not changed")
		})

		t.Run("collection cardinality", s.doCollectionCardinalityTests)
	})

	t.Run("read-write", func(t *ldtest.T) {
		// No cache is enabled
		s.runWithEmptyStore(t, "initializes store when data received", func(t *ldtest.T) {
			persistence := s.newPersistence()
			persistence.SetStore(servicedef.SDKConfigPersistentStore{
				Type: s.persistentStore.Type(),
				DSN:  s.persistentStore.DSN(),
			})
			persistence.SetCache(servicedef.SDKConfigPersistentCache{
				Mode: servicedef.CacheModeOff,
			})

			sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
			_, configurers := s.setupDataSystems(t, sdkData)
			configurers = append(configurers, persistence)

			value, _ := s.persistentStore.Get(s.defaultPrefix, persistenceInitedKey)
			require.False(t, value.IsDefined()) // should not exist

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
		})

		s.runWithEmptyStore(t, "applies updates to store", func(t *ldtest.T) {
			persistence := s.newPersistence()
			persistence.SetStore(servicedef.SDKConfigPersistentStore{
				Type: s.persistentStore.Type(),
				DSN:  s.persistentStore.DSN(),
			})
			persistence.SetCache(servicedef.SDKConfigPersistentCache{
				Mode: servicedef.CacheModeOff,
			})

			sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
			stream, configurers := s.setupDataSystems(t, sdkData)
			configurers = append(configurers, persistence)

			value, _ := s.persistentStore.Get(s.defaultPrefix, persistenceInitedKey)
			require.False(t, value.IsDefined()) // should not exist

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key": basicFlagValidationMatcher("flag-key", 1, "value"),
			})

			updateData := s.makeFlagData("flag-key", 2, ldvalue.String("new-value"))
			stream.Synchronizers[0].streaming.PushUpdate("flag", "flag-key", 2, updateData)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key": basicFlagValidationMatcher("flag-key", 2, "new-value"),
			})
		})

		s.runWithEmptyStore(t, "data source updates respect versioning", func(t *ldtest.T) {
			persistence := s.newPersistence()
			persistence.SetStore(servicedef.SDKConfigPersistentStore{
				Type: s.persistentStore.Type(),
				DSN:  s.persistentStore.DSN(),
			})
			persistence.SetCache(servicedef.SDKConfigPersistentCache{
				Mode: servicedef.CacheModeOff,
			})

			sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
			stream, configurers := s.setupDataSystems(t, sdkData)
			configurers = append(configurers, persistence)

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

			// Lower versioned updates are ignored
			updateData := s.makeFlagData("flag-key", 1, ldvalue.String("new-value"))
			stream.Synchronizers[0].streaming.PushUpdate("flag", "flag-key", 1, updateData)
			s.neverValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicFlagValidationMatcher("flag-key", 1, "new-value"),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "uncached-fallthrough"),
			})

			// Same versioned updates are ignored
			updateData = s.makeFlagData("flag-key", 100, ldvalue.String("new-value"))
			stream.Synchronizers[0].streaming.PushUpdate("flag", "flag-key", 100, updateData)
			s.neverValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicFlagValidationMatcher("flag-key", 1, "new-value"),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "uncached-fallthrough"),
			})

			// Higher versioned updates are applied
			updateData = s.makeFlagData("flag-key", 200, ldvalue.String("new-value"))
			stream.Synchronizers[0].streaming.PushUpdate("flag", "flag-key", 200, updateData)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicFlagValidationMatcher("flag-key", 200, "new-value"),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "uncached-fallthrough"),
			})
		})

		s.runWithEmptyStore(t, "data source deletions respect versioning", func(t *ldtest.T) {
			persistence := s.newPersistence()
			persistence.SetStore(servicedef.SDKConfigPersistentStore{
				Type: s.persistentStore.Type(),
				DSN:  s.persistentStore.DSN(),
			})
			persistence.SetCache(servicedef.SDKConfigPersistentCache{
				Mode: servicedef.CacheModeOff,
			})

			sdkData := s.makeSDKDataWithFlag(100, ldvalue.String("value"))
			stream, configurers := s.setupDataSystems(t, sdkData)
			configurers = append(configurers, persistence)

			_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

			// Lower versioned deletes are ignored
			stream.Synchronizers[0].streaming.PushDelete("flag", "flag-key", 1)
			s.neverValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicDeletedFlagValidationMatcher("flag-key", 1),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "uncached-fallthrough"),
			})

			// Higher versioned deletes are applied
			stream.Synchronizers[0].streaming.PushDelete("flag", "flag-key", 200)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key":          basicDeletedFlagValidationMatcher("flag-key", 200),
				"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "uncached-fallthrough"),
			})
		})

		cacheConfigs := []servicedef.SDKConfigPersistentCache{
			{Mode: servicedef.CacheModeInfinite},
			{Mode: servicedef.CacheModeTTL, TTL: o.Some(5)},
		}

		for _, cacheConfig := range cacheConfigs {
			t.Run(fmt.Sprintf("cache mode %s", cacheConfig.Mode), func(t *ldtest.T) {
				s.runWithEmptyStore(t, "does not cache flag miss", func(t *ldtest.T) {
					persistence := s.newPersistence()
					persistence.SetStore(servicedef.SDKConfigPersistentStore{
						Type: s.persistentStore.Type(),
						DSN:  s.persistentStore.DSN(),
					})
					persistence.SetCache(cacheConfig)

					stream, configurers := s.setupDataSystems(t, mockld.NewServerSDKDataBuilder().Build())
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
					stream.Synchronizers[0].streaming.PushUpdate("flag", "flag-key", 2, updateData)

					h.RequireEventually(t,
						checkForUpdatedValue(t, client, "flag-key", context,
							ldvalue.String("default"), ldvalue.String("new-value"), ldvalue.String("default")),
						time.Millisecond*500, time.Millisecond*20, "flag was never updated")
				})
				s.runWithEmptyStore(t, "sdk reflects data source updates even with cache", func(t *ldtest.T) {
					persistence := s.newPersistence()
					persistence.SetStore(servicedef.SDKConfigPersistentStore{
						Type: s.persistentStore.Type(),
						DSN:  s.persistentStore.DSN(),
					})
					persistence.SetCache(cacheConfig)

					sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
					stream, configurers := s.setupDataSystems(t, sdkData)
					configurers = append(configurers, persistence)

					client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
					context := ldcontext.New("user-key")
					s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

					pollUntilFlagValueUpdated(t, client, "flag-key", context,
						ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

					updateData := s.makeFlagData("flag-key", 2, ldvalue.String("new-value"))
					stream.Synchronizers[0].streaming.PushUpdate("flag", "flag-key", 2, updateData)

					// This change is reflected in less time than the cache TTL. This should
					// prove it isn't caching that value.
					h.RequireEventually(t,
						checkForUpdatedValue(t, client, "flag-key", context,
							ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default")),
						time.Millisecond*500, time.Millisecond*20, "flag was updated")
				})
				s.runWithEmptyStore(t, "ignores direct database modifications", func(t *ldtest.T) {
					persistence := s.newPersistence()
					persistence.SetStore(servicedef.SDKConfigPersistentStore{
						Type: s.persistentStore.Type(),
						DSN:  s.persistentStore.DSN(),
					})
					persistence.SetCache(cacheConfig)

					sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
					_, configurers := s.setupDataSystems(t, sdkData)
					configurers = append(configurers, persistence)

					client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
					context := ldcontext.New("user-key")
					s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)

					pollUntilFlagValueUpdated(t, client, "flag-key", context,
						ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

					require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

					switch cacheConfig.Mode {
					case servicedef.CacheModeInfinite:
						// This key was already cached, so it shouldn't see the change above.
						h.RequireNever(t,
							checkForUpdatedValue(t, client, "flag-key", context,
								ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default")),
							time.Millisecond*500, time.Millisecond*20, "flag-key was incorrectly updated")

						// But since we didn't evaluate this flag, this should actually be
						// reflected by directly changing the database.
						h.RequireEventually(t,
							checkForUpdatedValue(t, client, "uncached-flag-key", context,
								ldvalue.String("default"), ldvalue.String("uncached-fallthrough"), ldvalue.String("default")),
							time.Millisecond*500, time.Millisecond*20, "uncached-flag-key was incorrectly cached")
					case servicedef.CacheModeTTL:
						// This key was already cached, so it shouldn't see the change above. The check
						// window plus all setup steps must fit inside the cache TTL; keep it short so
						// a slow test runner cannot push the check past the cache expiry.
						h.RequireNever(t,
							checkForUpdatedValue(t, client, "flag-key", context,
								ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default")),
							time.Millisecond*500,
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
					persistence := s.newPersistence()
					persistence.SetStore(servicedef.SDKConfigPersistentStore{
						Type: s.persistentStore.Type(),
						DSN:  s.persistentStore.DSN(),
					})
					persistence.SetCache(cacheConfig)

					sdkData := s.makeSDKDataWithFlag(1, ldvalue.String("value"))
					_, configurers := s.setupDataSystems(t, sdkData)
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
							time.Duration(int(time.Second)*cacheConfig.TTL.Value()),
							time.Millisecond*20, "flag-key was incorrectly cached")
					}
				})
			})
		}
	})
}
