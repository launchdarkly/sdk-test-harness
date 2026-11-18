package sdktests

import (
	"net/http"
	"regexp"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// Requests to LaunchDarkly services identify the SDK and its version. SDKs send this in the
// User-Agent header, or in X-LaunchDarkly-User-Agent on platforms that do not let them set
// User-Agent (such as browsers).

// A user agent string is an SDK identifier, a slash, and a semantic version without a leading "v".
var userAgentPattern = regexp.MustCompile(
	`^[A-Za-z][A-Za-z0-9]*/\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

const userAgentHeader = "User-Agent"
const launchDarklyUserAgentHeader = "X-LaunchDarkly-User-Agent"

// RequestUserAgent verifies that a stream request identifies the SDK and its version.
func (c CommonStreamingTests) RequestUserAgent(t *ldtest.T) {
	t.Run("user agent", func(t *ldtest.T) {
		dataSystem, configurers := c.setupDataSystems(t, nil)
		_ = NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
		request := requireFlagDataRequest(t, dataSystem.Synchronizers[0].Endpoint())

		if c.sdkKind == mockld.JSClientSDK && request.Headers.Get(launchDarklyUserAgentHeader) == "" &&
			!userAgentPattern.MatchString(request.Headers.Get(userAgentHeader)) {
			// A browser's native EventSource cannot send any custom request headers.
			t.SkipWithReason("stream request carried no SDK user agent; the SDK may use an EventSource" +
				" implementation that cannot set request headers")
		}
		checkUserAgent(t, request.Headers)
	})
}

// RequestUserAgent verifies that a poll request identifies the SDK and its version.
func (c CommonPollingTests) RequestUserAgent(t *ldtest.T) {
	t.Run("user agent", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil, c.pollingDataSystemOptions()...)
		_ = NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)
		request := requireFlagDataRequest(t, dataSystem.Synchronizers[0].Endpoint())
		checkUserAgent(t, request.Headers)
	})
}

// RequestUserAgent verifies that an analytics event post identifies the SDK and its version.
func (c CommonEventTests) RequestUserAgent(t *ldtest.T) {
	t.Run("user agent", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, nil)
		events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem, WithEventsConfig(servicedef.SDKConfigEventParams{
			EnableGzip: o.Some(t.Capabilities().Has(servicedef.CapabilityEventGzip)),
		}), events)...)

		c.sendArbitraryEvent(t, client)
		client.FlushEvents(t)

		request := events.Endpoint().RequireConnection(t, defaultEventTimeout)
		checkUserAgent(t, request.Headers)
	})
}

// requireFlagDataRequest returns the first flag data request to the endpoint. It skips the Roku
// handshake request.
func requireFlagDataRequest(t *ldtest.T, endpoint *harness.MockEndpoint) harness.IncomingRequestInfo {
	deadline := time.Now().Add(time.Second * 5)
	request := endpoint.RequireConnection(t, time.Until(deadline))
	for request.URL.Path == mockld.StreamingPathRokuHandshake {
		request = endpoint.RequireConnection(t, time.Until(deadline))
	}
	return request
}

func checkUserAgent(t *ldtest.T, headers http.Header) {
	t.Debug("%s: %q, %s: %q", userAgentHeader, headers.Get(userAgentHeader),
		launchDarklyUserAgentHeader, headers.Get(launchDarklyUserAgentHeader))

	isSDKUserAgent := m.New(
		func(value interface{}) bool {
			s, ok := value.(string)
			return ok && userAgentPattern.MatchString(s)
		},
		func() string { return "an SDK user agent of the form Identifier/1.2.3" },
		nil,
	)
	// The correct header depends on whether the platform lets the SDK set User-Agent. The test
	// harness cannot know this, so it accepts either header.
	m.In(t).For("user agent headers").Assert(headers, m.AnyOf(
		Header(userAgentHeader).Should(isSDKUserAgent),
		Header(launchDarklyUserAgentHeader).Should(isSDKUserAgent),
	))
}
