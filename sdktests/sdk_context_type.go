package sdktests

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doSDKContextTypeTests(t *ldtest.T) {
	if !t.Capabilities().Has(servicedef.CapabilityStronglyTyped) {
		t.SkipWithReason("context type tests only apply to strongly-typed SDKs")
	}

	t.Run("build", doSDKContextBuildTests)
	t.Run("convert", doSDKContextConvertTests)
}

// Note: even though these tests don't involve an SDK client instance actually doing anything, so neither
// the data source nor the client itself are really involved-- because it's just the SDK library manipulating
// a context object-- the current test harness architecture requires all test service commands to be directed
// at a client instance.

func doSDKContextBuildTests(t *ldtest.T) {
	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
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
			{servicedef.ContextBuildSingleParams{Key: "a", Transient: optBool(false)},
				`{"kind": "user", "key": "a"}`},
			{servicedef.ContextBuildSingleParams{Key: "a", Transient: optBool(true)},
				`{"kind": "user", "key": "a", "transient": true}`},
			{servicedef.ContextBuildSingleParams{Key: "a", Secondary: optStr("b")},
				`{"kind": "user", "key": "a", "_meta": {"secondary": "b"}}`},
			{servicedef.ContextBuildSingleParams{Key: "a", Private: []string{"b"}},
				`{"kind": "user", "key": "a", "_meta": {"privateAttributes": ["b"]}}`},
			{servicedef.ContextBuildSingleParams{Key: "a", Secondary: optStr("b"), Private: []string{"c"}},
				`{"kind": "user", "key": "a", "_meta": {"secondary": "b", "privateAttributes": ["c"]}}`},
			{servicedef.ContextBuildSingleParams{Key: "a", Custom: map[string]ldvalue.Value{"attr": ldvalue.Null()}},
				`{"kind": "user", "key": "a"}`},
		}
		for _, v := range data.MakeStandardTestValues() {
			if v.IsNull() {
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
			{[]servicedef.ContextBuildSingleParams{{Key: "a"}}, `{"kind": "multi", "user": {"key": "a"}}`},
			{[]servicedef.ContextBuildSingleParams{{Kind: optStr("org"), Key: "a"}, {Key: "b"}},
				`{"kind": "multi", "org": {"key": "a"}, "user": {"key": "b"}}`},
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
	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
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
			`"transient": true`,
			`"attr1": "first"`,                    // basic case of 1 custom attr
			`"attr1": "first", "attr2": "second"`, // basic case of multiple custom attrs
			`"_meta": {"secondary": "b"}`,
			`"_meta": {"privateAttributes": ["x"]}`, // basic case of 1 private attr
			`"_meta": {"privateAttributes": ["x", "y"]}`,                   // basic case of multiple private attrs
			`"_meta": {"secondary": "b", "privateAttributes": ["x", "y"]}`, // two things in _meta
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
			`{"kind": "multi", "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ013456789-_.": {"key": "x"}}`,
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
			`"_meta": {}`,
			`"_meta": {"secondary": null}`,
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
		type contextConversionParams struct {
			in, out string // out only needs to be set if it's different from in
		}

		params := []contextConversionParams{
			{`{"key": ""}`, `{"kind": "user", "key": ""}`}, // empty key *is* allowed for old user format only
			{`{"key": "a"}`, `{"kind": "user", "key": "a"}`},
			{`{"key": "a", "anonymous": true}`, `{"kind": "user", "key": "a", "transient": true}`},
			{`{"key": "a", "anonymous": false}`, `{"kind": "user", "key": "a"}`},
			{`{"key": "a", "secondary": "b"}`, `{"kind": "user", "key": "a", "_meta": {"secondary": "b"}}`},
			{`{"key": "a", "secondary": null}`, `{"kind": "user", "key": "a"}`},
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
			`{"kind": "org", "transient": null}`,
			`{"kind": "org", "transient": "yes"}`,
			`{"kind": "org", "_meta": {"secondary": 3}}`,
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
		inputs := []string{
			`{}`,
			`{"key": true}`,
			`{"key": "a", "anonymous": 3}`,
			`{"key": "a", "secondary": 3}`,
			`{"key": "a", "privateAttributeNames": 3"}`,
			`{"key": "a", "privateAttributeNames": {}"}`,
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
