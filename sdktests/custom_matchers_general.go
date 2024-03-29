package sdktests

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// The functions in this file are for convenient use of the matchers API with complex
// types. For more information, see matchers.Transform.

// UniqueQueryParameters returns a MatcherTransform which parses a string representing a URL's
// RawQuery field into a map from parameter key to parameter value. If there are multiple values
// for a key, an error is returned.
func UniqueQueryParameters() m.MatcherTransform {
	return m.Transform("extract URL query parameter", func(i interface{}) (interface{}, error) {
		values, err := url.ParseQuery(i.(string))
		if err != nil {
			return nil, err
		}
		out := make(map[string]string)
		for k, v := range values {
			if len(v) > 1 {
				return nil, fmt.Errorf("parameter %s had %v values; expected 1", k, len(v))
			}
			out[k] = v[0]
		}
		return out, nil
	}).
		EnsureInputValueType("")
}
func Base64DecodedData() m.MatcherTransform {
	return m.Transform(
		"base64-decoded data",
		func(value interface{}) (interface{}, error) {
			data := value.(string)
			// Some of our SDKs use base64 with padding, others omit the padding; LD accepts both.
			// First try decoding without padding.
			decoded, err := base64.RawURLEncoding.DecodeString(data)
			if err != nil {
				// Try decoding with padding.
				decoded, err = base64.URLEncoding.DecodeString(data)
				if err == nil {
					return decoded, nil
				}
				return nil, fmt.Errorf("not a valid base64-encoded string (%w)", err)
			}
			return decoded, nil
		}).
		EnsureInputValueType("")
}

// EqualReason is a type-safe replacement for m.Equal(EvaluationReason) that also provides better
// failure output, by treating it as a JSON object-- since the default implementation of String()
// for EvaluationReason returns a shorter string that doesn't include every property.
func EqualReason(reason ldreason.EvaluationReason) m.Matcher {
	return m.JSONEqual(reason)
}

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
			return r.VariationIndex, nil
		}).
		EnsureInputValueType(servicedef.EvaluateFlagResponse{})
}

func EvalResponseReason() m.MatcherTransform {
	return m.Transform(
		"result reason",
		func(value interface{}) (interface{}, error) {
			r := value.(servicedef.EvaluateFlagResponse)
			if r.Reason.IsDefined() {
				return o.Some(r.Reason), nil
			}
			return o.None[ldreason.EvaluationReason](), nil
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

// HasAuthorizationHeader is a matcher for an http.Header map that verifies that the Authorization
// header is present and contains the specified key. Some SDKs send just the raw key, while others
// prefix it with an "api_key" scheme identifier; the latter is more technically correct, but we
// need to allow both since LD allows both.
func HasAuthorizationHeader(authKey string) m.Matcher {
	return Header("Authorization").Should(
		m.AnyOf(
			m.Equal(authKey),
			m.Equal("api_key "+authKey),
		))
}

func HasNoAuthorizationHeader() m.Matcher {
	return Header("Authorization").Should(m.Equal(""))
}

// Header allows matchers to be applied to a specific named header from an http.Header map. It
// assumes that there is just one value for that name (i.e. it calls Header.Get()).
func Header(name string) m.MatcherTransform {
	return m.Transform(
		fmt.Sprintf("header %q", name),
		func(value interface{}) (interface{}, error) {
			return value.(http.Header).Get(name), nil
		}).
		EnsureInputValueType(http.Header{})
}

func JSONPropertyKeysCanOnlyBe(keys ...string) m.Matcher {
	jsonKeys := func(value interface{}) []string {
		return ldvalue.Parse(jsonhelpers.ToJSON(value)).Keys(nil)
	}
	return m.New(
		func(value interface{}) bool {
			for _, key := range jsonKeys(value) {
				if !h.SliceContains(key, keys) {
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
				if !h.SliceContains(key, keys) {
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

func JSONPropertyNullOrAbsentOrEqualTo(name string, emptyValue interface{}) m.Matcher {
	return m.JSONOptProperty(name).Should(m.AnyOf(m.BeNil(), m.JSONEqual(emptyValue)))
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
