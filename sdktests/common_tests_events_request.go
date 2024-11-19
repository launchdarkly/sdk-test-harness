package sdktests

import (
	"net/url"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

const currentEventSchema = "4"
const phpLegacyEventSchema = "2"

func (c CommonEventTests) RequestMethodAndHeaders(t *ldtest.T, credential string, headersMatcher m.Matcher) {
	t.Run("method and headers", func(t *ldtest.T) {
		for _, transport := range c.withAvailableTransports(t) {
			transport.Run(t, func(t *ldtest.T) {
				dataSource := NewSDKDataSource(t, nil)
				events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, WithEventsConfig(servicedef.SDKConfigEventParams{
					EnableGzip: o.Some(t.Capabilities().Has(servicedef.CapabilityEventGzip)),
				}),
					events, transport.configurer)...)

				c.sendArbitraryEvent(t, client)
				client.FlushEvents(t)

				request := events.Endpoint().RequireConnection(t, time.Second)

				m.In(t).For("request method").Assert(request.Method, m.Equal("POST"))

				m.In(t).For("request headers").Assert(request.Headers, m.AllOf(
					headersMatcher,
					c.authorizationHeaderMatcher(credential),
				))

				if t.Capabilities().Has(servicedef.CapabilityEventGzip) {
					m.In(t).For("request headers").Assert(request.Headers, Header("Content-Encoding").Should(m.StringContains("gzip")))
				}
			})
		}
	})
	t.Run("invalid tls certificate", func(t *ldtest.T) {
		c.withHTTPSTransport(t).Run(t, func(t *ldtest.T) {
			//// It's not expected that the data source connection will succeed (since it's an https url, and the SDK's
			//// default trust store won't contain the self-signed cert.) This test is only concerned with events; the
			//// data source is being configured because it is required by the harness.
			dataSource := NewSDKDataSource(t, nil)
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

			c.sendArbitraryEvent(t, client)
			client.FlushEvents(t)

			_, err := events.Endpoint().AwaitConnection(time.Second)
			assert.Errorf(t, err, "expected connection error")
		})
	})
}

func (c CommonEventTests) RequestURLPath(t *ldtest.T, pathMatcher m.Matcher) {
	t.Run("URL path is computed correctly", func(t *ldtest.T) {
		for _, trailingSlash := range []bool{false, true} {
			t.Run(h.IfElse(trailingSlash, "base URI has a trailing slash", "base URI has no trailing slash"), func(t *ldtest.T) {
				dataSource := NewSDKDataSource(t, nil)
				events := NewSDKEventSink(t)

				eventsURI := strings.TrimSuffix(events.Endpoint().BaseURL(), "/")
				if trailingSlash {
					eventsURI += "/"
				}

				client := NewSDKClient(t, c.baseSDKConfigurationPlus(
					dataSource,
					WithEventsConfig(servicedef.SDKConfigEventParams{
						BaseURI: eventsURI,
					}))...)

				c.sendArbitraryEvent(t, client)
				client.FlushEvents(t)

				request := events.Endpoint().RequireConnection(t, time.Second)
				m.In(t).For("request path").Assert(request.URL.Path, pathMatcher)
			})
		}
	})
}

func (c CommonEventTests) UniquePayloadIDs(t *ldtest.T) {
	t.Run("new payload ID for each post", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, nil)
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

		numPayloads := 3
		requests := make([]harness.IncomingRequestInfo, 0, numPayloads)

		for i := 0; i < numPayloads; i++ {
			c.sendArbitraryEvent(t, client)
			client.FlushEvents(t)
			requests = append(requests, events.Endpoint().RequireConnection(t, time.Second))
		}

		seenIDs := make(map[string]bool)
		for _, request := range requests {
			id := request.Headers.Get("X-LaunchDarkly-Payload-Id")
			m.In(t).For("payload ID").Assert(id, m.Not(m.Equal("")))
			if seenIDs[id] {
				t.Errorf("saw payload ID %q twice", id)
			}
			seenIDs[id] = true
		}
	})
}

func (c CommonEventTests) HTTPProxy(t *ldtest.T) {
	t.Run("http proxy", func(t *ldtest.T) {
		events := NewSDKEventSink(t)

		// The idea here is that we'll configure the SDK's service endpoints with an arbitrary host, but with the
		// correct path that the test harness expects (like /endpoints/1). Then, we'll inject the actual test harness's
		// endpoint via the HTTP Proxy configuration.
		//
		// The SDK should therefore:
		// 1. Open a socket to the test harness's host and port
		// 2. Send an HTTP request that has the arbitrary host and the correct path
		//
		// If the SDK didn't support proxying, then it would attempt to connect to the arbitrary host and
		// the harness should fail the connection assertion.
		eventURI := strings.Replace(events.Endpoint().BaseURL(), "localhost", "not.valid.local", 1)

		u, err := url.Parse(events.Endpoint().BaseURL())
		if err != nil {
			t.Errorf("unexpected error parsing URL: %s", err)
			t.FailNow()
		}
		u.Path = ""

		dataSource := NewSDKDataSource(t, nil)

		client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events, WithEventsConfig(
			servicedef.SDKConfigEventParams{
				BaseURI: eventURI,
			}), c.withHTTPProxy(u.String()))...)

		c.sendArbitraryEvent(t, client)
		client.FlushEvents(t)

		request := events.Endpoint().RequireConnection(t, time.Second)

		m.In(t).For("request method").Assert(request.Method, m.Equal("POST"))
	})
}
