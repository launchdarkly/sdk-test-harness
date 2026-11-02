package sdktests

// Overrides apply whether or not LaunchDarkly data is available, and they change
// only the keys they override: a flag that the override layer does not hold
// evaluates as it does without an override source, and a source that supplies no
// flags leaves the SDK as it is without one. A client that has not initialized
// depends on these rules for both evaluations and the all-flags state.
//
// A store that is not initialized can still hold flags: a persistent store that
// holds data but no init key. The rest of the override tests use the SDK's
// in-memory store, which is empty until a data source provides data, so they
// cannot tell a read of the override layer apart from a read of everything the
// store returns.

import (
	"strings"
	"time"

	"github.com/stretchr/testify/require"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

// doFlagOverridesTests writes flags to the store without the init key, and the
// data source never provides data, so neither the client nor the store is
// initialized.
func (s *ServerSidePersistentTests) doFlagOverridesTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityFlagOverrides)

	context := ldcontext.New("user-key")

	s.runWithEmptyStore(t, "a flag without an override evaluates as it does without an override source",
		func(t *ldtest.T) {
			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

			withoutSource := s.uninitializedClient(t)
			overrides := overrideDocument{
				FlagValues: map[string]ldvalue.Value{"override-only-key": ldvalue.String("override-only-value")},
			}
			overrideFile := NewOverrideFile(t, overrides.String())
			withSource := s.uninitializedClient(t,
				WithFileOverrides(servicedef.SDKConfigOverridesParams{FilePaths: []string{overrideFile.Path}}))

			params := servicedef.EvaluateFlagParams{
				FlagKey:      "flag-key",
				Context:      o.Some(context),
				ValueType:    servicedef.ValueTypeAny,
				DefaultValue: ldvalue.String("default"),
				Detail:       true,
			}
			expected := withoutSource.EvaluateFlag(t, params)

			// The client without a source shows that the setup is uninitialized.
			m.In(t).Assert(expected.Value, m.JSONEqual("default"))
			m.In(t).Assert(withSource.EvaluateFlag(t, params), m.JSONEqual(expected))
		})

	s.runWithEmptyStore(t, "an empty override source leaves the all-flags state unchanged", func(t *ldtest.T) {
		require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

		withoutSource := s.uninitializedClient(t)
		emptyFile := NewOverrideFile(t, overrideDocument{}.String())
		withSource := s.uninitializedClient(t,
			WithFileOverrides(servicedef.SDKConfigOverridesParams{FilePaths: []string{emptyFile.Path}}))

		expected := canonicalizeAllFlagsData(withoutSource.EvaluateAllFlags(t,
			servicedef.EvaluateAllFlagsParams{Context: o.Some(context)}).State)
		actual := canonicalizeAllFlagsData(withSource.EvaluateAllFlags(t,
			servicedef.EvaluateAllFlagsParams{Context: o.Some(context)}).State)

		// The client without a source shows that the setup is uninitialized.
		m.In(t).Assert(expected["$valid"], m.JSONEqual(false))
		m.In(t).Assert(actual, m.JSONEqual(expected))
	})

	s.runWithEmptyStore(t, "overrides add to the all-flags state and make it valid", func(t *ldtest.T) {
		require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))

		withoutSource := s.uninitializedClient(t)
		overrides := overrideDocument{
			FlagValues: map[string]ldvalue.Value{
				"flag-key":          ldvalue.String("override-value"),
				"override-only-key": ldvalue.String("override-only-value"),
			},
		}
		overrideFile := NewOverrideFile(t, overrides.String())
		withSource := s.uninitializedClient(t,
			WithFileOverrides(servicedef.SDKConfigOverridesParams{FilePaths: []string{overrideFile.Path}}))

		baseline := canonicalizeAllFlagsData(withoutSource.EvaluateAllFlags(t,
			servicedef.EvaluateAllFlagsParams{Context: o.Some(context)}).State)
		state := canonicalizeAllFlagsData(withSource.EvaluateAllFlags(t,
			servicedef.EvaluateAllFlagsParams{Context: o.Some(context)}).State)

		// Overrides only add to the state. It holds every flag that it holds without
		// an override source, with the override's value for an overridden key, and
		// the flags that exist only in the override layer.
		expected := []m.KeyValueMatcher{
			m.KV("$valid", m.JSONEqual(true)),
			m.KV("$flagsState", m.Not(m.BeNil())),
			m.KV("flag-key", m.JSONEqual("override-value")),
			m.KV("override-only-key", m.JSONEqual("override-only-value")),
		}
		for key, value := range baseline {
			if _, overridden := overrides.FlagValues[key]; overridden || strings.HasPrefix(key, "$") {
				continue
			}
			expected = append(expected, m.KV(key, m.JSONEqual(value)))
		}
		m.In(t).Assert(state, m.MapOf(expected...))
	})
}

// uninitializedClient starts a client whose data source never provides data. It
// reads the store in read-only mode with no cache, so every read reaches the
// store and the SDK never writes the init key.
func (s *ServerSidePersistentTests) uninitializedClient(t *ldtest.T, configurers ...SDKConfigurer) *SDKClient {
	dataSystem := NewSDKDataSystem(t, mockld.BlockingUnavailableSDKData(mockld.ServerSideSDK))
	all := append([]SDKConfigurer{
		s.uncachedReadModePersistence(),
		dataSystem,
		WithWaitToStart(time.Millisecond, true),
	}, configurers...)
	return NewSDKClient(t, all...)
}
