package sdktests

import (
	"strconv"
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"

	"github.com/stretchr/testify/assert"
)

const currentEventSchemaVersion = 4

func doServerSideEventRequestTests(t *ldtest.T) {
	context := ldcontext.New("user-key")
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

		client.SendIdentifyEvent(t, context)
		client.FlushEvents(t)

		request := events.Endpoint().RequireConnection(t, time.Second)

		assert.Equal(t, "POST", request.Method)
		assert.Equal(t, sdkKey, request.Headers.Get("Authorization"))
		assert.NotEqual(t, "", request.Headers.Get("X-LaunchDarkly-Payload-Id"))
		assert.Equal(t, strconv.Itoa(currentEventSchemaVersion), request.Headers.Get("X-LaunchDarkly-Event-Schema"))
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
			client.SendIdentifyEvent(t, context)
			client.FlushEvents(t)
		}

		seenIDs := make(map[string]bool)
		for i := 0; i < numPayloads; i++ {
			request := events.Endpoint().RequireConnection(t, time.Second)
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

		client.SendIdentifyEvent(t, context)
		client.FlushEvents(t)

		request := events.Endpoint().RequireConnection(t, time.Second)
		assert.Equal(t, "/bulk", request.URL.Path)
	})

	t.Run("URL path is correct when base URI has no trailing slash", func(t *ldtest.T) {
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, dataSource, WithEventsConfig(servicedef.SDKConfigEventParams{
			BaseURI: strings.TrimSuffix(events.Endpoint().BaseURL(), "/"),
		}))

		client.SendIdentifyEvent(t, context)
		client.FlushEvents(t)

		request := events.Endpoint().RequireConnection(t, time.Second)
		assert.Equal(t, "/bulk", request.URL.Path)
	})
}
