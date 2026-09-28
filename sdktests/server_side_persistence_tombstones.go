package sdktests

// A tombstone is the record an SDK writes to a persistent store to mark an item
// deleted. Version-based conflict resolution needs it: with no record at all, a
// stale out-of-order update would bring the item back.
//
// The store already addresses the record by key - the Redis hash field, the
// Consul key, or the DynamoDB sort key - so the key inside the JSON body is
// redundant. Our SDKs write three different shapes anyway, and any SDK can be
// pointed at a store that a different SDK wrote. These tests prove that a
// reader accepts all three shapes, does not depend on the inner key, and does
// not bring a deleted item back to life.

import (
	"fmt"
	"maps"

	"github.com/stretchr/testify/require"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

const (
	// tombstonePlaceholderKey is what Go v5+, the Relay Proxy, and Rust put in
	// the body of a full-object tombstone in place of the real key.
	tombstonePlaceholderKey = "$deleted"

	tombstonedFlagKey    = "tombstoned-flag-key"
	tombstonedSegmentKey = "tombstoned-segment-key"
	segmentMatchFlagKey  = "segment-match-flag-key"

	tombstoneVersion = 100
)

// tombstoneShape is one encoding of a deleted item, as written by some of our SDKs.
type tombstoneShape struct {
	name string
	body string
}

// keylessTombstone is written by .NET, Java, Node (Redis upsert path), and Haskell.
func keylessTombstone(version int) string {
	return fmt.Sprintf(`{"version":%d,"deleted":true}`, version)
}

// keyedTombstone is written by Python, Ruby, C++, Erlang, and Node (init path).
// The key it carries is the real key of the item, which the store already knows.
func keyedTombstone(key string, version int) string {
	return fmt.Sprintf(`{"key":%q,"version":%d,"deleted":true}`, key, version)
}

func flagTombstoneShapes(t *ldtest.T, key string) []tombstoneShape {
	// A full flag object keyed "$deleted" is written by Go v5+, the Relay Proxy,
	// and Rust. The real key survives only as the store's own address for the
	// record.
	placeholderFlag, err := ldbuilders.NewFlagBuilder(tombstonePlaceholderKey).
		Version(tombstoneVersion).Deleted(true).Build().MarshalJSON()
	require.NoError(t, err)

	return []tombstoneShape{
		{name: "body has no key", body: keylessTombstone(tombstoneVersion)},
		{name: "body repeats the item key", body: keyedTombstone(key, tombstoneVersion)},
		{name: "body is a full object keyed " + tombstonePlaceholderKey, body: string(placeholderFlag)},
	}
}

// segmentTombstoneShapes builds the same three shapes for a deleted segment.
// The full-object shape includes includedKey so that a reader which treats the
// tombstone as a live segment produces a segment match we can observe.
func segmentTombstoneShapes(t *ldtest.T, key string, includedKey string) []tombstoneShape {
	placeholder := ldbuilders.NewSegmentBuilder(tombstonePlaceholderKey).
		Version(tombstoneVersion).Included(includedKey).Build()
	placeholder.Deleted = true
	placeholderSegment, err := placeholder.MarshalJSON()
	require.NoError(t, err)

	return []tombstoneShape{
		{name: "body has no key", body: keylessTombstone(tombstoneVersion)},
		{name: "body repeats the item key", body: keyedTombstone(key, tombstoneVersion)},
		{name: "body is a full object keyed " + tombstonePlaceholderKey, body: string(placeholderSegment)},
	}
}

// doTombstoneTests runs the tombstone-shape tests. It belongs to the daemon mode
// sub-tree: there is no data source, so everything the SDK serves comes straight
// out of the store, and the cache is off so that every read hits the store.
func (s *ServerSidePersistentTests) doTombstoneTests(t *ldtest.T) {
	t.Run("flags", s.doFlagTombstoneTests)
	t.Run("segments", s.doSegmentTombstoneTests)
}

func (s *ServerSidePersistentTests) daemonModePersistence() *Persistence {
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

func (s *ServerSidePersistentTests) doFlagTombstoneTests(t *ldtest.T) {
	context := ldcontext.New("user-key")

	for _, shape := range flagTombstoneShapes(t, tombstonedFlagKey) {
		s.runWithEmptyStore(t, shape.name, func(t *ldtest.T) {
			features := maps.Clone(s.initialFlags)
			features[tombstonedFlagKey] = shape.body
			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", features))

			client := NewSDKClient(t, s.daemonModePersistence())

			// One unrecognized record must not spoil the whole collection.
			pollUntilFlagValueUpdated(t, client, "flag-key", context,
				ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))

			// The deleted flag must not come back as a live flag.
			allFlags := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
				Context: o.Some(context),
			})
			m.In(t).Assert(allFlags, EvalAllFlagsStateMap().Should(
				m.ValueForKey("flag-key").Should(m.JSONEqual(ldvalue.String("fallthrough")))))
			requireFlagAbsentFromAllFlags(t, allFlags, tombstonedFlagKey)

			// Asking for the deleted flag must look the same as asking for a flag
			// that was never there.
			result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      tombstonedFlagKey,
				Context:      o.Some(context),
				ValueType:    servicedef.ValueTypeAny,
				DefaultValue: ldvalue.String("default"),
				Detail:       true,
			})
			m.In(t).Assert(result.Value, m.Equal(ldvalue.String("default")))
			m.In(t).Assert(result.Reason.Value().GetErrorKind(), m.Equal(ldreason.EvalErrorFlagNotFound))
		})
	}
}

func (s *ServerSidePersistentTests) doSegmentTombstoneTests(t *ldtest.T) {
	context := ldcontext.New("user-key")

	// This flag matches the deleted segment. A deleted segment matches nothing,
	// so the flag must serve "not-matched". If the SDK reads the tombstone as a
	// live segment instead, the full-object shape includes this context and the
	// flag serves "matched".
	segmentMatchFlag, err := makeFlagToCheckSegmentMatch(segmentMatchFlagKey, tombstonedSegmentKey,
		ldvalue.String("not-matched"), ldvalue.String("matched")).MarshalJSON()
	require.NoError(t, err)

	for _, shape := range segmentTombstoneShapes(t, tombstonedSegmentKey, context.Key()) {
		s.runWithEmptyStore(t, shape.name, func(t *ldtest.T) {
			features := maps.Clone(s.initialFlags)
			features[segmentMatchFlagKey] = string(segmentMatchFlag)
			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", features))
			require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "segments",
				map[string]string{tombstonedSegmentKey: shape.body}))

			client := NewSDKClient(t, s.daemonModePersistence())

			// The flags are still readable with a tombstone sitting in the
			// segments collection.
			pollUntilFlagValueUpdated(t, client, "flag-key", context,
				ldvalue.String("default"), ldvalue.String("fallthrough"), ldvalue.String("default"))

			// Reading the tombstone must neither fail the evaluation nor produce a
			// segment match.
			result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      segmentMatchFlagKey,
				Context:      o.Some(context),
				ValueType:    servicedef.ValueTypeAny,
				DefaultValue: ldvalue.String("default"),
				Detail:       true,
			})
			m.In(t).Assert(result.Value, m.Equal(ldvalue.String("not-matched")))
		})
	}
}

// requireFlagAbsentFromAllFlags fails if all-flags state has anything at all for
// the given key, either a value or an entry in the flag metadata.
func requireFlagAbsentFromAllFlags(
	t *ldtest.T, response servicedef.EvaluateAllFlagsResponse, flagKey string) {
	_, present := response.State[flagKey]
	require.False(t, present, "all-flags returned a value for deleted flag %q", flagKey)

	_, present = response.State["$flagsState"].AsValueMap().TryGet(flagKey)
	require.False(t, present, "all-flags returned metadata for deleted flag %q", flagKey)
}
