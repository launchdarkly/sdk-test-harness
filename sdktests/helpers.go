package sdktests

import (
	"crypto/sha1" //nolint:gosec
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
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
		User:         o.Some(user),
		ValueType:    servicedef.ValueTypeAny,
		DefaultValue: defaultValue,
	})
	return result.Value
}

// computeExpectedBucketValue implements the bucketing hash value calculation as per the evaluation spec,
// except that it returns the value as an integer in the range [0, 100000] - currently the SDKs convert
// this to a floating-point fraction by in effect dividing it by 100000, but this test code needs an
// integer in order to compute bucket weights.
func computeExpectedBucketValue(
	userValue string,
	flagOrSegmentKey, salt string,
	secondary o.Maybe[string],
	seed o.Maybe[int],
) int {
	hashInput := ""

	if seed.IsDefined() {
		hashInput += strconv.Itoa(seed.Value())
	} else {
		hashInput += flagOrSegmentKey + "." + salt
	}
	hashInput += "." + userValue
	if secondary.IsDefined() {
		hashInput += "." + secondary.Value()
	}

	hashOutputBytes := sha1.Sum([]byte(hashInput)) //nolint:gosec // this isn't for authentication
	hexEncodedChars := make([]byte, 64)
	hex.Encode(hexEncodedChars, hashOutputBytes[:])
	hash := hexEncodedChars[:15]

	hashVal, _ := strconv.ParseInt(string(hash), 16, 64)
	var product, result big.Int
	product.Mul(big.NewInt(hashVal), big.NewInt(100000))
	result.Div(&product, big.NewInt(0xFFFFFFFFFFFFFFF))
	return int(result.Int64())
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
		User:         o.Some(user),
		ValueType:    servicedef.ValueTypeAny,
		DefaultValue: defaultValue,
		Detail:       true,
	})
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

func inferDefaultFromFlag(sdkData mockld.SDKData, flagKey string) ldvalue.Value {
	var flagValue ldvalue.Value
	var flagExists bool

	switch data := sdkData.(type) {
	case mockld.ClientSDKData:
		if flagData, ok := data[flagKey]; ok {
			flagExists = true
			flagValue = flagData.Value
		}
	case mockld.ServerSDKData:
		if flagData, ok := data["flags"][flagKey]; ok {
			var flag ldmodel.FeatureFlag
			if err := json.Unmarshal(flagData, &flag); err == nil {
				if len(flag.Variations) != 0 {
					flagValue = flag.Variations[0]
					flagExists = true
				}
			}
		}
	}
	if !flagExists {
		return ldvalue.Null()
	}
	switch flagValue.Type() {
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

func makeCharactersNotInAllowedCharsetString(allowed string) []rune {
	var badChars []rune
	badChars = append(badChars, '\t', '\n', '\r') // don't bother including every control character
	for ch := 32; ch <= 127; ch++ {
		if strings.ContainsRune(allowed, rune(ch)) {
			continue
		}
		badChars = append(badChars, rune(ch))
	}
	// Don't try to cover the whole Unicode space, just pick a couple of multi-byte characters
	badChars = append(badChars, 'é', '😀')
	return badChars
}

// Returns a clause that will match any user.
func makeClauseThatAlwaysMatches() ldmodel.Clause {
	return ldbuilders.Negate(ldbuilders.Clause("key", ldmodel.OperatorIn, ldvalue.String("")))
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

func optionalIntFrom(m o.Maybe[int]) ldvalue.OptionalInt {
	if m.IsDefined() {
		return ldvalue.NewOptionalInt(m.Value())
	}
	return ldvalue.OptionalInt{}
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
	helpers.RequireEventually(
		t,
		checkForUpdatedValue(t, client, flagKey, user, previousValue, updatedValue, defaultValue),
		time.Second, time.Millisecond*50, "timed out without seeing updated flag value")
}

func sortedStrings(ss []string) []string {
	ret := append([]string(nil), ss...)
	sort.Strings(ret)
	return ret
}

func testDescFromType(valueType servicedef.ValueType) string {
	return fmt.Sprintf("type: %s", valueType)
}
