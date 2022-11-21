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

	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldmodel"

	"github.com/stretchr/testify/require"
)

var dummyValue0, dummyValue1, dummyValue2, dummyValue3 ldvalue.Value = ldvalue.String("a"), //nolint:gochecknoglobals
	ldvalue.String("b"), ldvalue.String("c"), ldvalue.String("d")

// Helper for constructing the parameters for an evaluation request and returning just the value field.
func basicEvaluateFlag(
	t *ldtest.T,
	client *SDKClient,
	flagKey string,
	context ldcontext.Context,
	defaultValue ldvalue.Value,
) ldvalue.Value {
	result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flagKey,
		Context:      o.Some(context),
		ValueType:    servicedef.ValueTypeAny,
		DefaultValue: defaultValue,
	})
	return result.Value
}

func basicEvaluateFlagWithOldUser(
	t *ldtest.T,
	client *SDKClient,
	flagKey string,
	user json.RawMessage,
	defaultValue ldvalue.Value,
) ldvalue.Value {
	result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flagKey,
		User:         user,
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
	seed o.Maybe[int],
) int {
	hashInput := ""

	if seed.IsDefined() {
		hashInput += strconv.Itoa(seed.Value())
	} else {
		hashInput += flagOrSegmentKey + "." + salt
	}
	hashInput += "." + userValue

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

func contextWithTransformedKeys(context ldcontext.Context, keyFn func(string) string) ldcontext.Context {
	if context.Multiple() {
		b := ldcontext.NewMultiBuilder()
		for _, c := range context.GetAllIndividualContexts(nil) {
			b.Add(contextWithTransformedKeys(c, keyFn))
		}
		return b.Build()
	}
	return ldcontext.NewBuilderFromContext(context).Key(keyFn(context.Key())).Build()
}

func evaluateFlagDetail(
	t *ldtest.T,
	client *SDKClient,
	flagKey string,
	context ldcontext.Context,
	defaultValue ldvalue.Value,
) servicedef.EvaluateFlagResponse {
	return client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flagKey,
		Context:      o.Some(context),
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

// Generates a list of characters that are not in the specified string, including some control characters
// and some multi-byte characters.
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

// Returns a clause that will match any context of any kind.
func makeClauseThatAlwaysMatches() ldmodel.Clause {
	return ldbuilders.Negate(ldbuilders.Clause(ldattr.KindAttr, ldmodel.OperatorIn, ldvalue.String("")))
}

// Returns a flag that evaluates to one of two values depending on whether the context matches the segment.
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

// Builds two versions of the same flag, ensuring that each returns the specified value.
func makeFlagVersionsWithValues(key string, version1, version2 int, value1, value2 ldvalue.Value) (
	ldmodel.FeatureFlag, ldmodel.FeatureFlag) {
	flag1 := ldbuilders.NewFlagBuilder(key).Version(version1).
		On(false).OffVariation(0).Variations(value1, value2).Build()
	flag2 := ldbuilders.NewFlagBuilder(key).Version(version2).
		On(false).OffVariation(1).Variations(value1, value2).Build()
	return flag1, flag2
}

// Polls the client once to see whether a flag's value has changed. Causes the test to fail if the
// result value is neither the expected new value nor the expected old value.
func checkForUpdatedValue(
	t *ldtest.T,
	client *SDKClient,
	flagKey string,
	context ldcontext.Context,
	previousValue ldvalue.Value,
	updatedValue ldvalue.Value,
	defaultValue ldvalue.Value,
) func() bool {
	return func() bool {
		actualValue := basicEvaluateFlag(t, client, flagKey, context, defaultValue)
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

// Polls the client repeatedly until the flag's value has changed. Causes the test to fail if the
// timeout elapses without a change, or if an unexpected value is returned.
func pollUntilFlagValueUpdated(
	t *ldtest.T,
	client *SDKClient,
	flagKey string,
	context ldcontext.Context,
	previousValue ldvalue.Value,
	updatedValue ldvalue.Value,
	defaultValue ldvalue.Value,
) {
	h.RequireEventually(
		t,
		checkForUpdatedValue(t, client, flagKey, context, previousValue, updatedValue, defaultValue),
		time.Second, time.Millisecond*50, "timed out without seeing updated flag value")
}

// Attempts to build an old-style user JSON representation that is equivalent to the given context.
// Returns the JSON data, or nil if there is no equivalent to this context in the old user model.
func representContextAsOldUser(t *ldtest.T, c ldcontext.Context) json.RawMessage {
	if !t.Capabilities().Has(servicedef.CapabilityUserType) {
		return nil
	}
	if c.Kind() != ldcontext.DefaultKind {
		return nil
	}
	o := ldvalue.ObjectBuild().SetString("key", c.Key())
	if c.Anonymous() {
		o.SetBool("anonymous", true)
	}
	custom := ldvalue.ObjectBuild()
	for _, a := range c.GetOptionalAttributeNames(nil) {
		value := c.GetValue(a)
		switch a {
		case "name", "firstName", "lastName", "email", "avatar", "country", "ip":
			if !value.IsString() {
				return nil // not a valid user - these built-in attrs must be strings
			}
			o.Set(a, value)
		default:
			custom.Set(a, value)
		}
	}
	if custom.Build().Count() != 0 {
		o.Set("custom", custom.Build())
	}
	if c.PrivateAttributeCount() != 0 {
		pas := ldvalue.ArrayBuild()
		for i := 0; i < c.PrivateAttributeCount(); i++ {
			if pa, ok := c.PrivateAttributeByIndex(i); ok {
				if pa.Depth() != 1 {
					return nil // not a valid user - users don't support attribute references
				}
				pas.Add(ldvalue.String(pa.Component(0)))
			}
		}
		o.Set("privateAttributeNames", pas.Build())
	}
	return json.RawMessage(o.Build().JSONString())
}

// Configures a (single-kind) context to have the specified value for a particular attribute-- or, if the
// ldattr.Ref is a complex reference, a particular object property or array element.
func setContextValueForAttrRef(b *ldcontext.Builder, ref ldattr.Ref, value ldvalue.Value) {
	for depth := ref.Depth() - 1; depth > 0; depth-- {
		name := ref.Component(depth)
		objectBuilder := ldvalue.ObjectBuild()
		objectBuilder.Set(name, value)
		value = objectBuilder.Build()
	}
	name := ref.Component(0)
	b.SetValue(name, value)
}

// Shortcut for creating a sorted copy of a string list.
func sortedStrings(ss []string) []string {
	ret := append([]string(nil), ss...)
	sort.Strings(ret)
	return ret
}

func testDescFromType(valueType servicedef.ValueType) string {
	return fmt.Sprintf("type: %s", valueType)
}
