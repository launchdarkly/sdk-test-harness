package testmodel

import (
	"testing"

	"github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

type testJSONOrYAMLStruct struct {
	Name string `json:"name"`
	On   bool   `json:"on"`
	Ints []int  `json:"ints"`
}

type testExpandStruct struct {
	Values ldvalue.Value `json:"values"`
}

func TestParseJSONOrYAML(t *testing.T) {
	for _, params := range []struct {
		desc  string
		input string
	}{
		{"JSON", `{"name":"x","on":true,"ints":[1,2]}`},
		{"YAML", `---
name: x
on: true
ints:
  - 1
  - 2
`},
	} {
		t.Run(params.desc, func(t *testing.T) {
			var out testJSONOrYAMLStruct
			require.NoError(t, ParseJSONOrYAML([]byte(params.input), &out))
			assert.Equal(t, "x", out.Name)
			assert.True(t, out.On)
			assert.Equal(t, []int{1, 2}, out.Ints)
		})
	}
}

func TestExpandSubstitutions(t *testing.T) {
	expectedValues := `[
  { "abc": { "key_for_abc": 1 } },
  { "def": { "key_for_def": "on" } }
]`

	for _, params := range []struct {
		desc  string
		input string
	}{
		{
			"JSON",
			`{
  "parameters": [
    { "THING_NAME": "abc", "THING_VALUE": 1 },
    { "THING_NAME": "def", "THING_VALUE": "on" }
  ],
  "values": {
    "<THING_NAME>": {
      "key_for_<THING_NAME>": "<THING_VALUE>"
    }
  }
}`,
		},
		{
			"YAML",
			`---
parameters:
  - THING_NAME: abc
    THING_VALUE: 1
  - THING_NAME: def
    THING_VALUE: on
values:
  "<THING_NAME>":
    key_for_<THING_NAME>: "<THING_VALUE>"
`,
		},
	} {
		t.Run(params.desc, func(t *testing.T) {
			expanded, err := expandSubstitutions([]byte(params.input))
			require.NoError(t, err)
			valuesList := ldvalue.ArrayBuild()
			for _, source := range expanded {
				var s testExpandStruct
				require.NoError(t, ParseJSONOrYAML(source.Data, &s))
				valuesList.Add(s.Values)
			}
			helpers.AssertJSONEqual(t, expectedValues, valuesList.Build().JSONString())
		})
	}
}

func TestExpandSubstitutionsWithPermutations(t *testing.T) {
	input := `---
parameters:
  -
    - A: 10
    - A: 11
  -
    - B: 20
    - B: 21
    - B: 22
  -
    - C: 30
    - C: 31

values:
  abc: "<A>,<B>,<C>"
`
	expectedValues := []string{
		"10,20,30", "11,20,30",
		"10,21,30", "11,21,30",
		"10,22,30", "11,22,30",
		"10,20,31", "11,20,31",
		"10,21,31", "11,21,31",
		"10,22,31", "11,22,31",
	}

	expanded, err := expandSubstitutions([]byte(input))
	require.NoError(t, err)
	var actualValues []string
	for _, source := range expanded {
		var s testExpandStruct
		require.NoError(t, ParseJSONOrYAML(source.Data, &s))
		actualValues = append(actualValues, s.Values.GetByKey("abc").StringValue())
	}
	assert.Equal(t, expectedValues, actualValues)
}

func TestCanUseYAMLAnchorReferences(t *testing.T) {
	input := `---
constants:
  reusable: &reusable_thing
    foo: 1
    bar: 2

values:
  extending_thing:
    <<: *reusable_thing
    baz: 3
`
	expectedValues := `{
  "extending_thing": {
    "foo": 1,
	"bar": 2,
	"baz": 3
  }
}`

	var s testExpandStruct
	require.NoError(t, ParseJSONOrYAML([]byte(input), &s))
	helpers.AssertJSONEqual(t, expectedValues, s.Values.JSONString())
}
