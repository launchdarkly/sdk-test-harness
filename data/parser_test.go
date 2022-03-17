package data

import (
	"testing"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testJSONOrYAMLStruct struct {
	Name string `json:"name"`
	On   bool   `json:"on"`
	Ints []int  `json:"ints"`
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
	m.In(t).Assert(s.Values, m.JSONStrEqual(expectedValues))
}
