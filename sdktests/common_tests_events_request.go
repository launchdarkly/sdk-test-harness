package sdktests

import (
	"strconv"
	"strings"
	"time"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
)

const currentEventSchema = 3

func (c CommonEventTests) RequestMethodAndHeaders(t *ldtest.T, headersMatcher m.Matcher) {
	t.Run("method and headers", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, nil)
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, append(c.SDKConfigurers, dataSource, events)...)

		client.SendIdentifyEvent(t, lduser.NewUser("user-key"))
		client.FlushEvents(t)

		request := events.Endpoint().RequireConnection(t, time.Second)

		m.In(t).For("request method").Assert(request.Method, m.Equal("POST"))

		m.In(t).For("request headers").Assert(request.Headers,
			m.AllOf(
				Header("X-LaunchDarkly-Event-Schema").Should(m.Equal(strconv.Itoa(currentEventSchema))),
				Header("X-LaunchDarkly-Payload-Id").Should(m.Not(m.Equal(""))),
				headersMatcher,
			),
		)
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

				client := NewSDKClient(t,
					append(c.SDKConfigurers, dataSource, WithEventsConfig(servicedef.SDKConfigEventParams{
						BaseURI: eventsURI,
					}))...)

				client.SendIdentifyEvent(t, lduser.NewUser("user-key"))
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
		client := NewSDKClient(t,
			append(c.SDKConfigurers, dataSource, events)...,
		)
		users := NewUserFactory("UniquePayloadIDs")

		numPayloads := 3
		for i := 0; i < numPayloads; i++ {
			client.SendIdentifyEvent(t, users.NextUniqueUser())
			client.FlushEvents(t)
		}

		seenIDs := make(map[string]bool)
		for i := 0; i < numPayloads; i++ {
			request := events.Endpoint().RequireConnection(t, time.Second)
			id := request.Headers.Get("X-LaunchDarkly-Payload-Id")
			m.In(t).For("payload ID").Assert(id, m.Not(m.Equal("")))
			if seenIDs[id] {
				t.Errorf("saw payload ID %q twice", id)
			}
			seenIDs[id] = true
		}
	})
}
