package sdktests

import (
	"time"

	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

func (s *ServerSidePersistenceTests) doDaemonModeTests(t *ldtest.T) {
	t.Run("ignores database initialization flag", s.ignoresInitialization)
	t.Run("can disable cache", s.canDisableCache)
	t.Run("caches flag for duration", s.cachesFlagForDuration)
	t.Run("caches flag forever", s.cachesFlagForever)
}

func (s *ServerSidePersistenceTests) ignoresInitialization(t *ldtest.T) {
	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigDataSystemPersistenceStore{
		Type: servicedef.Redis,
		DSN:  s.persistence.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigDataSystemPersistenceCache{
		Mode: servicedef.TTL,
	})
	context := ldcontext.New("user-key")

	require.NoError(t, s.persistence.Reset())
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

	require.NoError(t, s.persistence.WriteData("launchdarkly:features", s.initialFlags))
	pollUntilFlagValueUpdated(t, client, "flag-key", context,
		ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))
}

func (s *ServerSidePersistenceTests) canDisableCache(t *ldtest.T) {
	require.NoError(t, s.persistence.Reset())
	require.NoError(t, s.persistence.WriteData("launchdarkly:features", s.initialFlags))

	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigDataSystemPersistenceStore{
		Type: servicedef.Redis,
		DSN:  s.persistence.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigDataSystemPersistenceCache{
		Mode: servicedef.Off,
	})

	context := ldcontext.New("user-key")

	client := NewSDKClient(t, persistence)
	pollUntilFlagValueUpdated(t, client, "flag-key", context,
		ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))

	// Completely reset the database so there are no valid flag definitions
	require.NoError(t, s.persistence.Reset())

	h.RequireEventually(t,
		checkForUpdatedValue(t, client, "flag-key", context,
			ldvalue.String("fallthrough"), ldvalue.String("default"), ldvalue.String("default")),
		time.Second, time.Millisecond*20, "flag value was NOT updated after cache TTL")
}

func (s *ServerSidePersistenceTests) cachesFlagForDuration(t *ldtest.T) {
	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigDataSystemPersistenceStore{
		Type: servicedef.Redis,
		DSN:  s.persistence.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigDataSystemPersistenceCache{
		Mode: servicedef.TTL,
		TTL:  o.Some(1),
	})
	context := ldcontext.New("user-key")

	t.Run("will cache found flag for TTL", func(t *ldtest.T) {
		require.NoError(t, s.persistence.Reset())
		client := NewSDKClient(t, persistence)

		require.NoError(t, s.persistence.WriteData("launchdarkly:features", s.initialFlags))

		pollUntilFlagValueUpdated(t, client, "flag-key", context,
			ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))

		// Completely reset the database so there are no valid flag definitions
		require.NoError(t, s.persistence.Reset())

		h.RequireNever(t,
			checkForUpdatedValue(t, client, "flag-key", context,
				ldvalue.String("fallthrough"), ldvalue.String("default"), ldvalue.String("default")),
			time.Millisecond*500, time.Millisecond*20, "flag value was updated before cache TTL")

		h.RequireEventually(t,
			checkForUpdatedValue(t, client, "flag-key", context,
				ldvalue.String("fallthrough"), ldvalue.String("default"), ldvalue.String("default")),
			time.Second, time.Millisecond*20, "flag value was NOT updated after cache TTL")
	})

	t.Run("will cache missing flag for TTL", func(t *ldtest.T) {
		require.NoError(t, s.persistence.Reset())
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

		require.NoError(t, s.persistence.WriteData("launchdarkly:features", s.initialFlags))

		h.RequireNever(t,
			checkForUpdatedValue(t, client, "flag-key", context,
				ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
			time.Microsecond*500, time.Millisecond*20, "flag value was updated before cache TTL")

		h.RequireEventually(t,
			checkForUpdatedValue(t, client, "flag-key", context,
				ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
			time.Second, time.Millisecond*20, "flag value was NOT updated after cache TTL")
	})
}

func (s *ServerSidePersistenceTests) cachesFlagForever(t *ldtest.T) {
	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigDataSystemPersistenceStore{
		Type: servicedef.Redis,
		DSN:  s.persistence.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigDataSystemPersistenceCache{
		Mode: servicedef.Infinite,
	})
	context := ldcontext.New("user-key")

	require.NoError(t, s.persistence.Reset())
	require.NoError(t, s.persistence.WriteData("launchdarkly:features", s.initialFlags))

	client := NewSDKClient(t, persistence)

	h.RequireEventually(t,
		checkForUpdatedValue(t, client, "flag-key", context,
			ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default")),
		time.Millisecond*500, time.Millisecond*20, "flag value was not changed")

	// Reset the store and verify that the flag value is still cached
	require.NoError(t, s.persistence.Reset())

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
}
