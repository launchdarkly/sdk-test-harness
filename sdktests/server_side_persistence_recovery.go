package sdktests

// These tests verify how an SDK handles a persistent store outage:
// - The SDK keeps serving evaluations from memory while the store is
//   unreachable. It applies streamed updates to memory first.
// - The SDK monitors the store. Once the store is reachable again, the
//   SDK writes its entire in-memory state back to it. A flag deleted
//   during the outage must stay deleted after the write back.
// - If a write back fails, the SDK monitors again and retries.
//
// A TCP proxy sits between the SDK and the real store. Break() simulates
// the outage. The harness reads the real store directly, so the store
// assertions work while the proxy is broken.

import (
	"fmt"
	"strings"
	"time"

	"github.com/stretchr/testify/require"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

const (
	recoveryPrimaryFlagKey   = "flag-key"
	recoverySecondaryFlagKey = "other-flag-key"

	// A flag key the SDK never sends. The harness writes it directly to
	// the store during an outage. This bypasses the SDK and proves that
	// a write back replaces the whole feature set, not just the SDK's
	// own missed deltas.
	recoveryGhostFlagKey = "ghost-flag-key"

	// The delta payload pushed during an outage uses this version.
	recoveryUpdateVersion = 2

	// Availability checks run roughly every 500ms. A write back follows
	// a successful check. Use a generous window.
	recoveryWindow = 10 * time.Second
)

// recoveryTestEnv holds the moving parts for one store recovery scenario.
type recoveryTestEnv struct {
	proxy      *harness.TCPProxy
	dataSystem *SDKDataSystem
	client     *SDKClient
	context    ldcontext.Context
}

// setupRecoveryTest starts an SDK client whose store DSN points at a TCP
// proxy in front of the real store. It seeds a basis with two flags and
// waits until the SDK has populated the store through the proxy.
func (s *ServerSidePersistentTests) setupRecoveryTest(
	t *ldtest.T, cache servicedef.SDKConfigPersistentCache,
) recoveryTestEnv {
	proxy, err := harness.NewTCPProxy(s.persistentStore.Addr())
	require.NoError(t, err)
	t.Defer(proxy.Close)

	flagA := s.makeServerSideFlag(recoveryPrimaryFlagKey, 1, ldvalue.String("value"))
	flagB := s.makeServerSideFlag(recoverySecondaryFlagKey, 1, ldvalue.String("other-value"))
	serverData := mockld.NewServerSDKDataBuilder().Flag(flagA, flagB).Build()
	sdkData := mockld.FDv2SDKDataFromServerSDKData(serverData, "xfer-full", "initial", "initial")

	dataSystem, configurers := s.setupDataSystems(t, sdkData)

	persistence := NewPersistence()
	persistence.SetStoreMode(servicedef.DataStoreModeReadWrite)
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  s.persistentStore.DSNFor(proxy.Addr()),
	})
	persistence.SetCache(cache)
	configurers = append(configurers, persistence)

	client := NewSDKClient(t, s.baseSDKConfigurationPlus(configurers...)...)

	// Deferred cleanups run last-in-first-out, so this runs before the
	// SDK client is destroyed. The store becomes reachable again first.
	// Otherwise a client that flushes to the store on close would hang
	// the teardown while the proxy is still broken or armed.
	t.Defer(proxy.Restore)

	// The SDK writes through the proxy. The harness reads the real
	// store directly. Wait for the initial full population.
	s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
	s.eventuallyValidateFlagDataAfterRecovery(t, s.defaultPrefix, map[string]m.Matcher{
		recoveryPrimaryFlagKey:   basicFlagValidationMatcher(recoveryPrimaryFlagKey, 1, "value"),
		recoverySecondaryFlagKey: basicFlagValidationMatcher(recoverySecondaryFlagKey, 1, "other-value"),
	})

	return recoveryTestEnv{
		proxy:      proxy,
		dataSystem: dataSystem,
		client:     client,
		context:    ldcontext.New("user-key"),
	}
}

// pushOutageUpdates streams one delta payload. It updates the primary
// flag to the given value and deletes the secondary flag.
func (s *ServerSidePersistentTests) pushOutageUpdates(env recoveryTestEnv, value string) {
	updated := s.makeFlagData(recoveryPrimaryFlagKey, recoveryUpdateVersion, ldvalue.String(value))
	env.dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", recoveryPrimaryFlagKey, recoveryUpdateVersion, updated)
	env.dataSystem.Synchronizers[0].streaming.PushDelete(
		"flag", recoverySecondaryFlagKey, recoveryUpdateVersion)
	env.dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", recoveryUpdateVersion)
}

// injectGhostFlag writes an extra flag straight into the real store,
// bypassing the SDK and the proxy. A write back must replace the whole
// feature set from memory, so this ghost flag must be gone once the
// write back completes.
//
// WriteMap replaces the whole map for some stores. This reads the
// current contents first, then writes back their union with the ghost
// flag instead of writing the ghost flag alone.
func (s *ServerSidePersistentTests) injectGhostFlag(t *ldtest.T) {
	current, err := s.persistentStore.GetMap(s.defaultPrefix, "features")
	require.NoError(t, err)

	withGhost := make(map[string]string, len(current)+1)
	for k, v := range current {
		withGhost[k] = v
	}
	withGhost[recoveryGhostFlagKey] = string(s.makeFlagData(recoveryGhostFlagKey, 1, ldvalue.String("ghost")))

	require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", withGhost))
}

// recoveryUpdatedStateMatchers describes the exact expected store state
// after pushOutageUpdates(value) has been written back. It expects the
// updated primary flag plus a tombstone for the deleted secondary flag.
// The full state must match, so a resurrected or extra flag fails.
func recoveryUpdatedStateMatchers(value string) map[string]m.Matcher {
	return map[string]m.Matcher{
		recoveryPrimaryFlagKey: basicFlagValidationMatcher(
			recoveryPrimaryFlagKey, recoveryUpdateVersion, value),
		recoverySecondaryFlagKey: basicDeletedFlagValidationMatcher(
			recoverySecondaryFlagKey, recoveryUpdateVersion),
	}
}

// eventuallyValidateFlagDataAfterRecovery is eventuallyValidateFlagData
// with a window wide enough for an availability check interval plus a
// full write back.
func (s *ServerSidePersistentTests) eventuallyValidateFlagDataAfterRecovery(
	t *ldtest.T, prefix string, matchers map[string]m.Matcher,
) {
	h.RequireEventually(t, func() bool {
		data, err := s.persistentStore.GetMap(prefix, "features")
		if err != nil {
			return false
		}
		return validateFlagData(data, matchers)
	}, recoveryWindow, time.Millisecond*50, "flag data did not match")
}

// runStoreRecoveryTests holds the store outage and write back scenarios.
func (s *ServerSidePersistentTests) runStoreRecoveryTests(t *ldtest.T) {
	s.runWithEmptyStore(t, "serves updates from memory during outage", func(t *ldtest.T) {
		env := s.setupRecoveryTest(t, servicedef.SDKConfigPersistentCache{Mode: servicedef.CacheModeOff})

		env.proxy.Break()
		s.pushOutageUpdates(env, "new-value")

		// The SDK applies updates to memory first and keeps serving from
		// memory while the store is unreachable.
		pollUntilFlagValueUpdated(t, env.client, recoveryPrimaryFlagKey, env.context,
			ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default"))

		// The deleted flag returns the default value.
		pollUntilFlagValueUpdated(t, env.client, recoverySecondaryFlagKey, env.context,
			ldvalue.String("other-value"), ldvalue.String("default"), ldvalue.String("default"))

		// The store never receives the new state while the proxy is broken.
		s.neverValidateFlagData(t, s.defaultPrefix, recoveryUpdatedStateMatchers("new-value"))

		// The store still holds the exact pre-outage contents.
		data, err := s.persistentStore.GetMap(s.defaultPrefix, "features")
		require.NoError(t, err)
		require.True(t, validateFlagData(data, map[string]m.Matcher{
			recoveryPrimaryFlagKey:   basicFlagValidationMatcher(recoveryPrimaryFlagKey, 1, "value"),
			recoverySecondaryFlagKey: basicFlagValidationMatcher(recoverySecondaryFlagKey, 1, "other-value"),
		}), "store contents changed during the outage")
	})

	cacheConfigs := map[string]servicedef.SDKConfigPersistentCache{
		"no cache":       {Mode: servicedef.CacheModeOff},
		"ttl cache":      {Mode: servicedef.CacheModeTTL, TTL: o.Some(1)},
		"infinite cache": {Mode: servicedef.CacheModeInfinite},
	}
	for cacheDesc, cacheConfig := range cacheConfigs {
		s.runWithEmptyStore(t, fmt.Sprintf("%s - writes back full state on recovery", cacheDesc),
			func(t *ldtest.T) {
				env := s.setupRecoveryTest(t, cacheConfig)

				env.proxy.Break()
				s.pushOutageUpdates(env, "new-value")

				// Wait until the SDK has applied the payload to memory.
				pollUntilFlagValueUpdated(t, env.client, recoveryPrimaryFlagKey, env.context,
					ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default"))

				// A flag that only exists directly in the store, never in
				// SDK memory. The write back must remove it.
				s.injectGhostFlag(t)

				env.proxy.Restore()

				// On recovery the SDK writes its entire in-memory state to
				// the store. The store must match memory exactly: the
				// updated flag plus a tombstone for the deleted flag, and
				// nothing else. The deleted flag must not come back. The
				// ghost flag must be gone.
				s.eventuallyValidateFlagDataAfterRecovery(t, s.defaultPrefix,
					recoveryUpdatedStateMatchers("new-value"))

				// Evaluations still serve the in-memory values afterward.
				m.In(t).Assert(
					basicEvaluateFlag(t, env.client, recoveryPrimaryFlagKey, env.context,
						ldvalue.String("default")),
					m.Equal(ldvalue.String("new-value")))
			})
	}

	s.runWithEmptyStore(t, "retries write back after another failure", func(t *ldtest.T) {
		env := s.setupRecoveryTest(t, servicedef.SDKConfigPersistentCache{Mode: servicedef.CacheModeOff})

		// A large flag value, so a write back of it cannot fit in one
		// small burst. This makes the byte-threshold cut below land
		// inside the write back's transfer instead of missing it.
		retryValue := strings.Repeat("x", 128*1024)

		env.proxy.Break()
		s.pushOutageUpdates(env, retryValue)
		pollUntilFlagValueUpdated(t, env.client, recoveryPrimaryFlagKey, env.context,
			ldvalue.String("value"), ldvalue.String(retryValue), ldvalue.String("default"))

		// A flag that only exists directly in the store, never in SDK
		// memory. The retried write back must remove it too.
		s.injectGhostFlag(t)

		// Arm a cut partway through the next transfer. This threshold
		// is well clear of availability-check chatter. It is also well
		// under each store's item or transaction size limits, so only
		// the large write back payload can cross it.
		env.proxy.BreakAfterBytes(64 * 1024)
		h.RequireEventually(t, env.proxy.Broken, recoveryWindow, time.Millisecond*50,
			"a sustained transfer above the threshold was never cut")

		// The cut landed inside the first large flag value, so the full
		// new state (both keys) cannot be present yet. The ghost flag is
		// still present too. This checks that the delta-replay end state
		// never appears while broken. It is not just an incidental
		// key-count mismatch from the ghost.
		//
		// On Redis a severed full population may already have emptied
		// the feature set before the cut lands, because its DEL runs
		// first. This means the ghost here does not discriminate by
		// itself. That discrimination lives in the cache-mode and
		// repeated-outage scenarios below.
		neverMatchers := recoveryUpdatedStateMatchers(retryValue)
		neverMatchers[recoveryGhostFlagKey] = basicFlagValidationMatcher(recoveryGhostFlagKey, 1, "ghost")
		s.neverValidateFlagData(t, s.defaultPrefix, neverMatchers)

		// After the store is reachable again, monitoring resumes and a
		// write back completes. This also repairs the cut write back
		// and removes the ghost flag.
		env.proxy.Restore()
		s.eventuallyValidateFlagDataAfterRecovery(t, s.defaultPrefix,
			recoveryUpdatedStateMatchers(retryValue))
	})

	s.runWithEmptyStore(t, "recovers from repeated outages", func(t *ldtest.T) {
		env := s.setupRecoveryTest(t, servicedef.SDKConfigPersistentCache{Mode: servicedef.CacheModeOff})

		// First outage: update one flag and delete the other.
		env.proxy.Break()
		s.pushOutageUpdates(env, "new-value")
		pollUntilFlagValueUpdated(t, env.client, recoveryPrimaryFlagKey, env.context,
			ldvalue.String("value"), ldvalue.String("new-value"), ldvalue.String("default"))

		// A flag that only exists directly in the store, never in SDK
		// memory. The first write back must remove it.
		s.injectGhostFlag(t)

		env.proxy.Restore()
		s.eventuallyValidateFlagDataAfterRecovery(t, s.defaultPrefix,
			recoveryUpdatedStateMatchers("new-value"))

		// Second outage after a completed recovery: monitoring must arm
		// again for a new failure.
		env.proxy.Break()
		updated := s.makeFlagData(recoveryPrimaryFlagKey, 3, ldvalue.String("third-value"))
		env.dataSystem.Synchronizers[0].streaming.PushUpdate("flag", recoveryPrimaryFlagKey, 3, updated)
		env.dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("updated", 3)
		pollUntilFlagValueUpdated(t, env.client, recoveryPrimaryFlagKey, env.context,
			ldvalue.String("new-value"), ldvalue.String("third-value"), ldvalue.String("default"))

		// A second ghost flag, injected during the second outage. This
		// proves a later write back still does full population, not
		// just the first one.
		s.injectGhostFlag(t)

		// The store never sees the third version while broken. The
		// tombstone stays at the version of the first delete. The ghost
		// flag from the second outage is still present too. This checks
		// that the delta-replay end state never appears while broken.
		// It is not just an incidental key-count mismatch from the
		// ghost.
		s.neverValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
			recoveryPrimaryFlagKey: basicFlagValidationMatcher(recoveryPrimaryFlagKey, 3, "third-value"),
			recoverySecondaryFlagKey: basicDeletedFlagValidationMatcher(
				recoverySecondaryFlagKey, recoveryUpdateVersion),
			recoveryGhostFlagKey: basicFlagValidationMatcher(recoveryGhostFlagKey, 1, "ghost"),
		})

		env.proxy.Restore()

		// The second write back lands the third version. The flag
		// deleted in the first outage stays deleted. The ghost flag
		// from the second outage is gone.
		s.eventuallyValidateFlagDataAfterRecovery(t, s.defaultPrefix, map[string]m.Matcher{
			recoveryPrimaryFlagKey: basicFlagValidationMatcher(recoveryPrimaryFlagKey, 3, "third-value"),
			recoverySecondaryFlagKey: basicDeletedFlagValidationMatcher(
				recoverySecondaryFlagKey, recoveryUpdateVersion),
		})
	})
}
