package sdktests

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"github.com/stretchr/testify/require"
)

var dummyValue0, dummyValue1, dummyValue2, dummyValue3 ldvalue.Value = ldvalue.String("a"), //nolint:gochecknoglobals
	ldvalue.String("b"), ldvalue.String("c"), ldvalue.String("d")

func basicEvaluateFlag(
	t *ldtest.T,
	client *SDKClient,
	flagKey string,
	user lduser.User,
	defaultValue ldvalue.Value,
) ldvalue.Value {
	result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flagKey,
		User:         &user,
		ValueType:    servicedef.ValueTypeAny,
		DefaultValue: defaultValue,
	})
	return result.Value
}

func evaluateFlagDetail(
	t *ldtest.T,
	client *SDKClient,
	flagKey string,
	user lduser.User,
	defaultValue ldvalue.Value,
) servicedef.EvaluateFlagResponse {
	return client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flagKey,
		User:         &user,
		ValueType:    servicedef.ValueTypeAny,
		DefaultValue: defaultValue,
		Detail:       true,
	})
}

func expectNoMoreRequests(t *ldtest.T, endpoint *harness.MockEndpoint) {
	_, err := endpoint.AwaitConnection(time.Millisecond * 100)
	require.Error(t, err, "did not expect another request, but got one")
}

func expectRequest(t *ldtest.T, endpoint *harness.MockEndpoint, timeout time.Duration) harness.IncomingRequestInfo {
	request, err := endpoint.AwaitConnection(timeout)
	require.NoError(t, err, "timed out waiting for request")
	return request
}

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

func makeFlagToCheckSegmentMatch(
	flagKey string,
	segmentKey string,
	valueIfNotIncluded, valueIfIncluded ldvalue.Value,
) ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder(flagKey).Version(1).
		On(true).FallthroughVariation(0).Variations(valueIfNotIncluded, valueIfIncluded).
		AddRule(ldbuilders.NewRuleBuilder().ID("ruleid").Variation(1).Clauses(
			ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String(segmentKey)),
		)).
		Build()
}

func makeFlagVersionsWithValues(key string, version1, version2 int, value1, value2 ldvalue.Value) (
	ldmodel.FeatureFlag, ldmodel.FeatureFlag) {
	flag1 := ldbuilders.NewFlagBuilder(key).Version(version1).
		On(false).OffVariation(0).Variations(value1, value2).Build()
	flag2 := ldbuilders.NewFlagBuilder(key).Version(version2).
		On(false).OffVariation(1).Variations(value1, value2).Build()
	return flag1, flag2
}

func checkForUpdatedValue(
	t *ldtest.T,
	client *SDKClient,
	flagKey string,
	user lduser.User,
	previousValue ldvalue.Value,
	updatedValue ldvalue.Value,
	defaultValue ldvalue.Value,
) func() bool {
	return func() bool {
		actualValue := basicEvaluateFlag(t, client, flagKey, user, defaultValue)
		if actualValue.Equal(updatedValue) {
			return true
		}
		if !actualValue.Equal(previousValue) {
			require.Fail(t, "SDK returned neither previous value nor updated value",
				"previous: %s, updated: %s, actual: %s", previousValue, updatedValue, actualValue)
		}
		return false
	}
}

func pollUntilFlagValueUpdated(
	t *ldtest.T,
	client *SDKClient,
	flagKey string,
	user lduser.User,
	previousValue ldvalue.Value,
	updatedValue ldvalue.Value,
	defaultValue ldvalue.Value,
) {
	// We can't assume that the SDK will immediately apply the new flag data as soon as it has
	// reconnected, so we have to poll till the new data shows up
	require.Eventually(
		t,
		checkForUpdatedValue(t, client, flagKey, user, previousValue, updatedValue, defaultValue),
		time.Second, time.Millisecond*50, "timed out without seeing updated flag value")
}

func timeValueAsPointer(value ldtime.UnixMillisecondTime) *ldtime.UnixMillisecondTime {
	return &value
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
