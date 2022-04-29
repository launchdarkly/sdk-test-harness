package sdktests

import (
	"fmt"
	"time"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (c CommonStreamingTests) Updates(t *ldtest.T) {
	// These tests verify that updates and deletes received from the stream are correctly applied
	// by the SDK.
	//
	// Besides the procesing of the stream itself, these tests also verify that the SDK's default data
	// store works correctly in terms of versioning. Since data store operations are not surfaced
	// separately in SDK APIs, it is not possible to test them in isolation.

	flagKey, segmentKey := "flagkey", "segment-key"
	versionBefore := 100
	valueBefore, valueAfter := ldvalue.String("valueBefore"), ldvalue.String("valueAfter")
	defaultValue := ldvalue.String("defaultValue")
	user := lduser.NewUser("user-key")

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

	for _, isDelete := range []bool{false, true} {
		operationDesc := h.IfElse(isDelete, "delete", "patch")

		for _, versionDelta := range []int{1, 0, -1} {
			versionAfter := versionBefore + versionDelta
			shouldApply := versionDelta > 0

			flagTestDesc := fmt.Sprintf("flag %s with %s %s",
				operationDesc, versionDeltaDesc(versionDelta), isAppliedDesc(shouldApply))

			t.Run(flagTestDesc, func(t *ldtest.T) {
				dataBefore := c.makeSDKDataWithFlag(flagKey, versionBefore, valueBefore)

				client, stream := c.setupClientWithInitialDataAndStream(t, dataBefore)

				actualValue1 := basicEvaluateFlag(t, client, flagKey, user, defaultValue)
				m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

				_, _ = stream.Endpoint().AwaitConnection(time.Second)

				if isDelete {
					stream.StreamingService().PushDelete("flags", flagKey, versionAfter)
				} else {
					updateData := c.makeFlagData(flagKey, versionAfter, valueAfter)
					stream.StreamingService().PushUpdate("flags", flagKey, updateData)
				}

				expectedValueIfUpdated := valueAfter
				if isDelete {
					expectedValueIfUpdated = defaultValue
				}

				if shouldApply {
					pollUntilFlagValueUpdated(t, client, flagKey, user, valueBefore, expectedValueIfUpdated, defaultValue)
				} else {
					require.Never(
						t,
						checkForUpdatedValue(t, client, flagKey, user, valueBefore, expectedValueIfUpdated, defaultValue),
						time.Millisecond*100,
						time.Millisecond*20,
						"flag value was updated, but it should not have been",
					)
				}

				allFlags := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: o.Some(user)})
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
						Included(user.GetKey()).Build()
					segmentAfter := ldbuilders.NewSegmentBuilder(segmentKey).Version(versionAfter).
						Build() // user is not included in segmentAfter

					// Configure this flag so that if the user is included in the segment, it returns variation 0
					// (valueBefore); otherwise it returns variation 1 (valueAfter).
					flag := makeFlagToCheckSegmentMatch(flagKey, segmentKey, valueAfter, valueBefore)

					dataBefore := mockld.NewServerSDKDataBuilder().Flag(flag).Segment(segmentBefore).Build()
					stream := NewSDKDataSource(t, dataBefore)
					client := NewSDKClient(t, c.baseSDKConfigurationPlus(stream)...)

					actualValue1 := basicEvaluateFlag(t, client, flagKey, user, ldvalue.Null())
					m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

					if isDelete {
						stream.StreamingService().PushDelete("segments", segmentKey, versionAfter)
					} else {
						stream.StreamingService().PushUpdate("segments", segmentKey, jsonhelpers.ToJSON(segmentAfter))
					}

					// If we successfully delete the segment, the effect is the same as if we had updated the
					// segment to not include the user. SDKs should treat "segment not found" as equivalent to
					// "user not included in segment"; they should _not_ treat this as an error that would
					// make the flag return a default value.
					expectedValueIfUpdated := valueAfter

					if shouldApply {
						pollUntilFlagValueUpdated(t, client, flagKey, user, valueBefore, valueAfter, defaultValue)
					} else {
						require.Never(
							t,
							checkForUpdatedValue(t, client, flagKey, user, valueBefore, expectedValueIfUpdated, defaultValue),
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

			client, stream := c.setupClientWithInitialDataAndStream(t, nil)

			actualValue1 := basicEvaluateFlag(t, client, flagKey, user, defaultValue)
			m.In(t).Assert(actualValue1, m.JSONEqual(defaultValue))

			_, _ = stream.Endpoint().AwaitConnection(time.Second)

			if isDelete {
				stream.StreamingService().PushDelete("flags", flagKey, version)

				// A delete for an unknown flag should be persisted by the SDK so it knows this version was
				// deleted. A subsequent update for the same flag with an equal or lower version should be ignored.
				stream.StreamingService().PushUpdate("flags", flagKey, updateData)
				require.Never(
					t,
					checkForUpdatedValue(t, client, flagKey, user, defaultValue, valueAfter, defaultValue),
					time.Millisecond*100,
					time.Millisecond*20,
					"flag update after deletion should have been ignored due to version; deletion was not persisted",
				)
			} else {
				stream.StreamingService().PushUpdate("flags", flagKey, updateData)

				pollUntilFlagValueUpdated(t, client, flagKey, user, defaultValue, valueAfter, defaultValue)
			}
		})

		if !c.isClientSide {
			t.Run(fmt.Sprintf("segment %s for previously nonexistent segment is applied", operationDesc), func(t *ldtest.T) {
				version := 100
				segment := ldbuilders.NewSegmentBuilder(segmentKey).Version(version).
					Included(user.GetKey()).Build()
				flag := makeFlagToCheckSegmentMatch(flagKey, segmentKey, valueBefore, valueAfter)

				dataBefore := mockld.NewServerSDKDataBuilder().Flag(flag).Build() // data does *not* include segment yet
				stream := NewSDKDataSource(t, dataBefore)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(stream)...)

				actualValue1 := basicEvaluateFlag(t, client, flagKey, user, ldvalue.Null())
				m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

				if isDelete {
					stream.StreamingService().PushDelete("segments", segmentKey, version)

					// A delete for an unknown segment should be persisted by the SDK so it knows this version was
					// deleted. A subsequent update for the same segment with an equal or lower version should be ignored.
					stream.StreamingService().PushUpdate("segments", segmentKey, jsonhelpers.ToJSON(segment))
					require.Never(
						t,
						checkForUpdatedValue(t, client, flagKey, user, valueBefore, valueAfter, defaultValue),
						time.Millisecond*100,
						time.Millisecond*20,
						"segment update after deletion should have been ignored due to version; deletion was not persisted",
					)
				} else {
					stream.StreamingService().PushUpdate("segments", segmentKey, jsonhelpers.ToJSON(segment))

					// Now that the segment exists, the flag should return the "after" value
					pollUntilFlagValueUpdated(t, client, flagKey, user, valueBefore, valueAfter, defaultValue)
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
