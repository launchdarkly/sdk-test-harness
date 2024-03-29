package sdktests

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doSDKContextTypeTests(t *ldtest.T) {
	if t.Capabilities().Has(servicedef.CapabilityContextType) {
		t.Run("build", doSDKContextBuildTests)
		t.Run("convert", doSDKContextConvertTests)
	}

	if t.Capabilities().Has(servicedef.CapabilityContextComparison) {
		t.Run("compare", doSDKContextComparisonTests)
	}
}

// Note: even though these tests don't involve an SDK client instance actually doing anything, so neither
// the data source nor the client itself are really involved-- because it's just the SDK library manipulating
// a context object-- the current test harness architecture requires all test service commands to be directed
// at a client instance.

func doSDKContextBuildTests(t *ldtest.T) {
	dataSource := NewSDKDataSource(t, nil)
	client := NewSDKClient(t, dataSource)

	optStr := func(s string) *string { return &s }
	optBool := func(b bool) *bool { return &b }

	t.Run("valid", func(t *ldtest.T) {
		type singleKindTestCase struct {
			params   servicedef.ContextBuildSingleParams
			expected string
		}
		singleKindTestCases := []singleKindTestCase{
			{servicedef.ContextBuildSingleParams{Key: "a"}, `{"kind": "user", "key": "a"}`},
			{servicedef.ContextBuildSingleParams{Kind: optStr("org"), Key: "a"},
				`{"kind": "org", "key": "a"}`},
			{servicedef.ContextBuildSingleParams{Key: "a", Name: optStr("b")},
				`{"kind": "user", "key": "a", "name": "b"}`},
			{servicedef.ContextBuildSingleParams{Key: "a", Anonymous: optBool(false)},
				`{"kind": "user", "key": "a"}`},
			{servicedef.ContextBuildSingleParams{Key: "a", Anonymous: optBool(true)},
				`{"kind": "user", "key": "a", "anonymous": true}`},
			{servicedef.ContextBuildSingleParams{Key: "a", Private: []string{"b"}},
				`{"kind": "user", "key": "a", "_meta": {"privateAttributes": ["b"]}}`},
			{servicedef.ContextBuildSingleParams{Key: "a", Custom: map[string]ldvalue.Value{"attr": ldvalue.Null()}},
				`{"kind": "user", "key": "a"}`},
		}
		for _, v := range data.MakeStandardTestValues() {
			if v.IsNull() {
				continue
			}
			if v.Type() == ldvalue.ObjectType && v.Count() == 0 && t.Capabilities().Has(servicedef.CapabilityPHP) {
				// This is a special case where we're skipping "set an attribute to an empty JSON object {}" in
				// the PHP SDK only. The reason is that due to PHP's idiosyncratic implementation of associative
				// arrays, it is hard to accurately represent the empty JSON object value {} within the PHP SDK.
				continue
			}
			singleKindTestCases = append(singleKindTestCases, singleKindTestCase{
				servicedef.ContextBuildSingleParams{Key: "a", Custom: map[string]ldvalue.Value{"attr": v}},
				fmt.Sprintf(`{"kind": "user", "key": "a", "attr": %s}`, v.JSONString()),
			})
		}
		multiKindTestCases := []struct {
			kinds    []servicedef.ContextBuildSingleParams
			expected string
		}{
			{
				[]servicedef.ContextBuildSingleParams{{Kind: optStr("org"), Key: "a"}, {Key: "b"}},
				`{"kind": "multi", "org": {"key": "a"}, "user": {"key": "b"}}`,
			},
			{
				// multi-kind with only one kind becomes single-kind
				[]servicedef.ContextBuildSingleParams{{Kind: optStr("org"), Key: "a"}},
				`{"kind": "org", "key": "a"}`,
			},
		}

		type testCase struct {
			params   servicedef.ContextBuildParams
			expected string
		}
		var testCases []testCase
		for _, c := range singleKindTestCases {
			params := c.params
			testCases = append(testCases, testCase{servicedef.ContextBuildParams{Single: &params}, c.expected})
		}
		for _, c := range multiKindTestCases {
			testCases = append(testCases, testCase{servicedef.ContextBuildParams{Multi: c.kinds}, c.expected})
		}

		for _, p := range testCases {
			t.Run(jsonhelpers.ToJSONString(p.params), func(t *ldtest.T) {
				resp := client.ContextBuild(t, p.params)
				require.Equal(t, "", resp.Error)
				m.In(t).Assert(json.RawMessage(resp.Output), m.JSONEqual(json.RawMessage(p.expected)))
			})
		}
	})
}

func doSDKContextConvertTests(t *ldtest.T) {
	dataSource := NewSDKDataSource(t, nil)
	client := NewSDKClient(t, dataSource)

	basicInputPlusProps := func(extraProps string) string {
		if extraProps != "" {
			extraProps = ", " + extraProps
		}
		return fmt.Sprintf(`{"kind": "org", "key": "x"%s}`, extraProps)
	}

	t.Run("valid, no changes", func(t *ldtest.T) {
		var inputs []string

		singleKindProps := []string{
			``,
			`"name": "b"`,
			`"anonymous": true`,
			`"attr1": "first"`,                      // basic case of 1 custom attr
			`"attr1": "first", "attr2": "second"`,   // basic case of multiple custom attrs
			`"_meta": {"privateAttributes": ["x"]}`, // basic case of 1 private attr
			`"_meta": {"privateAttributes": ["x", "y"]}`, // basic case of multiple private attrs
		}
		for _, v := range data.MakeStandardTestValues() {
			// verify that custom attributes of various types and values are allowed - not counting null,
			// which is covered under "unnecessary properties" below
			if v.IsNull() {
				continue
			}
			singleKindProps = append(singleKindProps, fmt.Sprintf(`"attr1": %s`, v.JSONString()))
		}

		for _, extraProps := range singleKindProps {
			inputs = append(inputs, basicInputPlusProps(extraProps))
		}

		inputs = append(inputs,
			// basic multi-kind context
			`{"kind": "multi", "org": {"key": "org-key", "name": "org-name"}, "user": {"key": "user-key", "name": "user-name"}}`,

			// verify that all allowable characters in kind are accepted
			`{"kind": "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ013456789-_.", "key": "x"}`,
			`{"kind": "multi", "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ013456789-_.": {"key": "x"},`+
				` "other": {"key": "y"}}`,
		)

		for _, input := range inputs {
			t.Run(input, func(t *ldtest.T) {
				resp := client.ContextConvert(t, servicedef.ContextConvertParams{Input: input})
				require.Equal(t, "", resp.Error)
				m.In(t).Assert(json.RawMessage(resp.Output), m.JSONEqual(json.RawMessage(input)))
			})
		}
	})

	t.Run("unnecessary properties are dropped", func(t *ldtest.T) {
		expected := json.RawMessage(basicInputPlusProps(""))

		for _, extraProps := range []string{
			`"name": null`,
			`"attr1": null`,
			`"_meta": null`,
			`"_meta": {}`,
			`"_meta": {"privateAttributes": null}`,
			`"_meta": {"privateAttributes": []}`,
		} {
			input := basicInputPlusProps(extraProps)
			t.Run(input, func(t *ldtest.T) {
				resp := client.ContextConvert(t, servicedef.ContextConvertParams{Input: input})
				require.Equal(t, "", resp.Error)
				m.In(t).Assert(json.RawMessage(resp.Output), m.JSONEqual(expected))
			})
		}
	})

	t.Run("old user to context", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityUserType)
		type contextConversionParams struct {
			in, out string // out only needs to be set if it's different from in
		}

		params := []contextConversionParams{
			{`{"key": ""}`, `{"kind": "user", "key": ""}`}, // empty key *is* allowed for old user format only
			{`{"key": "a"}`, `{"kind": "user", "key": "a"}`},
			{`{"key": "a"}`, `{"kind": "user", "key": "a"}`},
			{`{"key": "a", "custom": {"b": true}}`, `{"kind": "user", "key": "a", "b": true}`},
			{`{"key": "a", "custom": {"b": 1}}`, `{"kind": "user", "key": "a", "b": 1}`},
			{`{"key": "a", "custom": {"b": "c"}}`, `{"kind": "user", "key": "a", "b": "c"}`},
			{`{"key": "a", "custom": {"b": [1, 2]}}`, `{"kind": "user", "key": "a", "b": [1, 2]}`},
			{`{"key": "a", "custom": {"b": {"c": 1}}}`, `{"kind": "user", "key": "a", "b": {"c": 1}}`},
			{`{"key": "a", "custom": {"b": 1, "c": 2}}`, `{"kind": "user", "key": "a", "b": 1, "c": 2}`},
			{`{"key": "a", "custom": {"b": 1, "c": null}}`, `{"kind": "user", "key": "a", "b": 1}`},
			{`{"key": "a", "custom": {}}`, `{"kind": "user", "key": "a"}`},
			{`{"key": "a", "custom": null}`, `{"kind": "user", "key": "a"}`},
			{`{"key": "a", "anonymous": true}`, `{"kind": "user", "key": "a", "anonymous": true}`},
			{`{"key": "a", "anonymous": false}`, `{"kind": "user", "key": "a"}`},
			{`{"key": "a", "anonymous": null}`, `{"kind": "user", "key": "a"}`},
			{`{"key": "a", "privateAttributeNames": ["b"]}`,
				`{"kind": "user", "key": "a", "_meta": {"privateAttributes": ["b"]}}`},
			{`{"key": "a", "privateAttributeNames": []}`, `{"kind": "user", "key": "a"}`},
			{`{"key": "a", "privateAttributeNames": null}`, `{"kind": "user", "key": "a"}`},
		}
		for _, stringAttrName := range []string{"name", "firstName", "lastName", "email", "country", "avatar", "ip"} {
			params = append(params,
				contextConversionParams{
					in:  fmt.Sprintf(`{"key": "a", "%s": "b"}`, stringAttrName),
					out: fmt.Sprintf(`{"kind": "user", "key": "a", "%s": "b"}`, stringAttrName),
				},
				contextConversionParams{
					in:  fmt.Sprintf(`{"key": "a", "%s": null}`, stringAttrName),
					out: `{"kind": "user", "key": "a"}`,
				})
		}
		for _, p := range params {
			t.Run(p.in, func(t *ldtest.T) {
				resp := client.ContextConvert(t, servicedef.ContextConvertParams{Input: p.in})
				require.Equal(t, "", resp.Error)
				m.In(t).Assert(json.RawMessage(resp.Output), m.JSONEqual(json.RawMessage(p.out)))
			})
		}
	})

	t.Run("multi-kind with only one kind becomes single-kind", func(t *ldtest.T) {
		singleKindJSON := `{"kind": "org", "key": "a", "name": "b"}`
		multiKindJSON := `{"kind": "multi", "org": {"key": "a", "name": "b"}}`
		resp := client.ContextConvert(t, servicedef.ContextConvertParams{Input: multiKindJSON})
		require.Equal(t, "", resp.Error)
		m.In(t).Assert(json.RawMessage(resp.Output), m.JSONEqual(json.RawMessage(singleKindJSON)))
	})

	t.Run("invalid context", func(t *ldtest.T) {
		inputs := []string{
			``,
			`{`,    // malformed JSON
			`true`, // not an object

			// wrong type for built-in property
			`{"kind": null, "key": "x"}`,
			`{"kind": true, "key": "x"}`,
			`{"kind": "org", "key": null}`,
			`{"kind": "org", "key": 3}`,
			`{"kind": "org", "name": 3}`,
			`{"kind": "org", "anonymous": null}`,
			`{"kind": "org", "anonymous": "yes"}`,
			`{"kind": "org", "_meta": {"privateAttributes": 3}}`,
			`{"kind": "org", "_meta": {"privateAttributes": {}}}`,

			`{"kind": "kind", "key": "x"}`, // kind cannot be "kind"
			`{"kind": "multi"}`,            // multi-kind with no kinds
			`{"kind": "", "key" : "x"}`,    // kind cannot be empty string

		}
		for _, input := range inputs {
			t.Run(input, func(t *ldtest.T) {
				resp := client.ContextConvert(t, servicedef.ContextConvertParams{Input: input})
				assert.NotEqual(t, "", resp.Error)
			})
		}
		t.Run("bad characters in kind", func(t *ldtest.T) {
			// Do this as just one subtest so that the test log isn't incredibly long. We won't cover the
			// whole Unicode space of course, just enough to verify that non-ASCII and multi-byte characters
			// are disallowed, as well as ASCII characters that are outside of the allowed set.
			for ch := 1; ch < 256; ch++ {
				if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') ||
					ch == '.' || ch == '-' || ch == '_' {
					continue
				}
				kind := "org" + string(rune(ch))
				// use real JSON encoding for the string in case escaping is needed
				encoded, _ := json.Marshal(kind)

				input1 := fmt.Sprintf(`{"kind": %s, "key": "x"}`, string(encoded))
				resp1 := client.ContextConvert(t, servicedef.ContextConvertParams{Input: input1})
				if resp1.Error == "" {
					t.Errorf("invalid character was allowed for single-kind (character code %d)", ch)
				}
				input2 := fmt.Sprintf(`{"kind": "multi", %s: {"key": "x"}}`, string(encoded))
				resp2 := client.ContextConvert(t, servicedef.ContextConvertParams{Input: input2})
				if resp2.Error == "" {
					t.Errorf("invalid character was allowed for multi-kind (character code %d)", ch)
				}
			}
		})
	})

	t.Run("invalid old user", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityUserType)
		inputs := []string{
			`{}`,
			`{"key": true}`,
			`{"key": "a", "custom": 3}`,
			`{"key": "a", "anonymous": 3}`,
			`{"key": "a", "privateAttributeNames": 3"}`,
		}
		for _, stringAttrName := range []string{"name", "firstName", "lastName", "email", "country", "avatar", "ip"} {
			inputs = append(inputs, fmt.Sprintf(`{"key": "a", "%s": 3}`, stringAttrName))
		}
		for _, input := range inputs {
			t.Run(input, func(t *ldtest.T) {
				resp := client.ContextConvert(t, servicedef.ContextConvertParams{Input: input})
				assert.NotEqual(t, "", resp.Error)
			})
		}
	})
}

func doSDKContextComparisonTests(t *ldtest.T) {
	dataSource := NewSDKDataSource(t, nil)
	client := NewSDKClient(t, dataSource)
	address := ldvalue.ObjectBuild().SetString("street", "123 Easy St").SetString("city", "Anytown").Build()
	privateAttributes := []servicedef.PrivateAttribute{
		{Value: "/address/street", Literal: false},
		{Value: "name", Literal: false},
	}

	t.Run("single contexts", func(t *ldtest.T) {
		t.Run("are equal when identical", func(t *ldtest.T) {
			attributes := []servicedef.AttributeDefinition{
				{Name: "name", Value: ldvalue.String("Example name")},
				{Name: "address", Value: address},
			}
			param := servicedef.ContextComparisonPairParams{
				Context1: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key",
						Attributes: attributes, PrivateAttributes: privateAttributes},
				},
				Context2: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key",
						Attributes: attributes, PrivateAttributes: privateAttributes},
				},
			}

			resp := client.ContextComparison(t, param)
			m.In(t).Assert(resp.Equals, m.Equal(true))
		})

		t.Run("are equal when properties are out of order", func(t *ldtest.T) {
			param := servicedef.ContextComparisonPairParams{
				Context1: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key",
						Attributes: []servicedef.AttributeDefinition{
							{Name: "name", Value: ldvalue.String("Example name")},
							{Name: "address", Value: address},
						}},
				},
				Context2: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key",
						Attributes: []servicedef.AttributeDefinition{
							{Name: "address", Value: address},
							{Name: "name", Value: ldvalue.String("Example name")},
						}},
				},
			}

			resp := client.ContextComparison(t, param)
			m.In(t).Assert(resp.Equals, m.Equal(true))
		})

		t.Run("are equal when private attributes are out of order", func(t *ldtest.T) {
			outOfOrderPrivateAttributes := []servicedef.PrivateAttribute{
				{Value: "name", Literal: false},
				{Value: "/address/street", Literal: false},
			}

			param := servicedef.ContextComparisonPairParams{
				Context1: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key",
						PrivateAttributes: privateAttributes},
				},
				Context2: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key",
						PrivateAttributes: outOfOrderPrivateAttributes},
				},
			}

			resp := client.ContextComparison(t, param)
			m.In(t).Assert(resp.Equals, m.Equal(true))
		})

		t.Run("are equal when private attributes are equivalent", func(t *ldtest.T) {
			param := servicedef.ContextComparisonPairParams{
				Context1: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key",
						PrivateAttributes: []servicedef.PrivateAttribute{{Value: "/address/street", Literal: true}}},
				},
				Context2: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key",
						PrivateAttributes: []servicedef.PrivateAttribute{{Value: "/~1address~1street", Literal: false}}},
				},
			}

			resp := client.ContextComparison(t, param)
			m.In(t).Assert(resp.Equals, m.Equal(true))
		})

		t.Run("are different if kinds are different", func(t *ldtest.T) {
			param := servicedef.ContextComparisonPairParams{
				Context1: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "org", Key: "same-key"},
				},
				Context2: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "same-key"},
				},
			}

			resp := client.ContextComparison(t, param)
			m.In(t).Assert(resp.Equals, m.Equal(false))
		})

		t.Run("are different if custom attribute map is not identical", func(t *ldtest.T) {
			customAttributeBuilder := ldvalue.ObjectBuild().SetString("example string", "hi").SetInt("example int", 1)

			param := servicedef.ContextComparisonPairParams{
				Context1: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key",
						Attributes: []servicedef.AttributeDefinition{
							{Name: "custom", Value: customAttributeBuilder.Build()},
						}},
				},
				Context2: servicedef.ContextComparisonParams{
					Single: &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key",
						Attributes: []servicedef.AttributeDefinition{
							{Name: "custom", Value: customAttributeBuilder.SetBool("extra-parameter", true).Build()},
						}},
				},
			}

			resp := client.ContextComparison(t, param)
			m.In(t).Assert(resp.Equals, m.Equal(false))
		})
	})

	t.Run("multi contexts", func(t *ldtest.T) {
		userContext := &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key"}
		orgContext := &servicedef.ContextComparisonSingleParams{Kind: "org", Key: "org-key"}
		deviceContext := &servicedef.ContextComparisonSingleParams{Kind: "device", Key: "device-key"}

		t.Run("are equal when contexts are out of order", func(t *ldtest.T) {
			param := servicedef.ContextComparisonPairParams{
				Context1: servicedef.ContextComparisonParams{
					Multi: []servicedef.ContextComparisonSingleParams{*userContext, *orgContext},
				},
				Context2: servicedef.ContextComparisonParams{
					Multi: []servicedef.ContextComparisonSingleParams{*orgContext, *userContext},
				},
			}

			resp := client.ContextComparison(t, param)
			m.In(t).Assert(resp.Equals, m.Equal(true))
		})

		t.Run("with one kind are equivalent to that single kind context", func(t *ldtest.T) {
			param := servicedef.ContextComparisonPairParams{
				Context1: servicedef.ContextComparisonParams{
					Multi: []servicedef.ContextComparisonSingleParams{*userContext},
				},
				Context2: servicedef.ContextComparisonParams{Single: userContext},
			}

			resp := client.ContextComparison(t, param)
			m.In(t).Assert(resp.Equals, m.Equal(true))
		})

		t.Run("are not equal if one has more contexts", func(t *ldtest.T) {
			param := servicedef.ContextComparisonPairParams{
				Context1: servicedef.ContextComparisonParams{
					Multi: []servicedef.ContextComparisonSingleParams{*userContext, *orgContext},
				},
				Context2: servicedef.ContextComparisonParams{
					Multi: []servicedef.ContextComparisonSingleParams{*userContext, *orgContext, *deviceContext},
				},
			}

			resp := client.ContextComparison(t, param)
			m.In(t).Assert(resp.Equals, m.Equal(false))
		})

		t.Run("are not equal if one is different", func(t *ldtest.T) {
			differentUserContext := &servicedef.ContextComparisonSingleParams{Kind: "user", Key: "user-key2"}
			param := servicedef.ContextComparisonPairParams{
				Context1: servicedef.ContextComparisonParams{
					Multi: []servicedef.ContextComparisonSingleParams{*userContext, *orgContext},
				},
				Context2: servicedef.ContextComparisonParams{
					Multi: []servicedef.ContextComparisonSingleParams{*differentUserContext, *orgContext},
				},
			}

			resp := client.ContextComparison(t, param)
			m.In(t).Assert(resp.Equals, m.Equal(false))
		})
	})
}
