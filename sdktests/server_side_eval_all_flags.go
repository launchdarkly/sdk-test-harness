package sdktests

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"

	"github.com/stretchr/testify/assert"
)

// Note that all of the tests in this file assume that the SDK will produce a compact JSON
// representation of the flags state, by omitting all boolean properties that are false, and
// omitting all nullable properties that are null.

func RunServerSideEvalAllFlagsTests(t *ldtest.T) {
	t.Run("default behavior", doServerSideAllFlagsBasicTest)
	t.Run("with reasons", doServerSideAllFlagsWithReasonsTest)
	t.Run("experimentation", doServerSideAllFlagsExperimentationTest)
	t.Run("error in flag", doServerSideAllFlagsErrorInFlagTest)
	t.Run("client-side filter", doServerSideAllFlagsClientSideOnlyTest)
	t.Run("details only for tracked flags", doServerSideAllFlagsDetailsOnlyForTrackedFlagsTest)
	t.Run("client not ready", doServerSideAllFlagsClientNotReadyTest)
	t.Run("compact representations", doServerSideAllFlagsCompactRepresentationsTest)
}

func doServerSideAllFlagsBasicTest(t *ldtest.T) {
	flag1 := ldbuilders.NewFlagBuilder("flag1").Version(100).
		Variations(dummyValue0, ldvalue.String("value1")).
		On(false).OffVariation(1).
		Build()

	// flag2 has event tracking enabled
	flag2 := ldbuilders.NewFlagBuilder("flag2").Version(200).
		Variations(dummyValue0, dummyValue1, ldvalue.String("value2")).
		On(false).OffVariation(2).
		TrackEvents(true).
		Build()

	// flag3 has debugging enabled
	flag3DebugTime := ldtime.UnixMillisNow() + 100000
	flag3 := ldbuilders.NewFlagBuilder("flag3").Version(300).
		Variations(dummyValue0, dummyValue1, dummyValue2, ldvalue.String("value3")).
		On(false).OffVariation(3).
		DebugEventsUntilDate(flag3DebugTime).
		Build()

	// flag4 had debugging enabled, but the timestamp is in the past so debugging is no longer enabled
	flag4DebugTime := ldtime.UnixMillisNow() - 100000
	flag4 := ldbuilders.NewFlagBuilder("flag4").Version(400).
		Variations(dummyValue0, dummyValue1, dummyValue2, dummyValue3, ldvalue.String("value4")).
		On(false).OffVariation(4).
		DebugEventsUntilDate(flag4DebugTime).
		Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2, flag3, flag4)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	client := NewSDKClient(t, dataSource)
	user := lduser.NewUser("user-key")

	result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
		User: &user,
	})
	resultJSON, _ := json.Marshal(canonicalizeAllFlagsData(result.State))
	expectedJSON := `{
		"flag1": "value1",
		"flag2": "value2",
		"flag3": "value3",
		"flag4": "value4",
		"$flagsState": {
			"flag1": {
				"variation": 1, "version": 100
			},
			"flag2": {
				"variation": 2, "version": 200, "trackEvents": true
			},
			"flag3": {
				"variation": 3, "version": 300, "debugEventsUntilDate": ` + fmt.Sprintf("%d", flag3DebugTime) + `
			},
			"flag4": {
				"variation": 4, "version": 400, "debugEventsUntilDate": ` + fmt.Sprintf("%d", flag4DebugTime) + `
			}
		},
		"$valid": true
	}`
	m.In(t).Assert(resultJSON, m.JSONStrEqual(expectedJSON))
}

func doServerSideAllFlagsWithReasonsTest(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityAllFlagsWithReasons)

	// flag1 has reason "OFF"
	flag1 := ldbuilders.NewFlagBuilder("flag1").Version(100).
		Variations(dummyValue0, ldvalue.String("value1")).
		On(false).OffVariation(1).
		Build()

	// flag2 has reason "FALLTHROUGH"
	flag2 := ldbuilders.NewFlagBuilder("flag2").Version(200).
		Variations(dummyValue0, dummyValue1, ldvalue.String("value2")).
		On(true).FallthroughVariation(2).
		Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	client := NewSDKClient(t, dataSource)
	user := lduser.NewUser("user-key")

	result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
		User:        &user,
		WithReasons: true,
	})
	resultJSON, _ := json.Marshal(canonicalizeAllFlagsData(result.State))
	expectedJSON := `{
		"flag1": "value1",
		"flag2": "value2",
		"$flagsState": {
			"flag1": {
				"variation": 1, "version": 100, "reason": { "kind": "OFF" }
			},
			"flag2": {
				"variation": 2, "version": 200, "reason": { "kind": "FALLTHROUGH" }
			}
		},
		"$valid": true
	}`
	m.In(t).Assert(resultJSON, m.JSONStrEqual(expectedJSON))
}

func doServerSideAllFlagsExperimentationTest(t *ldtest.T) {
	// flag1 has experiment behavior because it's a fallthrough and has trackEventsFallthrough=true
	flag1 := ldbuilders.NewFlagBuilder("flag1").Version(100).
		Variations(dummyValue0, ldvalue.String("value1")).
		On(true).FallthroughVariation(1).
		TrackEventsFallthrough(true).
		Build()

	// flag2 has experiment behavior because it's a rule match and has trackEvents=true on that rule
	flag2 := ldbuilders.NewFlagBuilder("flag2").Version(200).
		Variations(dummyValue0, dummyValue1, ldvalue.String("value2")).
		On(true).FallthroughVariation(0).
		AddRule(ldbuilders.NewRuleBuilder().ID("rule0").Variation(2).TrackEvents(true)).
		Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	client := NewSDKClient(t, dataSource)
	user := lduser.NewUser("user-key")

	result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
		User: &user,
	})
	resultJSON, _ := json.Marshal(canonicalizeAllFlagsData(result.State))
	expectedJSON := `{
		"flag1": "value1",
		"flag2": "value2",
		"$flagsState": {
			"flag1": {
				"variation": 1, "version": 100, "reason": { "kind": "FALLTHROUGH" },
				"trackEvents": true, "trackReason": true
			},
			"flag2": {
				"variation": 2, "version": 200, "reason": { "kind": "RULE_MATCH", "ruleIndex": 0, "ruleId": "rule0" },
				"trackEvents": true, "trackReason": true
			}
		},
		"$valid": true
	}`
	m.In(t).Assert(resultJSON, m.JSONStrEqual(expectedJSON))
}

func doServerSideAllFlagsErrorInFlagTest(t *ldtest.T) {
	// This test verifies that 1. an error in evaluation of one flag does not prevent evaluation
	// of the rest of the flags, and 2. the failed flag is still included in the results, with a
	// value of null (and, if reasons are present, a reason that explains the error)

	// flag1 returns a MALFORMED_FLAG error due to an invalid offVariation
	flag1 := ldbuilders.NewFlagBuilder("flag1").Version(100).
		Variations(dummyValue0, dummyValue1).
		On(false).OffVariation(-1).
		Build()

	// flag2 does not have an error
	flag2 := ldbuilders.NewFlagBuilder("flag2").Version(200).
		Variations(dummyValue0, dummyValue1, ldvalue.String("value2")).
		On(false).OffVariation(2).
		Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	client := NewSDKClient(t, dataSource)
	user := lduser.NewUser("user-key")

	t.Run("without reasons", func(t *ldtest.T) {
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
			User: &user,
		})

		resultJSON, _ := json.Marshal(canonicalizeAllFlagsData(result.State))
		expectedJSON := `{
			"flag1": null,
			"flag2": "value2",
			"$flagsState": {
				"flag1": {
					"version": 100
				},
				"flag2": {
					"variation": 2, "version": 200
				}
			},
			"$valid": true
		}`
		m.In(t).Assert(resultJSON, m.JSONStrEqual(expectedJSON))
	})

	t.Run("with reasons", func(t *ldtest.T) {
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
			User:        &user,
			WithReasons: true,
		})

		resultJSON, _ := json.Marshal(canonicalizeAllFlagsData(result.State))

		expectedJSON := `{
			"flag1": null,
			"flag2": "value2",
			"$flagsState": {
				"flag1": {
					"version": 100, "reason": { "kind": "ERROR", "errorKind": "MALFORMED_FLAG" }
				},
				"flag2": {
					"variation": 2, "version": 200, "reason": { "kind": "OFF" }
				}
			},
			"$valid": true
		}`
		m.In(t).Assert(resultJSON, m.JSONStrEqual(expectedJSON))
	})
}

func doServerSideAllFlagsClientSideOnlyTest(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityAllFlagsClientSideOnly)

	flag1 := ldbuilders.NewFlagBuilder("server-side-1").Build()
	flag2 := ldbuilders.NewFlagBuilder("server-side-2").Build()
	flag3 := ldbuilders.NewFlagBuilder("client-side-1").SingleVariation(ldvalue.String("value1")).
		ClientSideUsingEnvironmentID(true).Build()
	flag4 := ldbuilders.NewFlagBuilder("client-side-2").SingleVariation(ldvalue.String("value2")).
		ClientSideUsingEnvironmentID(true).Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2, flag3, flag4)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	client := NewSDKClient(t, dataSource)
	user := lduser.NewUser("user-key")

	result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
		User:           &user,
		ClientSideOnly: true,
	})
	assert.Contains(t, result.State, flag3.Key)
	assert.Contains(t, result.State, flag4.Key)
	assert.NotContains(t, result.State, flag1.Key)
	assert.NotContains(t, result.State, flag2.Key)
}

func doServerSideAllFlagsDetailsOnlyForTrackedFlagsTest(t *ldtest.T) {
	// Note that it's only "version" and "reason" that are omitted for untracked flags in this mode.
	// The variation index always must be included, because it's necessary for summary events. The
	// point of this option is to save bandwidth for applications that don't care about evaluation
	// reasons in their front-end code, but still want those to show up in event data when
	// appropriate.

	t.RequireCapability(servicedef.CapabilityAllFlagsDetailsOnlyForTrackedFlags)

	// flag1 will have details removed because it's not in any of the other categories below
	flag1 := ldbuilders.NewFlagBuilder("flag1").Version(100).
		Variations(dummyValue0, ldvalue.String("value1")).
		On(false).OffVariation(1).
		Build()

	// flag2 has event tracking enabled, and a reason of OFF
	flag2 := ldbuilders.NewFlagBuilder("flag2").Version(200).
		Variations(dummyValue0, dummyValue1, ldvalue.String("value2")).
		On(false).OffVariation(2).
		TrackEvents(true).
		Build()

	// flag3 has debugging enabled, and a reason of OFF
	flag3DebugTime := ldtime.UnixMillisNow() + 100000
	flag3 := ldbuilders.NewFlagBuilder("flag3").Version(300).
		Variations(dummyValue0, dummyValue1, dummyValue2, ldvalue.String("value3")).
		On(false).OffVariation(3).
		DebugEventsUntilDate(flag3DebugTime).
		Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2, flag3)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	client := NewSDKClient(t, dataSource)
	user := lduser.NewUser("user-key")

	result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
		User:                       &user,
		WithReasons:                true,
		DetailsOnlyForTrackedFlags: true,
	})
	resultJSON, _ := json.Marshal(canonicalizeAllFlagsData(result.State))
	expectedJSON := `{
		"flag1": "value1",
		"flag2": "value2",
		"flag3": "value3",
		"$flagsState": {
			"flag1": {
				"variation": 1
			},
			"flag2": {
				"variation": 2, "version": 200, "reason": { "kind": "OFF" }, "trackEvents": true
			},
			"flag3": {
				"variation": 3, "version": 300, "reason": { "kind": "OFF" },
				"debugEventsUntilDate": ` + fmt.Sprintf("%d", flag3DebugTime) + `
			}
		},
		"$valid": true
	}`
	m.In(t).Assert(resultJSON, m.JSONStrEqual(expectedJSON))
}

func doServerSideAllFlagsClientNotReadyTest(t *ldtest.T) {
	dataSource := NewSDKDataSource(t, mockld.BlockingUnavailableSDKData(mockld.ServerSideSDK))
	client := NewSDKClient(t,
		WithConfig(servicedef.SDKConfigParams{StartWaitTimeMS: 1, InitCanFail: true}),
		dataSource)
	user := lduser.NewUser("user-key")

	result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
		User: &user,
	})
	resultJSON, _ := json.Marshal(result.State)
	expectedJSON := `{
		"$valid": false,
		"$flagsState": {}
	}`
	m.In(t).Assert(resultJSON, m.JSONStrEqual(expectedJSON))
}

func doServerSideAllFlagsCompactRepresentationsTest(t *ldtest.T) {
	t.NonCritical(`If this failed but the other 'all flags' tests passed, the SDK is including null-valued` +
		` properties within the $flagsState part of the representation. To save bandwidth, it's desirable` +
		` to omit such properties.`)

	flag1 := ldbuilders.NewFlagBuilder("flag1").Version(100).
		Variations(ldvalue.String("value1")).On(false).OffVariation(0).
		Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").Version(200).
		Variations(dummyValue0).On(false).OffVariation(-1).
		Build()

	data := mockld.NewServerSDKDataBuilder().Flag(flag1, flag2).Build()
	dataSource := NewSDKDataSource(t, data)
	client := NewSDKClient(t, dataSource)

	user := lduser.NewUser("user-key")

	result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
		User: &user,
	})

	resultJSON, _ := json.Marshal(result.State["$flagsState"])
	expectedMetadata := `{
		"flag1": {
			"variation": 0, "version": 100
		},
		"flag2": {
			"version": 200
		}
	}`
	m.In(t).Assert(resultJSON, m.JSONStrEqual(expectedMetadata))
}

// canonicalizeAllFlagsData transforms the JSON flags data to adjust for variable SDK behavior that
// we don't care about: 1. SDKs may or may not strip null properties in the metadata, so we'll
// strip them all; 2. SDKs are allowed to omit $valid, in which case it's assumed to be true.
func canonicalizeAllFlagsData(originalData map[string]ldvalue.Value) map[string]ldvalue.Value {
	ret := make(map[string]ldvalue.Value, len(originalData))
	for k, v := range originalData {
		if k == "$flagsState" {
			allMetadata := ldvalue.ObjectBuild()
			for k1, v1 := range v.AsValueMap().AsMap() {
				flagMetadata := ldvalue.ObjectBuild()
				for k2, v2 := range v1.AsValueMap().AsMap() {
					if !v2.IsNull() {
						flagMetadata.Set(k2, v2)
					}
				}
				allMetadata.Set(k1, flagMetadata.Build())
			}
			ret[k] = allMetadata.Build()
		} else {
			ret[k] = v
		}
	}
	if _, found := ret["$valid"]; !found {
		ret["$valid"] = ldvalue.Bool(true)
	}
	return ret
}
