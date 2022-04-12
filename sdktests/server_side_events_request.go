package sdktests

import (
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"

	"github.com/stretchr/testify/assert"
)

func doServerSideEventRequestTests(t *ldtest.T) {
	user := lduser.NewUser("user-key")
	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())

	t.Run("method and headers", func(t *ldtest.T) {
		sdkKey := "my-sdk-key"
		events := NewSDKEventSink(t)
		client := NewSDKClient(t,
			WithConfig(servicedef.SDKConfigParams{
				Credential: sdkKey,
			}),
			dataSource,
			events,
		)

		client.SendIdentifyEvent(t, user)
		client.FlushEvents(t)

		request := expectRequest(t, events.Endpoint(), time.Second)

		assert.Equal(t, "POST", request.Method)
		assert.Equal(t, sdkKey, request.Headers.Get("Authorization"))
		assert.NotEqual(t, "", request.Headers.Get("X-LaunchDarkly-Payload-Id"))
		assert.Equal(t, "3", request.Headers.Get("X-LaunchDarkly-Event-Schema"))
	})

	t.Run("new payload ID for each post", func(t *ldtest.T) {
		sdkKey := "my-sdk-key"
		events := NewSDKEventSink(t)
		client := NewSDKClient(t,
			WithConfig(servicedef.SDKConfigParams{
				Credential: sdkKey,
			}),
			dataSource,
			events,
		)

		numPayloads := 3
		for i := 0; i < numPayloads; i++ {
			client.SendIdentifyEvent(t, user)
			client.FlushEvents(t)
		}

		seenIDs := make(map[string]bool)
		for i := 0; i < numPayloads; i++ {
			request := expectRequest(t, events.Endpoint(), time.Second)
			id := request.Headers.Get("X-LaunchDarkly-Payload-Id")
			assert.NotEqual(t, "", id)
			assert.False(t, seenIDs[id], "saw payload ID %q twice", id)
			seenIDs[id] = true
		}
	})

	t.Run("URL path is correct when base URI has a trailing slash", func(t *ldtest.T) {
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, dataSource, WithEventsConfig(servicedef.SDKConfigEventParams{
			BaseURI: strings.TrimSuffix(events.Endpoint().BaseURL(), "/") + "/",
		}))

		client.SendIdentifyEvent(t, user)
		client.FlushEvents(t)

		request := expectRequest(t, events.Endpoint(), time.Second)
		assert.Equal(t, "/bulk", request.URL.Path)
	})

	t.Run("URL path is correct when base URI has no trailing slash", func(t *ldtest.T) {
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, dataSource, WithEventsConfig(servicedef.SDKConfigEventParams{
			BaseURI: strings.TrimSuffix(events.Endpoint().BaseURL(), "/"),
		}))

		client.SendIdentifyEvent(t, user)
		client.FlushEvents(t)

		request := expectRequest(t, events.Endpoint(), time.Second)
		assert.Equal(t, "/bulk", request.URL.Path)
	})
}
