package sdktests

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
)

var dummyValue0, dummyValue1, dummyValue2, dummyValue3 ldvalue.Value = ldvalue.String("a"), //nolint:gochecknoglobals
	ldvalue.String("b"), ldvalue.String("c"), ldvalue.String("d")

func getValueTypesToTest(t *ldtest.T) []servicedef.ValueType {
	// For strongly-typed SDKs, make sure we test each of the typed Variation methods to prove
	// that they all correctly copy the flag value and default value into the event data. For
	// weakly-typed SDKs, we can just use the universal Variation method.
	var ret []servicedef.ValueType
	if t.Capabilities().Has("strongly-typed") {
		ret = append(ret,
			servicedef.ValueTypeBool,
			servicedef.ValueTypeInt,
			servicedef.ValueTypeDouble,
			servicedef.ValueTypeString,
		)
	}
	return append(ret, servicedef.ValueTypeAny)
}

func inferDefaultFromFlag(sdkData mockld.ServerSDKData, flagKey string) ldvalue.Value {
	flagData := sdkData["flags"][flagKey]
	if flagData == nil {
		return ldvalue.Null()
	}
	var flag ldmodel.FeatureFlag
	if err := json.Unmarshal(flagData, &flag); err != nil {
		return ldvalue.Null() // we should deal with malformed flag data at an earlier point
	}
	if len(flag.Variations) == 0 {
		return ldvalue.Null()
	}
	switch flag.Variations[0].Type() {
	case ldvalue.BoolType:
		return ldvalue.Bool(false)
	case ldvalue.NumberType:
		return ldvalue.Int(0)
	case ldvalue.StringType:
		return ldvalue.String("")
	default:
		return ldvalue.Null()
	}
}

func testDescFromType(valueType servicedef.ValueType) string {
	return fmt.Sprintf("type: %s", valueType)
}

func testDescWithOrWithoutReason(withReason bool) string {
	if withReason {
		return "with reason"
	}
	return "without reason"
}
