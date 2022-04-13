package sdktests

import (
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"

	"github.com/stretchr/testify/assert"
)

type tagsTestParams struct {
	description         string
	tags                servicedef.SDKConfigTagsParams
	expectedHeaderValue string
}

func doServerSideTagsTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityTags)

	verifyRequestHeader := func(t *ldtest.T, p tagsTestParams, endpoint *harness.MockEndpoint) {
		request := endpoint.RequireConnection(t, time.Second)

		if p.expectedHeaderValue == "" {
			assert.NotContains(t, request.Headers, "X-LaunchDarkly-Tags")
		} else {
			assert.Equal(t, p.expectedHeaderValue, request.Headers.Get("X-LaunchDarkly-Tags"))
		}
	}

	t.Run("stream requests", func(t *ldtest.T) {
		for _, p := range makeValidTagsTestParams() {
			t.Run(p.description, func(t *ldtest.T) {
				tags := p.tags
				dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
				_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
					Tags: o.Some(tags),
				}), dataSource)
				verifyRequestHeader(t, p, dataSource.Endpoint())
			})
		}
	})

	t.Run("event posts", func(t *ldtest.T) {
		unimportantDataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
		for _, p := range makeValidTagsTestParams() {
			t.Run(p.description, func(t *ldtest.T) {
				tags := p.tags
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{Tags: o.Some(tags)}), unimportantDataSource, events)

				client.SendIdentifyEvent(t, lduser.NewUser("user-key"))
				client.FlushEvents(t)

				verifyRequestHeader(t, p, events.Endpoint())
			})
		}
	})

	t.Run("disallowed characters", func(t *ldtest.T) {
		params := []tagsTestParams{}
		badStrings := makeTagStringsWithDisallowedCharacters()
		for _, badString := range badStrings {
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
					ApplicationVersion: o.Some("ok"),
				},
				expectedHeaderValue: tagNameAppVersion + "/ok",
			})
		}
		for _, p := range params {
			// We're not using t.Run to make a subtest here because there would be so many. We'll
			// just print details of any failures we see.
			tags := p.tags
			dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
			client, err := TryNewSDKClient(t, WithConfig(servicedef.SDKConfigParams{Tags: o.Some(tags)}), dataSource)
			if err != nil {
				assert.Fail(t, "error initializing client", "for input tags: %s\nerror: %s", jsonhelpers.ToJSONString(tags), err)
				continue
			}
			if request, err := dataSource.Endpoint().AwaitConnection(time.Second); err == nil {
				headerTags := request.Headers.Get("X-LaunchDarkly-Tags")
				assert.Equal(t, p.expectedHeaderValue, headerTags, "for input tags: %s", jsonhelpers.ToJSONString(tags))
			} else {
				assert.Fail(t, "timed out waiting for request", "for input tags: %s", jsonhelpers.ToJSONString(tags))
			}
			_ = client.Close()
		}
	})
}

func makeValidTagsTestParams() []tagsTestParams {
	ret := make([]tagsTestParams, 0)
	values := []o.Maybe[string]{
		// Note that on *some* platforms, there's a distinction between "undefined" and "empty string".
		// We test both, to ensure that empty strings are correctly ignored in terms of the header.
		o.None[string](),
		o.Some(""), // empty string
		o.Some(allAllowedTagChars),
	}
	for _, appID := range values {
		for _, appVersion := range values {
			tags := servicedef.SDKConfigTagsParams{ApplicationID: appID, ApplicationVersion: appVersion}
			ret = append(ret, tagsTestParams{
				description:         jsonhelpers.ToJSONString(tags),
				tags:                tags,
				expectedHeaderValue: makeExpectedTagsHeader(tags),
			})
		}
	}
	return ret
}

func makeExpectedTagsHeader(tags servicedef.SDKConfigTagsParams) string {
	headerParts := []string{}
	if tags.ApplicationID.Value() != "" {
		headerParts = append(headerParts, "application-id/"+tags.ApplicationID.Value())
	}
	if tags.ApplicationVersion.Value() != "" {
		headerParts = append(headerParts, "application-version/"+tags.ApplicationVersion.Value())
	}
	return strings.Join(headerParts, " ")
}

func makeTagStringsWithDisallowedCharacters() []string {
	badChars := makeCharactersNotInAllowedCharsetString(allAllowedTagChars)
	ret := make([]string, 0, len(badChars))
	for _, ch := range badChars {
		ret = append(ret, "bad-"+string(ch))
	}
	return ret
}
