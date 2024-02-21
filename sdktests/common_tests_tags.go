package sdktests

import (
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"

	"github.com/stretchr/testify/assert"
)

const maxTagValueLength = 64

type tagsTestParams struct {
	description           string
	tags                  servicedef.SDKConfigTagsParams
	expectedHeaderValue   string
	unexpectedHeaderValue string
}

// CommonTagsTests groups together event-related test methods that are shared between server-side and client-side.
type CommonTagsTests struct {
	commonTestsBase
}

func NewCommonTagsTests(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) CommonTagsTests {
	return CommonTagsTests{newCommonTestsBase(t, testName, baseSDKConfigurers...)}
}

func (c CommonTagsTests) Run(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityTags)

	verifyRequestHeader := func(t *ldtest.T, p tagsTestParams, endpoint *harness.MockEndpoint) {
		request := endpoint.RequireConnection(t, time.Second)

		if p.expectedHeaderValue == "" {
			assert.NotContains(t, request.Headers, "X-LaunchDarkly-Tags")
		} else {
			assert.Equal(t, p.expectedHeaderValue, request.Headers.Get("X-LaunchDarkly-Tags"))
		}
	}

	withTagsConfig := func(tags servicedef.SDKConfigTagsParams) SDKConfigurer {
		return h.ConfigOptionFunc[servicedef.SDKConfigParams](func(config *servicedef.SDKConfigParams) error {
			config.Tags = o.Some(tags)
			return nil
		})
	}

	t.Run("stream requests", func(t *ldtest.T) {
		for _, p := range c.makeValidTagsTestParams() {
			t.Run(p.description, func(t *ldtest.T) {
				tags := p.tags
				dataSource := NewSDKDataSource(t, nil, DataSourceOptionStreaming())
				configurers := c.baseSDKConfigurationPlus(
					withTagsConfig(tags),
					dataSource)
				if c.isClientSide {
					// client-side SDKs in streaming mode may *also* need a polling data source
					configurers = append(configurers,
						NewSDKDataSource(t, nil, DataSourceOptionPolling()))
				}
				_ = NewSDKClient(t, configurers...)
				verifyRequestHeader(t, p, dataSource.Endpoint())
			})
		}
	})

	t.Run("poll requests", func(t *ldtest.T) {
		// Currently server-side SDK test services do not support polling
		t.RequireCapability(servicedef.CapabilityClientSide)

		for _, p := range c.makeValidTagsTestParams() {
			t.Run(p.description, func(t *ldtest.T) {
				tags := p.tags
				dataSource := NewSDKDataSource(t, nil, DataSourceOptionPolling())
				_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
					withTagsConfig(tags),
					dataSource)...)
				verifyRequestHeader(t, p, dataSource.Endpoint())
			})
		}
	})

	t.Run("event posts", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, nil)
		for _, p := range c.makeValidTagsTestParams() {
			t.Run(p.description, func(t *ldtest.T) {
				tags := p.tags
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					withTagsConfig(tags),
					dataSource,
					events)...)

				c.sendArbitraryEvent(t, client)
				client.FlushEvents(t)

				verifyRequestHeader(t, p, events.Endpoint())
			})
		}
	})

	runPermutations := func(t *ldtest.T, params []tagsTestParams) {
		for _, p := range params {
			// We're not using t.Run to make a subtest here because there would be so many. We'll
			// just print details of any failures we see.
			tags := p.tags
			dataSource := NewSDKDataSource(t, nil)
			client, err := TryNewSDKClient(t, c.baseSDKConfigurationPlus(
				withTagsConfig(tags),
				dataSource)...)
			if err != nil {
				assert.Fail(t, "error initializing client", "for input tags: %s\nerror: %s", jsonhelpers.ToJSONString(tags), err)
				continue
			}
			if request, err := dataSource.Endpoint().AwaitConnection(time.Second); err == nil {
				headerTags := request.Headers.Get("X-LaunchDarkly-Tags")
				if p.expectedHeaderValue != "" {
					assert.Equal(t, p.expectedHeaderValue, headerTags, "for input tags: %s", jsonhelpers.ToJSONString(tags))
				}

				if p.unexpectedHeaderValue != "" {
					assert.NotContains(t, p.unexpectedHeaderValue, headerTags, "for input tags: %s", jsonhelpers.ToJSONString(tags))
				}
			} else {
				assert.Fail(t, "timed out waiting for request", "for input tags: %s", jsonhelpers.ToJSONString(tags))
			}
			_ = client.Close()
		}
	}

	t.Run("disallowed characters", func(t *ldtest.T) {
		params := []tagsTestParams{}
		badStrings := c.makeTagStringsWithDisallowedCharacters()
		for _, badString := range badStrings {
			params = append(params, tagsTestParams{
				tags: servicedef.SDKConfigTagsParams{
					ApplicationID:      o.Some("ok"),
					ApplicationVersion: o.Some("ok"),
				},
				expectedHeaderValue: tagNameAppID + "/ok " + tagNameAppVersion + "/ok",
			})
			params = append(params, tagsTestParams{
				tags: servicedef.SDKConfigTagsParams{
					ApplicationID:      o.Some("ok"),
					ApplicationVersion: o.Some(badString),
				},
				expectedHeaderValue: tagNameAppID + "/ok",
			})
			params = append(params, tagsTestParams{
				tags: servicedef.SDKConfigTagsParams{
					ApplicationID:      o.Some(badString),
					ApplicationVersion: o.Some("iShouldntBeSeenBecauseInvalidIDTriggersFallback"),
				},
				unexpectedHeaderValue: "iShouldntBeSeenBecauseInvalidIDTriggersFallback",
			})
		}
		runPermutations(t, params)
	})

	t.Run("length limit", func(t *ldtest.T) {
		t.NonCritical("not all SDKs have tag length validation yet")

		makeStringOfLength := func(n int) string {
			// makes nice strings that look like "12345678901234" etc. so it's easier to see when one is longer than another
			b := make([]byte, n)
			for i := 0; i < n; i++ {
				b[i] = byte('0' + ((i + 1) % 10))
			}
			return string(b)
		}

		goodString := makeStringOfLength(maxTagValueLength)
		badString := makeStringOfLength(maxTagValueLength + 1)
		params := []tagsTestParams{
			{
				tags: servicedef.SDKConfigTagsParams{
					ApplicationID:      o.Some(goodString),
					ApplicationVersion: o.Some(badString),
				},
				expectedHeaderValue: tagNameAppID + "/" + goodString,
			},
			{
				tags: servicedef.SDKConfigTagsParams{
					ApplicationID:      o.Some(badString),
					ApplicationVersion: o.Some("iShouldntBeSeenBecauseInvalidIDTriggersFallback"),
				},
				unexpectedHeaderValue: "iShouldntBeSeenBecauseInvalidIDTriggersFallback",
			},
		}
		runPermutations(t, params)
	})
}

func (c CommonTagsTests) makeValidTagsTestParams() []tagsTestParams {
	ret := make([]tagsTestParams, 0)
	values := []o.Maybe[string]{
		// Note that on *some* platforms, there's a distinction between "undefined" and "empty string".
		// We test both, to ensure that empty strings are correctly ignored in terms of the header.
		o.None[string](),
		o.Some(""), // empty string
	}
	for i := 0; i < len(allAllowedTagChars); i += maxTagValueLength {
		j := h.IfElse(i > len(allAllowedTagChars), len(allAllowedTagChars), i)
		values = append(values, o.Some(allAllowedTagChars[i:j]))
	}
	for _, appID := range values {
		for _, appVersion := range values {
			tags := servicedef.SDKConfigTagsParams{ApplicationID: appID, ApplicationVersion: appVersion}
			ret = append(ret, tagsTestParams{
				description:         jsonhelpers.ToJSONString(tags),
				tags:                tags,
				expectedHeaderValue: c.makeExpectedTagsHeader(tags),
			})
		}
	}
	return ret
}

func (c CommonTagsTests) makeExpectedTagsHeader(tags servicedef.SDKConfigTagsParams) string {
	headerParts := []string{}
	if tags.ApplicationID.Value() != "" {
		headerParts = append(headerParts, "application-id/"+tags.ApplicationID.Value())
	}
	if tags.ApplicationVersion.Value() != "" {
		headerParts = append(headerParts, "application-version/"+tags.ApplicationVersion.Value())
	}
	return strings.Join(headerParts, " ")
}

func (c CommonTagsTests) makeTagStringsWithDisallowedCharacters() []string {
	badChars := makeCharactersNotInAllowedCharsetString(allAllowedTagChars)
	ret := make([]string, 0, len(badChars))
	for _, ch := range badChars {
		ret = append(ret, "bad-"+string(ch))
	}
	return ret
}
