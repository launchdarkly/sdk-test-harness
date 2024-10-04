package sdktests

import (
	"fmt"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
	"github.com/stretchr/testify/require"
)

func (s *ServerSidePersistentTests) doReadWriteTests(t *ldtest.T) {
	// No cache is enabled
	t.Run("initializes store when data received", s.initializesStoreWhenDataReceived)
	t.Run("applies updates to store", s.appliesUpdatesToStore)

	t.Run("data source updates respect versioning", s.dataSourceUpdatesRespectVersioning)
	t.Run("data source deletions respect versioning", s.dataSourceDeletesRespectVersioning)

	cacheConfigs := []servicedef.SDKConfigPersistentCache{
		{Mode: servicedef.Infinite},
		{Mode: servicedef.TTL, TTL: o.Some(1)},
	}

	for _, cacheConfig := range cacheConfigs {
		t.Run(fmt.Sprintf("cache mode %s", cacheConfig.Mode), func(t *ldtest.T) {
			t.Run("does not cache flag miss", func(t *ldtest.T) {
				s.doesNotCacheFlagMiss(t, cacheConfig)
			})
			t.Run("sdk reflects data source updates even with cache", func(t *ldtest.T) {
				s.sdkReflectsDataSourceUpdatesEvenWithCache(t, cacheConfig)
			})
			t.Run("ignores dropped flags", func(t *ldtest.T) {
				s.ignoresDroppedFlagsWithForeverCache(t)
			})
		})
	}

	t.Run("infinite cache", func(t *ldtest.T) {
		t.Run("ignores dropped flags", s.ignoresDroppedFlagsWithForeverCache)
	})

	t.Run("ttl cache", func(t *ldtest.T) {
		t.Run("ignores dropped flags", s.ignoresDroppedFlagsWithTTLCache)
	})
}

func (s *ServerSidePersistentTests) initializesStoreWhenDataReceived(t *ldtest.T) {
	require.NoError(t, s.persistentStore.Reset())

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  s.persistentStore.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.Off,
	})

	sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
	_, configurers := s.setupDataSources(t, sdkData)
	configurers = append(configurers, persistence)

	_, err := s.persistentStore.ReadField("launchdarkly:$inited")
	require.Error(t, err) // should not exist

	_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
	s.eventuallyRequireDataStoreInit(t, "launchdarkly")
}

func (s *ServerSidePersistentTests) appliesUpdatesToStore(t *ldtest.T) {
	require.NoError(t, s.persistentStore.Reset())

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  s.persistentStore.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.Off,
	})

	sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
	stream, configurers := s.setupDataSources(t, sdkData)
	configurers = append(configurers, persistence)

	_, err := s.persistentStore.ReadField("launchdarkly:$inited")
	require.Error(t, err) // should not exist

	_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
	s.eventuallyRequireDataStoreInit(t, "launchdarkly")
	s.eventuallyValidateFlagData(t, "launchdarkly", map[string]m.Matcher{
		"flag-key": basicFlagValidationMatcher("flag-key", 1, "value"),
	})

	updateData := s.makeFlagData("flag-key", 2, ldvalue.String("new-value"))
	stream.StreamingService().PushUpdate("flags", "flag-key", updateData)
	s.eventuallyValidateFlagData(t, "launchdarkly", map[string]m.Matcher{
		"flag-key": basicFlagValidationMatcher("flag-key", 2, "new-value"),
	})
}

func (s *ServerSidePersistentTests) dataSourceUpdatesRespectVersioning(t *ldtest.T) {
	require.NoError(t, s.persistentStore.Reset())

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  s.persistentStore.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.Off,
	})

	sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
	stream, configurers := s.setupDataSources(t, sdkData)
	configurers = append(configurers, persistence)

	_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
	s.eventuallyRequireDataStoreInit(t, "launchdarkly")

	require.NoError(t, s.persistentStore.WriteData("launchdarkly:features", s.initialFlags))

	// Lower versioned updates are ignored
	updateData := s.makeFlagData("flag-key", 1, ldvalue.String("new-value"))
	stream.StreamingService().PushUpdate("flags", "flag-key", updateData)
	s.neverValidateFlagData(t, "launchdarkly", map[string]m.Matcher{
		"flag-key":          basicFlagValidationMatcher("flag-key", 1, "new-value"),
		"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "value"),
	})

	// Higher versioned updates are applied
	updateData = s.makeFlagData("flag-key", 200, ldvalue.String("new-value"))
	stream.StreamingService().PushUpdate("flags", "flag-key", updateData)
	s.neverValidateFlagData(t, "launchdarkly", map[string]m.Matcher{
		"flag-key":          basicFlagValidationMatcher("flag-key", 200, "new-value"),
		"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "value"),
	})
}

func (s *ServerSidePersistentTests) dataSourceDeletesRespectVersioning(t *ldtest.T) {
	require.NoError(t, s.persistentStore.Reset())

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  s.persistentStore.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.Off,
	})

	sdkData := s.makeSDKDataWithFlag("flag-key", 100, ldvalue.String("value"))
	stream, configurers := s.setupDataSources(t, sdkData)
	configurers = append(configurers, persistence)

	_ = NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
	s.eventuallyRequireDataStoreInit(t, "launchdarkly")

	require.NoError(t, s.persistentStore.WriteData("launchdarkly:features", s.initialFlags))

	// Lower versioned deletes are ignored
	stream.StreamingService().PushDelete("flags", "flag-key", 1)
	s.neverValidateFlagData(t, "launchdarkly", map[string]m.Matcher{
		"flag-key":          basicDeletedFlagValidationMatcher(1),
		"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "fallthrough"),
	})

	// Higher versioned deletes are applied
	stream.StreamingService().PushDelete("flags", "flag-key", 200)
	s.eventuallyValidateFlagData(t, "launchdarkly", map[string]m.Matcher{
		"flag-key":          basicDeletedFlagValidationMatcher(200),
		"uncached-flag-key": basicFlagValidationMatcher("uncached-flag-key", 100, "fallthrough"),
	})
}

func (s *ServerSidePersistentTests) ignoresDirectDatabaseModifications(t *ldtest.T, cacheConfig servicedef.SDKConfigPersistentCache) {
	require.NoError(t, s.persistentStore.Reset())

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
	s.eventuallyRequireDataStoreInit(t, "launchdarkly")

	pollUntilFlagValueUpdated(t, client, "flag-key", context,
		ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

	require.NoError(t, s.persistentStore.WriteData("launchdarkly:features", s.initialFlags))

	// This key was already cached, so it shouldn't see the change above.
	h.RequireNever(t,
		checkForUpdatedValue(t, client, "flag-key", context,
			ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default")),
		time.Millisecond*500, time.Millisecond*20, "flag-key was incorrectly updated")

	if cacheConfig.Mode == servicedef.Infinite {
		// But since we didn't evaluate this flag, this should actually be
		// reflected by directly changing the database.
		h.RequireEventually(t,
			checkForUpdatedValue(t, client, "uncached-flag-key", context,
				ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
			time.Millisecond*500, time.Millisecond*20, "uncached-flag-key was incorrectly cached")
	} else if cacheConfig.Mode == servicedef.TTL {
		// But eventually, it will expire and then we will fetch it from the database.
		h.RequireEventually(t,
			checkForUpdatedValue(t, client, "flag-key", context,
				ldvalue.String("value"), ldvalue.String("fallthrough"), ldvalue.String("default")),
			time.Second, time.Millisecond*20, "flag-key was incorrectly cached")
	}
}

func (s *ServerSidePersistentTests) ignoresFlagsBeingDiscardedFromStore(t *ldtest.T, cacheConfig servicedef.SDKConfigPersistentCache) {
	require.NoError(t, s.persistentStore.Reset())

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  s.persistentStore.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.Infinite,
	})

	sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
	_, configurers := s.setupDataSources(t, sdkData)
	configurers = append(configurers, persistence)

	client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
	context := ldcontext.New("user-key")
	s.eventuallyRequireDataStoreInit(t, "launchdarkly")

	pollUntilFlagValueUpdated(t, client, "flag-key", context,
		ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

	require.NoError(t, s.persistentStore.Reset())

	// This key was already cached, so it shouldn't see the change above.
	h.RequireNever(t,
		checkForUpdatedValue(t, client, "flag-key", context,
			ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default")),
		time.Millisecond*500, time.Millisecond*20, "flag was never updated")

	if cacheConfig.Mode == servicedef.TTL {
		// But eventually, it will expire and then we will fetch it from the database.
		h.RequireEventually(t,
			checkForUpdatedValue(t, client, "flag-key", context,
				ldvalue.String("value"), ldvalue.String("default"), ldvalue.String("default")),
			time.Second, time.Millisecond*20, "flag-key was incorrectly cached")
	}
}

func (s *ServerSidePersistentTests) ignoresDroppedFlagsWithForeverCache(t *ldtest.T) {
	require.NoError(t, s.persistentStore.Reset())

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  s.persistentStore.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.Infinite,
	})

	sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
	_, configurers := s.setupDataSources(t, sdkData)
	configurers = append(configurers, persistence)

	client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
	context := ldcontext.New("user-key")
	s.eventuallyRequireDataStoreInit(t, "launchdarkly")

	pollUntilFlagValueUpdated(t, client, "flag-key", context,
		ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

	require.NoError(t, s.persistentStore.Reset())

	// This key was already cached, so it shouldn't see the change above.
	h.RequireNever(t,
		checkForUpdatedValue(t, client, "flag-key", context,
			ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default")),
		time.Millisecond*500, time.Millisecond*20, "flag was never updated")
}

func (s *ServerSidePersistentTests) doesNotCacheFlagMiss(t *ldtest.T, cacheConfig servicedef.SDKConfigPersistentCache) {
	require.NoError(t, s.persistentStore.Reset())

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
	s.eventuallyRequireDataStoreInit(t, "launchdarkly")

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
}

func (s *ServerSidePersistentTests) sdkReflectsDataSourceUpdatesEvenWithCache(t *ldtest.T, cacheConfig servicedef.SDKConfigPersistentCache) {
	require.NoError(t, s.persistentStore.Reset())

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
	s.eventuallyRequireDataStoreInit(t, "launchdarkly")

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
}

func (s *ServerSidePersistentTests) ignoresDroppedFlagsWithTTLCache(t *ldtest.T) {
	require.NoError(t, s.persistentStore.Reset())

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  s.persistentStore.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.TTL,
		TTL:  o.Some(1),
	})

	sdkData := s.makeSDKDataWithFlag("flag-key", 1, ldvalue.String("value"))
	_, configurers := s.setupDataSources(t, sdkData)
	configurers = append(configurers, persistence)

	client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)
	context := ldcontext.New("user-key")
	s.eventuallyRequireDataStoreInit(t, "launchdarkly")

	pollUntilFlagValueUpdated(t, client, "flag-key", context,
		ldvalue.String("default"), ldvalue.String("value"), ldvalue.String("default"))

	require.NoError(t, s.persistentStore.Reset())

	// The flag change isn't going to get noticed for some period of time.
	h.RequireNever(t,
		checkForUpdatedValue(t, client, "flag-key", context,
			ldvalue.String("value"), ldvalue.String("default"), ldvalue.String("default")),
		time.Millisecond*500, time.Millisecond*20, "flag-key was incorrectly cached")

	// But eventually, it will expire and then we will fetch it from the database.
	h.RequireEventually(t,
		checkForUpdatedValue(t, client, "flag-key", context,
			ldvalue.String("value"), ldvalue.String("default"), ldvalue.String("default")),
		time.Second, time.Millisecond*20, "flag-key was incorrectly cached")
}

func (s *ServerSidePersistentTests) eventuallyRequireDataStoreInit(t *ldtest.T, prefix string) {
	h.RequireEventually(t, func() bool {
		_, err := s.persistentStore.ReadField(prefix + ":$inited")
		return err == nil
	}, time.Second, time.Millisecond*20, prefix+":$inited key was not set")
}

func (s *ServerSidePersistentTests) eventuallyValidateFlagData(t *ldtest.T, prefix string, matchers map[string]m.Matcher) {
	h.RequireEventually(t, func() bool {
		data, err := s.persistentStore.ReadData(prefix + ":features")
		if err != nil {
			return false
		}

		return validateFlagData(data, matchers)
	}, time.Second, time.Millisecond*20, "flag data did not match")
}

func (s *ServerSidePersistentTests) neverValidateFlagData(t *ldtest.T, prefix string, matchers map[string]m.Matcher) {
	h.RequireNever(t, func() bool {
		data, err := s.persistentStore.ReadData(prefix + ":features")
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
