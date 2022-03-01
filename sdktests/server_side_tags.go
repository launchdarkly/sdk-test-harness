package sdktests

import (
	"time"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"

	"github.com/stretchr/testify/assert"
)

type tagsTestParams struct {
	description         string
	tags                map[string][]string
	expectedHeaderValue string
}

func doServerSideTagsTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityTags)

	verifyRequestHeader := func(t *ldtest.T, p tagsTestParams, endpoint *harness.MockEndpoint) {
		request := expectRequest(t, endpoint, time.Second)

		if p.expectedHeaderValue == "" {
			assert.NotContains(t, request.Headers, "X-LaunchDarkly-Tags")
		} else {
			assert.Equal(t, p.expectedHeaderValue, request.Headers.Get("X-LaunchDarkly-Tags"))
		}
	}

	t.Run("stream requests", func(t *ldtest.T) {
		for _, p := range makeTagsTestParams() {
			t.Run(p.description, func(t *ldtest.T) {
				dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
				_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
					Tags: p.tags,
				}), dataSource)
				verifyRequestHeader(t, p, dataSource.Endpoint())
			})
		}
	})

	t.Run("event posts", func(t *ldtest.T) {
		for _, p := range makeTagsTestParams() {
			t.Run(p.description, func(t *ldtest.T) {
				dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
					Tags: p.tags,
				}), dataSource, events)

				client.SendIdentifyEvent(t, lduser.NewUser("user-key"))
				client.FlushEvents(t)

				verifyRequestHeader(t, p, events.Endpoint())
			})
		}
	})

	t.Run("disallowed characters", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())

		type scenario struct {
			tags map[string][]string
		}
		var scenarios []scenario
		badStrings := makeTagStringsWithDisallowedCharacters()
		for _, badString := range badStrings {
			scenarios = append(scenarios, scenario{
				tags: map[string][]string{
					"goodkey": {"goodvalue"},
					badString: {"badvalue"},
				},
			})
			scenarios = append(scenarios, scenario{
				tags: map[string][]string{
					"goodkey": {badString, "goodvalue"},
				},
			})
		}
		expectedHeader := "goodkey/goodvalue"
		for _, scenario := range scenarios {
			// We're not using t.Run to make a subtest here because there would be so many. We'll
			// just print details of any failures we see.
			config := servicedef.SDKConfigParams{
				Tags: scenario.tags,
			}
			_ = NewSDKClient(t, WithConfig(config), dataSource)
			request := expectRequest(t, dataSource.Endpoint(), time.Second)
			headerTags := request.Headers.Get("X-LaunchDarkly-Tags")
			assert.Equal(t, expectedHeader, headerTags, "for input tags: %s", jsonhelpers.ToJSONString(scenario.tags))
		}
	})
}

func makeTagsTestParams() []tagsTestParams {
	return []tagsTestParams{
		{
			description:         "no tags",
			tags:                nil,
			expectedHeaderValue: "",
		},
		{
			description: "single key-value pair",
			tags: map[string][]string{
				"tagname": {"tagvalue"},
			},
			expectedHeaderValue: "tagname/tagvalue",
		},
		{
			description: "multiple keys, one value each",
			tags: map[string][]string{
				"tagname1": {"tagvalue1"},
				"tagname2": {"tagvalue2"},
			},
			expectedHeaderValue: "tagname1/tagvalue1:tagname2/tagvalue2",
		},
		{
			description: "multiple values per key",
			tags: map[string][]string{
				"tagname1": {"tagvalue1a", "tagvalue1b"},
			},
			expectedHeaderValue: "tagname1/tagvalue1a:tagname1/tagvalue1b",
		},
		{
			description: "tags are sorted by key and then value",
			tags: map[string][]string{
				"tagname2": {"tagvalue2b", "tagvalue2a"},
				"tagname4": {"tagvalue4a", "tagvalue4c", "tagvalue4b"},
				"tagname1": {"tagvalue1"},
				"tagname3": {"tagvalue3"},
			},
			expectedHeaderValue: "tagname1/tagvalue1:tagname2/tagvalue2a:tagname2/tagvalue2b" +
				":tagname3/tagvalue3:tagname4/tagvalue4a:tagname4/tagvalue4b:tagname4/tagvalue4c",
		},
		{
			description: "all allowable characters",
			tags: map[string][]string{
				"._-abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789": {
					"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-",
				},
			},
			expectedHeaderValue: "._-abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" +
				"/abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-",
		},
	}
}

func makeTagStringsWithDisallowedCharacters() []string {
	var badChars []rune
	badChars = append(badChars, '\t', '\n', '\r') // don't bother including every control character
	for ch := 1; ch <= 127; ch++ {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			ch == '.' || ch == '-' || ch == '_' {
			continue
		}
		badChars = append(badChars, rune(ch))
	}
	// Don't try to cover the whole Unicode space, just pick a couple of multi-byte characters
	badChars = append(badChars, 'é', '😀')

	ret := make([]string, 0, len(badChars))
	for _, ch := range badChars {
		ret = append(ret, "bad-"+string(ch))
	}
	return ret
}
