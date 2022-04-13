package sdktests

import (
	"fmt"
	"time"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldbuilders"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doServerSideDataStoreStreamUpdateTests(t *ldtest.T) {
	// These tests verify that the SDK's default data store correctly implements updates and
	// deletes with versioning.
	//
	// Since our mechanism for putting updates into the SDK is our stream test fixture, these
	// tests also verify that the stream is working correctly.

	flagKey := "flagkey"
	segmentKey := "segment-key"
	versionBefore := 100
	valueBefore := ldvalue.String("valueBefore")
	valueAfter := ldvalue.String("valueAfter")
	defaultValue := ldvalue.String("defaultValue")
	context := ldcontext.New("user-key")

	versionDeltaDesc := func(delta int) string {
		switch {
		case delta < 0:
			return "lower version"
		case delta > 0:
			return "higher version"
		default:
			return "same version"
		}
	}

	isAppliedDesc := func(b bool) string {
		if b {
			return "is applied"
		}
		return "is not applied"
	}

	for _, isDelete := range []bool{false, true} {
		operationDesc := "patch"
		if isDelete {
			operationDesc = "delete"
		}

		for _, versionDelta := range []int{1, 0, -1} {
			versionAfter := versionBefore + versionDelta
			shouldApply := versionDelta > 0

			flagTestDesc := fmt.Sprintf("flag %s with %s %s",
				operationDesc, versionDeltaDesc(versionDelta), isAppliedDesc(shouldApply))

			t.Run(flagTestDesc, func(t *ldtest.T) {
				flagBefore, flagAfter := makeFlagVersionsWithValues(flagKey, versionBefore, versionAfter, valueBefore, valueAfter)
				dataBefore := mockld.NewServerSDKDataBuilder().Flag(flagBefore).Build()
				stream := NewSDKDataSource(t, dataBefore)
				client := NewSDKClient(t, stream)

				actualValue1 := basicEvaluateFlag(t, client, flagKey, context, defaultValue)
				m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

				if isDelete {
					stream.Service().PushDelete("flags", flagKey, versionAfter)
				} else {
					stream.Service().PushUpdate("flags", flagKey, jsonhelpers.ToJSON(flagAfter))
				}

				expectedValueIfUpdated := valueAfter
				if isDelete {
					expectedValueIfUpdated = defaultValue
				}

				if shouldApply {
					pollUntilFlagValueUpdated(t, client, flagKey, context, valueBefore, expectedValueIfUpdated, defaultValue)
				} else {
					require.Never(
						t,
						checkForUpdatedValue(t, client, flagKey, context, valueBefore, expectedValueIfUpdated, defaultValue),
						time.Millisecond*100,
						time.Millisecond*20,
						"flag value was updated, but it should not have been",
					)
				}

				allFlags := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
				if shouldApply {
					if isDelete {
						assert.NotContains(t, allFlags.State, flagKey)
					} else {
						m.In(t).Assert(allFlags, EvalAllFlagsValueForKeyShouldEqual(flagKey, expectedValueIfUpdated))
					}
				} else {
					m.In(t).Assert(allFlags, EvalAllFlagsValueForKeyShouldEqual(flagKey, valueBefore))
				}
			})

			segmentTestDesc := fmt.Sprintf("segment %s with %s %s",
				operationDesc, versionDeltaDesc(versionDelta), isAppliedDesc(shouldApply))

			t.Run(segmentTestDesc, func(t *ldtest.T) {
				segmentBefore := ldbuilders.NewSegmentBuilder(segmentKey).Version(versionBefore).
					Included(context.Key()).Build()
				segmentAfter := ldbuilders.NewSegmentBuilder(segmentKey).Version(versionAfter).
					Build() // context is not included in segmentAfter

				// Configure this flag so that if the context is included in the segment, it returns variation 0
				// (valueBefore); otherwise it returns variation 1 (valueAfter).
				flag := makeFlagToCheckSegmentMatch(flagKey, segmentKey, valueAfter, valueBefore)

				dataBefore := mockld.NewServerSDKDataBuilder().Flag(flag).Segment(segmentBefore).Build()
				stream := NewSDKDataSource(t, dataBefore)
				client := NewSDKClient(t, stream)

				actualValue1 := basicEvaluateFlag(t, client, flagKey, context, ldvalue.Null())
				m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

				if isDelete {
					stream.Service().PushDelete("segments", segmentKey, versionAfter)
				} else {
					stream.Service().PushUpdate("segments", segmentKey, jsonhelpers.ToJSON(segmentAfter))
				}

				// If we successfully delete the segment, the effect is the same as if we had updated the
				// segment to not include the context. SDKs should treat "segment not found" as equivalent to
				// "context not included in segment"; they should _not_ treat this as an error that would
				// make the flag return a default value.
				expectedValueIfUpdated := valueAfter

				if shouldApply {
					pollUntilFlagValueUpdated(t, client, flagKey, context, valueBefore, valueAfter, defaultValue)
				} else {
					require.Never(
						t,
						checkForUpdatedValue(t, client, flagKey, context, valueBefore, expectedValueIfUpdated, defaultValue),
						time.Millisecond*100,
						time.Millisecond*20,
						"flag value was updated, but it should not have been",
					)
				}

				// Note that we can't directly test for the existence of a segment, as we can test for the
				// existence of a flag, because segments aren't surfaced at all in the SDK API.
			})
		}

		t.Run(fmt.Sprintf("flag %s for previously nonexistent flag is applied", operationDesc), func(t *ldtest.T) {
			version := 100
			flag := ldbuilders.NewFlagBuilder(flagKey).Version(100).
				On(false).OffVariation(0).Variations(valueAfter).Build()

			stream := NewSDKDataSource(t, mockld.EmptyServerSDKData())
			client := NewSDKClient(t, stream)

			actualValue1 := basicEvaluateFlag(t, client, flagKey, context, defaultValue)
			m.In(t).Assert(actualValue1, m.JSONEqual(defaultValue))

			if isDelete {
				stream.Service().PushDelete("flags", flagKey, version)

				// A delete for an unknown flag should be persisted by the SDK so it knows this version was
				// deleted. A subsequent update for the same flag with an equal or lower version should be ignored.
				stream.Service().PushUpdate("flags", flagKey, jsonhelpers.ToJSON(flag))
				require.Never(
					t,
					checkForUpdatedValue(t, client, flagKey, context, defaultValue, valueAfter, defaultValue),
					time.Millisecond*100,
					time.Millisecond*20,
					"flag update after deletion should have been ignored due to version; deletion was not persisted",
				)
			} else {
				stream.Service().PushUpdate("flags", flagKey, jsonhelpers.ToJSON(flag))

				pollUntilFlagValueUpdated(t, client, flagKey, context, defaultValue, valueAfter, defaultValue)
			}
		})

		t.Run(fmt.Sprintf("segment %s for previously nonexistent segment is applied", operationDesc), func(t *ldtest.T) {
			version := 100
			segment := ldbuilders.NewSegmentBuilder(segmentKey).Version(version).
				Included(context.Key()).Build()
			flag := makeFlagToCheckSegmentMatch(flagKey, segmentKey, valueBefore, valueAfter)

			dataBefore := mockld.NewServerSDKDataBuilder().Flag(flag).Build() // data does *not* include segment yet
			stream := NewSDKDataSource(t, dataBefore)
			client := NewSDKClient(t, stream)

			actualValue1 := basicEvaluateFlag(t, client, flagKey, context, ldvalue.Null())
			m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

			if isDelete {
				stream.Service().PushDelete("segments", segmentKey, version)

				// A delete for an unknown segment should be persisted by the SDK so it knows this version was
				// deleted. A subsequent update for the same segment with an equal or lower version should be ignored.
				stream.Service().PushUpdate("segments", segmentKey, jsonhelpers.ToJSON(segment))
				require.Never(
					t,
					checkForUpdatedValue(t, client, flagKey, context, valueBefore, valueAfter, defaultValue),
					time.Millisecond*100,
					time.Millisecond*20,
					"segment update after deletion should have been ignored due to version; deletion was not persisted",
				)
			} else {
				stream.Service().PushUpdate("segments", segmentKey, jsonhelpers.ToJSON(segment))

				// Now that the segment exists, the flag should return the "after" value
				pollUntilFlagValueUpdated(t, client, flagKey, context, valueBefore, valueAfter, defaultValue)
			}
		})
	}
}
