package sdktests

import (
	"fmt"
	"time"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/assert"
)

func (c CommonStreamingTests) makeSDKDataWithFlag(version int, value ldvalue.Value) mockld.SDKData {
	if c.isClientSide {
		cd := mockld.NewClientSDKDataBuilder().
			Flag("flag-key", c.makeClientSideFlag("flag-key", version, value).ClientSDKFlag).
			Build()
		return mockld.FDv2SDKDataFromClientSDKData(cd, "xfer-full", "initial", "initial")
	}
	sd := mockld.NewServerSDKDataBuilder().
		Flag(c.makeServerSideFlag("flag-key", version, value)).
		Build()
	return mockld.FDv2SDKDataFromServerSDKData(sd, "xfer-full", "initial", "initial")
}

// fdv2ServerData builds FDv2 mock data from server-side flags with an explicit envelope.
func (c CommonStreamingTests) fdv2ServerData(
	intentCode, intentReason, state string, flags ...ldmodel.FeatureFlag,
) mockld.FDv2SDKData {
	b := mockld.NewServerSDKDataBuilder()
	if len(flags) > 0 {
		b.Flag(flags...)
	}
	return mockld.FDv2SDKDataFromServerSDKData(b.Build(), intentCode, intentReason, state)
}

// fdv2ClientData builds FDv2 mock data from client-side flag eval results with an explicit envelope.
func (c CommonStreamingTests) fdv2ClientData(
	intentCode, intentReason, state string, flags ...mockld.ClientSDKFlagWithKey,
) mockld.FDv2SDKData {
	b := mockld.NewClientSDKDataBuilder()
	for _, f := range flags {
		b.FullFlag(f)
	}
	return mockld.FDv2SDKDataFromClientSDKData(b.Build(), intentCode, intentReason, state)
}

func (c CommonStreamingTests) newFDv2SDKClient(t *ldtest.T, configurers ...SDKConfigurer) *SDKClient {
	return NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
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

// Updates verifies that the SDK applies FDv1 patch and delete events from the stream. An FDv1 SDK
// applies a change only if its version is higher than the version that the SDK has. (An FDv2 SDK
// applies every change that the server sends, so the FDv2 tests check the opposite.)
//
// Besides the processing of the stream itself, these tests also verify that the SDK's default data
// store works correctly in terms of versioning. Since data store operations are not surfaced
// separately in SDK APIs, it is not possible to test them in isolation.
func (c CommonStreamingTests) Updates(t *ldtest.T) {
	requireDataProtocol(t, mockld.FDv1)

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

	for _, isDelete := range []bool{false, true} {
		operationDesc := h.IfElse(isDelete, "delete", "patch")

		for _, versionDelta := range []int{1, 0, -1} {
			versionAfter := versionBefore + versionDelta
			shouldApply := versionDelta > 0

			flagTestDesc := fmt.Sprintf("flag %s with %s %s",
				operationDesc, versionDeltaDesc(versionDelta), isAppliedDesc(shouldApply))

			t.Run(flagTestDesc, func(t *ldtest.T) {
				dataBefore := c.makeSDKDataWithKeyedFlag(flagKey, versionBefore, valueBefore)

				dataSystem, configurers := c.setupDataSystems(t, dataBefore)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

				actualValue1 := basicEvaluateFlag(t, client, flagKey, context, defaultValue)
				m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

				_, _ = dataSystem.Synchronizers[0].Endpoint().AwaitConnection(time.Second)

				stream := dataSystem.Synchronizers[0].streaming
				if isDelete {
					stream.PushDelete("flag", flagKey, versionAfter)
				} else {
					stream.PushUpdate("flag", flagKey, versionAfter, c.makeFlagData(flagKey, versionAfter, valueAfter))
				}

				expectedValueIfUpdated := valueAfter
				if isDelete {
					expectedValueIfUpdated = defaultValue
				}

				if shouldApply {
					pollUntilFlagValueUpdated(t, client, flagKey, context, valueBefore, expectedValueIfUpdated, defaultValue)
				} else {
					h.RequireNever(
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
					dataSystem := NewSDKDataSystem(t, dataBefore)
					client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)

					actualValue1 := basicEvaluateFlag(t, client, flagKey, context, ldvalue.Null())
					m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

					stream := dataSystem.Synchronizers[0].streaming
					if isDelete {
						stream.PushDelete("segment", segmentKey, versionAfter)
					} else {
						stream.PushUpdate("segment", segmentKey, versionAfter, jsonhelpers.ToJSON(segmentAfter))
					}

					// If we successfully delete the segment, the effect is the same as if we had updated the
					// segment to not include the context. SDKs should treat "segment not found" as equivalent to
					// "context not included in segment"; they should _not_ treat this as an error that would
					// make the flag return a default value.
					expectedValueIfUpdated := valueAfter

					if shouldApply {
						pollUntilFlagValueUpdated(t, client, flagKey, context, valueBefore, valueAfter, defaultValue)
					} else {
						h.RequireNever(
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
			updateData := c.makeFlagData(flagKey, version, valueAfter)

			dataSystem, configurers := c.setupDataSystems(t, nil)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)

			actualValue1 := basicEvaluateFlag(t, client, flagKey, context, defaultValue)
			m.In(t).Assert(actualValue1, m.JSONEqual(defaultValue))

			_, _ = dataSystem.Synchronizers[0].Endpoint().AwaitConnection(time.Second)

			stream := dataSystem.Synchronizers[0].streaming
			if isDelete {
				stream.PushDelete("flag", flagKey, version)

				// A delete for an unknown flag should be persisted by the SDK so it knows this version was
				// deleted. A subsequent update for the same flag with an equal or lower version should be ignored.
				stream.PushUpdate("flag", flagKey, version, updateData)
				h.RequireNever(
					t,
					checkForUpdatedValue(t, client, flagKey, context, defaultValue, valueAfter, defaultValue),
					time.Millisecond*100,
					time.Millisecond*20,
					"flag update after deletion should have been ignored due to version; deletion was not persisted",
				)
			} else {
				stream.PushUpdate("flag", flagKey, version, updateData)

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
				dataSystem := NewSDKDataSystem(t, dataBefore)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)

				actualValue1 := basicEvaluateFlag(t, client, flagKey, context, ldvalue.Null())
				m.In(t).Assert(actualValue1, m.JSONEqual(valueBefore))

				stream := dataSystem.Synchronizers[0].streaming
				if isDelete {
					stream.PushDelete("segment", segmentKey, version)

					// A delete for an unknown segment should be persisted by the SDK so it knows this version was
					// deleted. A subsequent update for the same segment with an equal or lower version should be ignored.
					stream.PushUpdate("segment", segmentKey, version, jsonhelpers.ToJSON(segment))
					h.RequireNever(
						t,
						checkForUpdatedValue(t, client, flagKey, context, valueBefore, valueAfter, defaultValue),
						time.Millisecond*100,
						time.Millisecond*20,
						"segment update after deletion should have been ignored due to version; deletion was not persisted",
					)
				} else {
					stream.PushUpdate("segment", segmentKey, version, jsonhelpers.ToJSON(segment))

					// Now that the segment exists, the flag should return the "after" value
					pollUntilFlagValueUpdated(t, client, flagKey, context, valueBefore, valueAfter, defaultValue)
				}
			})
		}
	}
}

// makeSDKDataWithKeyedFlag returns a data set that has one flag, with the specified key, version, and value.
func (c CommonStreamingTests) makeSDKDataWithKeyedFlag(key string, version int, value ldvalue.Value) mockld.SDKData {
	if c.isClientSide {
		return mockld.NewClientSDKDataBuilder().Flag(key, c.makeClientSideFlag(key, version, value).ClientSDKFlag).Build()
	}
	return mockld.NewServerSDKDataBuilder().Flag(c.makeServerSideFlag(key, version, value)).Build()
}
