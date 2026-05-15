package sdktests

import (
	"net/http"
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
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
		for _, p := range c.makeValidTagsTestParams(t) {
			t.Run(p.description, func(t *ldtest.T) {
				tags := p.tags
				dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionStreaming())
				configurers := c.baseSDKConfigurationPlus(
					withTagsConfig(tags),
					dataSystem)
				if c.isClientSide {
					// client-side SDKs in streaming mode may *also* need a polling data source
					configurers = append(configurers,
						NewSDKDataSystem(t, nil, DataSystemOptionPolling()))
				}
				_ = NewSDKClient(t, configurers...)
				verifyRequestHeader(t, p, dataSystem.Synchronizers[0].Endpoint())
			})
		}
	})

	if t.Capabilities().HasAny(servicedef.CapabilityClientSide, servicedef.CapabilityServerSidePolling) {
		t.Run("poll requests", func(t *ldtest.T) {
			for _, p := range c.makeValidTagsTestParams(t) {
				t.Run(p.description, func(t *ldtest.T) {
					tags := p.tags
					dataSystem := NewSDKDataSystem(t, nil, DataSystemOptionPolling())
					_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
						withTagsConfig(tags),
						dataSystem)...)
					verifyRequestHeader(t, p, dataSystem.Synchronizers[0].Endpoint())
				})
			}
		})
	}

	t.Run("event posts", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil)
		for _, p := range c.makeValidTagsTestParams(t) {
			t.Run(p.description, func(t *ldtest.T) {
				tags := p.tags
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					withTagsConfig(tags),
					dataSystem,
					events)...)

				c.sendArbitraryEvent(t, client)
				client.FlushEvents(t)

				verifyRequestHeader(t, p, events.Endpoint())
			})
		}
	})

	// FDv2 introduces request shapes the streaming/polling synchronizer cases
	// above do not cover: a polling Initializer that runs before the
	// synchronizer, a Secondary Synchronizer reached after the Primary is
	// permanently removed, and the FDv1 Fallback Synchronizer reached via the
	// server-directed FDv1 Fallback Directive. The endpoint-coverage property
	// for these new request shapes is orthogonal to the tag-value variation
	// the subtests above exercise, so a single representative tags config is
	// sufficient.
	fdv2TagParams := tagsTestParams{
		tags: servicedef.SDKConfigTagsParams{
			ApplicationID:      o.Some("test-app"),
			ApplicationVersion: o.Some("1.0.0"),
		},
	}
	fdv2TagParams.expectedHeaderValue = c.makeExpectedTagsHeader(fdv2TagParams.tags)

	if !c.isClientSide {
		t.Run("polling initializer requests", func(t *ldtest.T) {
			initializerData := mockld.NewServerSDKDataBuilder().Build()
			synchronizerData := mockld.NewServerSDKDataBuilder().
				IntentCode("none").IntentReason("up-to-date").Build()
			dataSystem := NewSDKDataSystem(t, synchronizerData,
				DataSystemOptionPollingInitializer(initializerData))
			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
				withTagsConfig(fdv2TagParams.tags),
				dataSystem)...)
			verifyRequestHeader(t, fdv2TagParams, dataSystem.Initializers[0].Endpoint())
			verifyRequestHeader(t, fdv2TagParams, dataSystem.Synchronizers[0].Endpoint())
		})

		t.Run("secondary synchronizer requests after permanent fallback", func(t *ldtest.T) {
			// Primary returns 401, a non-recoverable status that permanently
			// removes it from the synchronizer chain and causes the SDK to
			// fall through to the Secondary immediately.
			primaryEndpoint := requireContext(t).harness.NewMockEndpoint(
				httphelpers.HandlerWithStatus(401), t.DebugLogger(),
				harness.MockEndpointDescription("unauthorized primary streaming service"))
			t.Defer(primaryEndpoint.Close)

			secondaryStream := mockld.NewStreamingService(
				mockld.NewServerSDKDataBuilder().Build(),
				requireContext(t).sdkKind, t.DebugLogger())
			secondaryEndpoint := requireContext(t).harness.NewMockEndpoint(
				secondaryStream, t.DebugLogger(),
				harness.MockEndpointDescription("secondary streaming service"))
			t.Defer(secondaryEndpoint.Close)

			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
				withTagsConfig(fdv2TagParams.tags),
				WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
					BaseURI: primaryEndpoint.BaseURL(),
				}),
				WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
					BaseURI: secondaryEndpoint.BaseURL(),
				}))...)

			verifyRequestHeader(t, fdv2TagParams, primaryEndpoint)
			verifyRequestHeader(t, fdv2TagParams, secondaryEndpoint)
		})
	}

	if t.Capabilities().Has(servicedef.CapabilityFDv1Fallback) {
		t.Run("FDv1 fallback directive requests", func(t *ldtest.T) {
			// FDv2 streaming responds with 403 + directive on every request
			// so the SDK transitions to the FDv1 Fallback Synchronizer. The
			// FDv1 endpoint serves an empty payload so initialization can
			// complete along the fallback path.
			streamEndpoint := requireContext(t).harness.NewMockEndpoint(
				httphelpers.HandlerWithResponse(
					403, http.Header{"X-LD-FD-Fallback": []string{"true"}}, nil),
				t.DebugLogger(),
				harness.MockEndpointDescription("FDv2 streaming service (403 + directive)"))
			t.Defer(streamEndpoint.Close)

			fdv1Endpoint := requireContext(t).harness.NewMockEndpoint(
				httphelpers.HandlerWithResponse(
					200,
					http.Header{"Content-Type": []string{"application/json"}},
					[]byte(`{"flags":{},"segments":{}}`)),
				t.DebugLogger(),
				harness.MockEndpointDescription("FDv1 polling service"))
			t.Defer(fdv1Endpoint.Close)

			_ = NewSDKClient(t, c.baseSDKConfigurationPlus(
				withTagsConfig(fdv2TagParams.tags),
				WithWaitToStart(5*time.Second, false),
				WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{
					BaseURI: streamEndpoint.BaseURL(),
				}),
				WithFDv1Fallback(servicedef.SDKConfigPollingParams{
					BaseURI: fdv1Endpoint.BaseURL(),
				}))...)

			verifyRequestHeader(t, fdv2TagParams, streamEndpoint)
			verifyRequestHeader(t, fdv2TagParams, fdv1Endpoint)
		})
	}

	runPermutations := func(t *ldtest.T, params []tagsTestParams) {
		for _, p := range params {
			// We're not using t.Run to make a subtest here because there would be so many. We'll
			// just print details of any failures we see.
			tags := p.tags
			dataSystem := NewSDKDataSystem(t, nil)
			client, err := TryNewSDKClient(t, c.baseSDKConfigurationPlus(
				withTagsConfig(tags),
				dataSystem)...)
			if err != nil {
				assert.Fail(t, "error initializing client", "for input tags: %s\nerror: %s", jsonhelpers.ToJSONString(tags), err)
				continue
			}
			if request, err := dataSystem.Synchronizers[0].Endpoint().AwaitConnection(time.Second); err == nil {
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
			if t.Capabilities().Has(servicedef.CapabilityAutoEnvAttributes) {
				params = append(params, tagsTestParams{
					tags: servicedef.SDKConfigTagsParams{
						ApplicationID:      o.Some(badString),
						ApplicationVersion: o.Some("iShouldntBeSeenBecauseInvalidIDTriggersFallback"),
					},
					unexpectedHeaderValue: tagNameAppVersion + "/iShouldntBeSeenBecauseInvalidIDTriggersFallback",
				})
			} else {
				params = append(params, tagsTestParams{
					tags: servicedef.SDKConfigTagsParams{
						ApplicationID:      o.Some(badString),
						ApplicationVersion: o.Some("ok"),
					},
					expectedHeaderValue: tagNameAppVersion + "/ok",
				})
			}

			params = append(params, tagsTestParams{
				tags: servicedef.SDKConfigTagsParams{
					ApplicationID:      o.Some("ok"),
					ApplicationVersion: o.Some(badString),
				},
				expectedHeaderValue: tagNameAppID + "/ok",
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

func (c CommonTagsTests) makeValidTagsTestParams(t *ldtest.T) []tagsTestParams {
	values := make([]o.Maybe[string], 0)

	// The auto env spec does not allow for specifying only an ID or a version.
	// Therefore, we exclude these "empty" options.
	if !t.Capabilities().Has(servicedef.CapabilityAutoEnvAttributes) {
		values = []o.Maybe[string]{
			// Note that on *some* platforms, there's a distinction between "undefined" and "empty string".
			// We test both, to ensure that empty strings are correctly ignored in terms of the header.
			o.None[string](),
			o.Some(""), // empty string
		}
	}

	// Generate test to use all valid characters
	batchSize := min(maxTagValueLength, len(allAllowedTagChars))
	for i := 0; i < len(allAllowedTagChars)-batchSize; i += batchSize {
		if i+batchSize > len(allAllowedTagChars) {
			values = append(values, o.Some(allAllowedTagChars[i:]))
		} else {
			values = append(values, o.Some(allAllowedTagChars[i:i+batchSize]))
		}
	}

	// Ensure we test the maximum length
	values = append(values, o.Some(strings.Repeat(allAllowedTagChars[1:2], maxTagValueLength)))

	ret := make([]tagsTestParams, 0, len(values)*len(values))
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
