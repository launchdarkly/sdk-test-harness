package sdktests

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"

	"github.com/stretchr/testify/assert"
)

// Note that all of the tests in this file assume that the SDK will produce a compact JSON
// representation of the flags state, by omitting all boolean properties that are false, and
// omitting all nullable properties that are null.

var dummyValue0, dummyValue1, dummyValue2, dummyValue3 ldvalue.Value = ldvalue.String("a"), //nolint:gochecknoglobals
	ldvalue.String("b"), ldvalue.String("c"), ldvalue.String("d")

func RunServerSideEvalAllFlagsTests(t *ldtest.T) {
	t.Run("default behavior", doServerSideAllFlagsBasicTest)
	t.Run("with reasons", doServerSideAllFlagsWithReasonsTest)
	t.Run("client-side filter", doServerSideAllFlagsClientSideOnlyTest)
	t.Run("details only for tracked flags", doServerSideAllFlagsDetailsOnlyForTrackedFlagsTest)
	t.Run("experimentation", doServerSideAllFlagsExperimentationTest)
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

	// flag5 returns a MALFORMED_FLAG error due to an invalid offVariation
	flag5 := ldbuilders.NewFlagBuilder("flag5").Version(500).
		Variations(dummyValue0, dummyValue1).
		On(false).OffVariation(-1).
		Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2, flag3, flag4, flag5)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	client := NewSDKClient(t, dataSource)
	user := lduser.NewUser("user-key")

	result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
		User: &user,
	})
	resultJSON, _ := json.Marshal(result.State)
	expectedJSON := `{
		"flag1": "value1",
		"flag2": "value2",
		"flag3": "value3",
		"flag4": "value4",
		"flag5": null,
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
			},
			"flag5": {
				"version": 500
			}
		},
		"$valid": true
	}`
	assert.JSONEq(t, expectedJSON, string(resultJSON))
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

	// flag3 returns a MALFORMED_FLAG error due to an invalid offVariation
	flag3 := ldbuilders.NewFlagBuilder("flag3").Version(300).
		Variations(dummyValue0, dummyValue1).
		On(false).OffVariation(-1).
		Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2, flag3)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	client := NewSDKClient(t, dataSource)
	user := lduser.NewUser("user-key")

	result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
		User:        &user,
		WithReasons: true,
	})
	resultJSON, _ := json.Marshal(result.State)
	expectedJSON := `{
		"flag1": "value1",
		"flag2": "value2",
		"flag3": null,
		"$flagsState": {
			"flag1": {
				"variation": 1, "version": 100, "reason": { "kind": "OFF" }
			},
			"flag2": {
				"variation": 2, "version": 200, "reason": { "kind": "FALLTHROUGH" }
			},
			"flag3": {
				"version": 300, "reason": { "kind": "ERROR", "errorKind": "MALFORMED_FLAG" }
			}
		},
		"$valid": true
	}`
	assert.JSONEq(t, expectedJSON, string(resultJSON))
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
	// Note that it's really only the *reason* that is omitted for untracked flags in this mode.
	// The variation index and flag version always must be included, because they're used in
	// summary events.

	t.RequireCapability(servicedef.CapabilityAllFlagsDetailsOnlyForTrackedFlags)

	// flag1 does not get a reason because it's not in any of the other categories below
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

	// flag4 involves an experiment, and has a reason of FALLTHROUGH
	flag4 := ldbuilders.NewFlagBuilder("flag4").Version(400).
		Variations(dummyValue0, dummyValue1, dummyValue2, dummyValue3, ldvalue.String("value4")).
		On(true).FallthroughVariation(4).TrackEventsFallthrough(true).
		Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2, flag3, flag4)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	client := NewSDKClient(t, dataSource)
	user := lduser.NewUser("user-key")

	result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
		User:                       &user,
		WithReasons:                true,
		DetailsOnlyForTrackedFlags: true,
	})
	resultJSON, _ := json.Marshal(result.State)
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
				"variation": 2, "version": 200, "reason": { "kind": "OFF" }, "trackEvents": true
			},
			"flag3": {
				"variation": 3, "version": 300, "reason": { "kind": "OFF" },
				"debugEventsUntilDate": ` + fmt.Sprintf("%d", flag3DebugTime) + `
			},
			"flag4": {
				"variation": 4, "version": 400, "reason": { "kind": "FALLTHROUGH" }, "trackEvents": true, "trackReason": true
			}
		},
		"$valid": true
	}`
	assert.JSONEq(t, expectedJSON, string(resultJSON))
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
	resultJSON, _ := json.Marshal(result.State)
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
	assert.JSONEq(t, expectedJSON, string(resultJSON))
}

// func canonicalizeAllFlagsData(originalData map[string]ldvalue.Value) map[string]ldvalue.Value {
// 	ret := make(map[string]ldvalue.Value, len(originalData))
// 	for k, v := range originalData {
// 		if k != "$flagsState" {
// 			ret[k] = v
// 			continue
// 		}
// 		buildMeta := ldvalue.ObjectBuild()
// 		for flagKey, flagMetaValue := range v.AsValueMap().AsMap() {
// 			flagMetaMap := flagMetaValue.AsValueMap().AsArbitraryValueMap()
// 			setDefaultValue(flagMetaMap, "variation", nil)
// 			setDefaultValue(flagMetaMap, "reason", nil)
// 			setDefaultValue(flagMetaMap, "trackEvents", false)
// 			setDefaultValue(flagMetaMap, "trackReason", false)
// 			setDefaultValue(flagMetaMap, "debugEventsUntilDate", nil)
// 			buildMeta.Set(flagKey, ldvalue.CopyArbitraryValue(flagMetaMap))
// 		}
// 		ret[k] = buildMeta.Build()
// 	}
// 	return ret
// }

// func setDefaultValue(m map[string]interface{}, key string, defaultValue interface{}) {
// 	if _, ok := m[key]; !ok {
// 		m[key] = defaultValue
// 	}
// }
