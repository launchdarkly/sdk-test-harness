package sdktests

// Reading a whole collection is a different code path from reading one record
// by key, and its result changes shape with the number of records. A database
// client can return a lone matching record on its own instead of inside a list,
// and can report "nothing matched" with a sentinel instead of an empty list. A
// store wrapper that assumes a list fails the whole collection, so every flag
// falls back to its default.
//
// The rest of this suite only ever reads collections of three or four items, so
// these tests cover the two sizes it never produces: one and zero.

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/stretchr/testify/require"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

const (
	deletedOnlyFlagKey     = "deleted-flag-key"
	deletedOnlyFlagVersion = 100

	// emptyAllFlagsState is what all-flags returns when the collection holds
	// nothing to serve. The "$valid" property is the important part: a read that
	// failed reports false, and a read that returned nothing reports true.
	emptyAllFlagsState = `{"$valid": true, "$flagsState": {}}`
)

// doCollectionCardinalityTests belongs to the daemon mode sub-tree: there is no
// data source, so everything the SDK serves comes out of the store, and the
// cache is off so every read hits the store.
//
// All-flags is the trigger for the collection read. A single flag evaluation
// reads one record by key instead and so does not exercise it.
func (s *ServerSidePersistentTests) doCollectionCardinalityTests(t *ldtest.T) {
	context := ldcontext.New("user-key")

	s.runWithEmptyStore(t, "collection holds one flag", func(t *ldtest.T) {
		require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features",
			map[string]string{"flag-key": s.initialFlags["flag-key"]}))

		client := NewSDKClient(t, s.storeOnlyPersistence())

		// Reading the record by key works on a store that cannot read the
		// collection, which keeps the two failures apart.
		pollUntilFlagValueUpdated(t, client, "flag-key", context,
			ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))

		h.RequireEventually(t, func() bool {
			allFlags := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
				Context: o.Some(context),
			})
			matched, _ := EvalAllFlagsValueForKeyShouldEqual(
				"flag-key", ldvalue.String("fallthrough")).Test(allFlags)
			return matched
		}, time.Second, time.Millisecond*20,
			"all-flags never served the only flag in the collection")
	})

	s.runWithEmptyStore(t, "collection holds one flag and it is deleted", func(t *ldtest.T) {
		tombstone := fmt.Sprintf(`{"version": %d, "deleted": true}`, deletedOnlyFlagVersion)
		require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features",
			map[string]string{deletedOnlyFlagKey: tombstone}))

		client := NewSDKClient(t, s.storeOnlyPersistence())

		requireEmptyAllFlagsState(t, client, context)
		requireFlagNotFound(t, client, deletedOnlyFlagKey, context)
	})

	s.runWithEmptyStore(t, "collection holds no flags", func(t *ldtest.T) {
		// Nothing is written, so this is a store the SDK has never used.
		client := NewSDKClient(t, s.storeOnlyPersistence())

		requireEmptyAllFlagsState(t, client, context)
		requireFlagNotFound(t, client, "flag-key", context)
	})
}

// storeOnlyPersistence configures daemon mode: the store is the only source of
// data, and the cache is off so every read reaches it.
func (s *ServerSidePersistentTests) storeOnlyPersistence() *Persistence {
	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  s.persistentStore.DSN(),
	})
	persistence.SetCache(servicedef.SDKConfigPersistentCache{
		Mode: servicedef.CacheModeOff,
	})
	return persistence
}

// requireEmptyAllFlagsState waits for all-flags to report a successful read that
// found nothing to serve. It polls because an SDK can report an invalid state
// for a moment while it is still starting up.
func requireEmptyAllFlagsState(t *ldtest.T, client *SDKClient, context ldcontext.Context) {
	h.RequireEventually(t, func() bool {
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
			Context: o.Some(context),
		})
		state, err := json.Marshal(result.State)
		if err != nil {
			return false
		}
		matched, _ := m.JSONStrEqual(emptyAllFlagsState).Test(state)
		return matched
	}, time.Second, time.Millisecond*20,
		"all-flags never reported a valid, empty state; expected "+emptyAllFlagsState)
}

// requireFlagNotFound fails unless the flag looks to the SDK like a flag that
// was never in the store.
func requireFlagNotFound(t *ldtest.T, client *SDKClient, flagKey string, context ldcontext.Context) {
	result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flagKey,
		Context:      o.Some(context),
		ValueType:    servicedef.ValueTypeAny,
		DefaultValue: ldvalue.String("default"),
		Detail:       true,
	})
	m.In(t).Assert(result.Value, m.Equal(ldvalue.String("default")))
	m.In(t).Assert(result.Reason.Value().GetErrorKind(), m.Equal(ldreason.EvalErrorFlagNotFound))
}
