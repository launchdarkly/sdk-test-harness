package sdktests

import (
	"fmt"
	"time"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (c CommonStreamingTests) Updates(t *ldtest.T) {
	// These tests verify that updates and deletes received from the stream are correctly applied
	// by the SDK.
	//
	// Besides the processing of the stream itself, these tests also verify that the SDK's default data
	// store works correctly in terms of versioning. Since data store operations are not surfaced
	// separately in SDK APIs, it is not possible to test them in isolation.

	flagKey, segmentKey := "flagkey", "segment-key"
	versionBefore := 100
	valueBefore, valueAfter := ldvalue.String("valueBefore"), ldvalue.String("valueAfter")
	defaultValue := ldvalue.String("defaultValue")
	context := ldcontext.New("context-key")

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

	isAppliedDesc := func(b bool) string { return h.IfElse(b, "is applied", "is not applied") }
	stateVersion := 1

	for _, isDelete := range []bool{false, true} {
		operationDesc := h.IfElse(isDelete, "delete", "patch")

		for _, versionDelta := range []int{1, 0, -1} {
			versionAfter := versionBefore + versionDelta
			shouldApply := versionDelta > 0

			flagTestDesc := fmt.Sprintf("flag %s with %s %s",
				operationDesc, versionDeltaDesc(versionDelta), isAppliedDesc(shouldApply))

			t.Run(flagTestDesc, func(t *ldtest.T) {
				dataBefore := c.makeSDKDataWithFlag(flagKey, versionBefore, valueBefore)

				stream, configurers := c.setupDataSources(t, dataBefore)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

				actualValue1 := basicEvaluateFlag(t, client, flagKey, context, defaultValue)
				m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

				_, _ = stream.Endpoint().AwaitConnection(time.Second)

				if isDelete {
					stream.StreamingService().PushDelete("flag", flagKey, versionAfter)
				} else {
					updateData := c.makeFlagData(flagKey, versionAfter, valueAfter)
					stream.StreamingService().PushUpdate("flag", flagKey, versionAfter, updateData)
				}

				stateVersion++
				stream.StreamingService().PushPayloadTransferred("state", stateVersion)

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

			if !c.isClientSide {
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
					client := NewSDKClient(t, c.baseSDKConfigurationPlus(stream)...)

					actualValue1 := basicEvaluateFlag(t, client, flagKey, context, ldvalue.Null())
					m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

					if isDelete {
						stream.StreamingService().PushDelete("segment", segmentKey, versionAfter)
					} else {
						stream.StreamingService().PushUpdate(
							"segment", segmentKey, segmentAfter.Version, jsonhelpers.ToJSON(segmentAfter))
					}
					stateVersion++
					stream.StreamingService().PushPayloadTransferred("state", stateVersion)

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
		} // end of subtest group that varies by versionDelta

		t.Run(fmt.Sprintf("flag %s for previously nonexistent flag is applied", operationDesc), func(t *ldtest.T) {
			version := 100
			updateData := c.makeFlagData(flagKey, 100, valueAfter)

			stream, configurers := c.setupDataSources(t, nil)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

			actualValue1 := basicEvaluateFlag(t, client, flagKey, context, defaultValue)
			m.In(t).Assert(actualValue1, m.JSONEqual(defaultValue))

			_, _ = stream.Endpoint().AwaitConnection(time.Second)

			if isDelete {
				stream.StreamingService().PushDelete("flag", flagKey, version)

				// A delete for an unknown flag should be persisted by the SDK so it knows this version was
				// deleted. A subsequent update for the same flag with an equal or lower version should be ignored.
				stream.StreamingService().PushUpdate("flag", flagKey, version, updateData)
				stateVersion++
				stream.StreamingService().PushPayloadTransferred("state", stateVersion)
				h.RequireNever(
					t,
					checkForUpdatedValue(t, client, flagKey, context, defaultValue, valueAfter, defaultValue),
					time.Millisecond*100,
					time.Millisecond*20,
					"flag update after deletion should have been ignored due to version; deletion was not persisted",
				)
			} else {
				stream.StreamingService().PushUpdate("flag", flagKey, version, updateData)
				stateVersion++
				stream.StreamingService().PushPayloadTransferred("state", stateVersion)

				pollUntilFlagValueUpdated(t, client, flagKey, context, defaultValue, valueAfter, defaultValue)
			}
		})

		if !c.isClientSide {
			t.Run(fmt.Sprintf("segment %s for previously nonexistent segment is applied", operationDesc), func(t *ldtest.T) {
				version := 100
				segment := ldbuilders.NewSegmentBuilder(segmentKey).Version(version).
					Included(context.Key()).Build()
				flag := makeFlagToCheckSegmentMatch(flagKey, segmentKey, valueBefore, valueAfter)

				dataBefore := mockld.NewServerSDKDataBuilder().Flag(flag).Build() // data does *not* include segment yet
				stream := NewSDKDataSource(t, dataBefore)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(stream)...)

				actualValue1 := basicEvaluateFlag(t, client, flagKey, context, ldvalue.Null())
				m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

				if isDelete {
					stream.StreamingService().PushDelete("segment", segmentKey, version)

					// A delete for an unknown segment should be persisted by the SDK so it knows this version was
					// deleted. A subsequent update for the same segment with an equal or lower version should be ignored.
					stream.StreamingService().PushUpdate("segment", segmentKey, segment.Version, jsonhelpers.ToJSON(segment))
					stateVersion++
					stream.StreamingService().PushPayloadTransferred("state", stateVersion)
					require.Never(
						t,
						checkForUpdatedValue(t, client, flagKey, context, valueBefore, valueAfter, defaultValue),
						time.Millisecond*100,
						time.Millisecond*20,
						"segment update after deletion should have been ignored due to version; deletion was not persisted",
					)
				} else {
					stream.StreamingService().PushUpdate("segment", segmentKey, segment.Version, jsonhelpers.ToJSON(segment))
					stateVersion++
					stream.StreamingService().PushPayloadTransferred("state", stateVersion)

					// Now that the segment exists, the flag should return the "after" value
					pollUntilFlagValueUpdated(t, client, flagKey, context, valueBefore, valueAfter, defaultValue)
				}
			})
		}
	}
}

func (c CommonStreamingTests) makeSDKDataWithFlag(key string, version int, value ldvalue.Value) mockld.SDKData {
	if c.isClientSide {
		return mockld.NewClientSDKDataBuilder().
			Flag(key, c.makeClientSideFlag(key, version, value).ClientSDKFlag).
			Build()
	}
	return mockld.NewServerSDKDataBuilder().Flag(c.makeServerSideFlag(key, version, value)).Build()
}

func (c CommonStreamingTests) makeFlagData(key string, version int, value ldvalue.Value) []byte {
	if c.isClientSide {
		return jsonhelpers.ToJSON(c.makeClientSideFlag(key, version, value))
	}
	return jsonhelpers.ToJSON(c.makeServerSideFlag(key, version, value))
}

func (c CommonStreamingTests) makeClientSideFlag(
	key string,
	version int,
	value ldvalue.Value,
) mockld.ClientSDKFlagWithKey {
	return mockld.ClientSDKFlagWithKey{
		Key: key,
		ClientSDKFlag: mockld.ClientSDKFlag{
			Version: version,
			Value:   value,
		},
	}
}

func (c CommonStreamingTests) makeServerSideFlag(key string, version int, value ldvalue.Value) ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder(key).Version(version).
		On(false).OffVariation(0).Variations(value, ldvalue.String("other")).
		Build()
}
