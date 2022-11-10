package sdktests

import (
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

func doSDKAttrRefTypeTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityAttrRefType)
	t.Run("construct", doSDKAttrRefConstructionTests)
	t.Run("convert", doSDKAttrRefConvertAttributeNameTests)
}

// Represents valid attribute names that don't start with a
// leading slash. These are also valid references.
func validAttrsWithoutLeadingSlash() []string {
	return []string{
		" ",
		"a",
		"ab",
		"a~b",
		"a~1b",
		"a~0b",
		"~1",
		"~0",
		"a/b",
		"a b",
		"~",
		"~/",
		"an attribute name",
		"       a       ",
	}
}

// Represents valid attributes that start with a leading slash.
// These can be converted into valid (single component) references.
func validAttrsWithLeadingSlash() []string {
	return []string{
		"/",
		"/a",
		"/a/",
		"/a//",
		"//a",
		"/a~",
		"//",
		"/~",
		"/~~",
		"/~",
		"/~/",
		"/a/b/c",
		"/a~1b~0c",
		"/a/b/c",
		"/~1/~0",
		"/ an attribute name",
	}
}

// Note: even though these tests don't involve an SDK client instance actually doing anything, so neither
// the data source nor the client itself are really involved-- because it's just the SDK library manipulating
// an Attribute Reference object-- the current test harness architecture requires all test service commands to be
// directed at a client instance.

func doSDKAttrRefConstructionTests(t *ldtest.T) {
	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
	client := NewSDKClient(t, dataSource)

	t.Run("valid", func(t *ldtest.T) {
		t.Run("plain", func(t *ldtest.T) {
			for _, p := range validAttrsWithoutLeadingSlash() {
				t.Run(jsonhelpers.ToJSONString(p), func(t *ldtest.T) {
					resp := client.AttrRefConstruct(t, servicedef.AttrRefConstructParams{Input: p})
					require.True(t, resp.Valid)
					m.In(t).Assert(resp.Components, m.Equal([]string{p}))
				})
			}
		})
		t.Run("pointer", func(t *ldtest.T) {
			t.Run("no escapes", func(t *ldtest.T) {
				type testCase struct {
					input      string
					components []string
				}
				testCases := []testCase{
					{"/a", []string{"a"}},
					{"/a/b/c", []string{"a", "b", "c"}},
					{"/foo/bar/baz", []string{"foo", "bar", "baz"}},
					{"/ ", []string{" "}},
					{"/  /  ", []string{"  ", "  "}},
					{"/ a / b ", []string{" a ", " b "}},
				}
				for _, p := range testCases {
					t.Run(jsonhelpers.ToJSONString(p.input), func(t *ldtest.T) {
						resp := client.AttrRefConstruct(t, servicedef.AttrRefConstructParams{Input: p.input})
						require.True(t, resp.Valid)
						m.In(t).Assert(resp.Components, m.Equal(p.components))
					})
				}
			})

			t.Run("escapes", func(t *ldtest.T) {
				type testCase struct {
					input      string
					components []string
				}
				testCases := []testCase{
					{"/~0", []string{"~"}},
					{"/~1", []string{"/"}},
					{"/~01", []string{"~1"}},
					{"/~10", []string{"/0"}},
					{"/a~1b", []string{"a/b"}},
					{"/a~0b", []string{"a~b"}},
					{"/~0~1/~1~0", []string{"~/", "/~"}},
					{"/~1~1/~0~0", []string{"//", "~~"}},
				}
				for _, p := range testCases {
					t.Run(jsonhelpers.ToJSONString(p.input), func(t *ldtest.T) {
						resp := client.AttrRefConstruct(t, servicedef.AttrRefConstructParams{Input: p.input})
						require.True(t, resp.Valid)
						m.In(t).Assert(resp.Components, m.Equal(p.components))
					})
				}
			})
		})
	})

	t.Run("invalid", func(t *ldtest.T) {
		testCases := []string{
			"",
			"/",
			"/~2",
			"/a~",
			"/~~",
			"/~/",
			"/~",
			"//",
			"/a/",
			"/a//",
			"//a",
		}
		for _, p := range testCases {
			t.Run(jsonhelpers.ToJSONString(p), func(t *ldtest.T) {
				resp := client.AttrRefConstruct(t, servicedef.AttrRefConstructParams{Input: p})
				require.False(t, resp.Valid)
			})
		}
	})
}

func doSDKAttrRefConvertAttributeNameTests(t *ldtest.T) {
	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
	client := NewSDKClient(t, dataSource)

	t.Run("valid attribute names", func(t *ldtest.T) {
		testCases := validAttrsWithoutLeadingSlash()
		testCases = append(testCases, validAttrsWithLeadingSlash()...)
		for _, p := range testCases {
			t.Run(jsonhelpers.ToJSONString(p), func(t *ldtest.T) {
				resp := client.AttrRefConstruct(t, servicedef.AttrRefConstructParams{Input: p, Literal: true})
				require.True(t, resp.Valid)
				require.Equal(t, 1, len(resp.Components))
				m.In(t).Assert(resp.Components[0], m.Equal(p))
			})
		}
	})
}
