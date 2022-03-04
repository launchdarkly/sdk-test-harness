package sdktests

import (
	"fmt"
	"sort"
	"strings"

	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

// The functions in this file are for convenient use of the matchers API with complex
// types. For more information, see matchers.Transform.

func EvalResponseValue() m.MatcherTransform {
	return m.Transform(
		"result value",
		func(value interface{}) (interface{}, error) {
			r := value.(servicedef.EvaluateFlagResponse)
			return r.Value, nil
		}).
		EnsureInputValueType(servicedef.EvaluateFlagResponse{})
}

func EvalResponseVariation() m.MatcherTransform {
	return m.Transform(
		"result variation index",
		func(value interface{}) (interface{}, error) {
			r := value.(servicedef.EvaluateFlagResponse)
			return ldvalue.NewOptionalIntFromPointer(r.VariationIndex), nil
		}).
		EnsureInputValueType(servicedef.EvaluateFlagResponse{})
}

func EvalResponseReason() m.MatcherTransform {
	return m.Transform(
		"result reason",
		func(value interface{}) (interface{}, error) {
			r := value.(servicedef.EvaluateFlagResponse)
			if r.Reason == nil {
				return nil, nil
			}
			return *r.Reason, nil
		}).
		EnsureInputValueType(servicedef.EvaluateFlagResponse{})
}

func EvalAllFlagsStateMap() m.MatcherTransform {
	return m.Transform(
		"result reason",
		func(value interface{}) (interface{}, error) {
			return value.(servicedef.EvaluateAllFlagsResponse).State, nil
		}).
		EnsureInputValueType(servicedef.EvaluateAllFlagsResponse{})
}

func EvalAllFlagsValueForKeyShouldEqual(key string, value ldvalue.Value) m.Matcher {
	return EvalAllFlagsStateMap().Should(m.ValueForKey(key).Should(m.JSONEqual(value)))
}

func JSONPropertyKeysCanOnlyBe(keys ...string) m.Matcher {
	jsonKeys := func(value interface{}) []string {
		return ldvalue.Parse(jsonhelpers.ToJSON(value)).Keys()
	}
	return m.New(
		func(value interface{}) bool {
			for _, key := range jsonKeys(value) {
				if !stringInSlice(key, keys) {
					return false
				}
			}
			return true
		},
		func() string {
			return fmt.Sprintf("JSON property keys can only be [%s]", strings.Join(sortedStrings(keys), ", "))
		},
		func(value interface{}) string {
			var badKeys []string
			for _, key := range jsonKeys(value) {
				if !stringInSlice(key, keys) {
					badKeys = append(badKeys, key)
				}
			}
			return fmt.Sprintf("Unexpected JSON property key(s) [%s]; allowed keys are [%s]",
				strings.Join(sortedStrings(badKeys), ", "),
				strings.Join(sortedStrings(keys), ", "))
		},
	)
}

func JSONPropertyNullOrAbsent(name string) m.Matcher {
	return m.JSONOptProperty(name).Should(m.BeNil())
}

func SortedStrings() m.MatcherTransform {
	return m.Transform("in order",
		func(value interface{}) (interface{}, error) {
			a := ldvalue.Parse(jsonhelpers.ToJSON(value))
			if a.IsNull() {
				return nil, nil
			}
			if a.Type() != ldvalue.ArrayType {
				return nil, fmt.Errorf("expected strings but got %T", value)
			}
			ret := make([]string, 0, a.Count())
			for _, v := range a.AsValueArray().AsSlice() {
				if !v.IsString() {
					return nil, fmt.Errorf("expected strings but got %+v", value)
				}
				ret = append(ret, v.StringValue())
			}
			sort.Strings(ret)
			return ret, nil
		})
}

func ValueIsPositiveNonZeroInteger() m.Matcher {
	return m.New(
		func(value interface{}) bool {
			v := ldvalue.Parse(jsonhelpers.ToJSON(value))
			return v.IsInt() && v.IntValue() > 0
		},
		func() string {
			return "is an int > 0"
		},
		func(value interface{}) string {
			return "was not an int or was negative"
		},
	)
}
