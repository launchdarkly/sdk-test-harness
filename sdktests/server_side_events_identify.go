package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doServerSideIdentifyEventTests(t *ldtest.T) {
	// These do not include detailed tests of the encoding of user attributes in identify events,
	// which are in server_side_events_users.go.

	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

	t.Run("basic properties", func(t *ldtest.T) {
		for _, contextCategory := range data.NewContextFactoriesForAnonymousAndNonAnonymous("doServerSideIdentifyEventTests") {
			t.Run(contextCategory.Description(), func(t *ldtest.T) {
				context := contextCategory.NextUniqueContext()
				client.SendIdentifyEvent(t, context)
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					m.AllOf(
						JSONPropertyKeysCanOnlyBe("kind", "creationDate", "key", "context"),
						IsIdentifyEventForUserKey(context.Key()),
						HasAnyCreationDate(),
					),
				))
			})
		}
	})

	t.Run("identify event makes index event for same user unnecessary", func(t *ldtest.T) {
		contexts := data.NewContextFactory("doServerSideIdentifyEventTests2")
		context := contexts.NextUniqueContext()
		client.SendIdentifyEvent(t, context)
		client.SendCustomEvent(t, servicedef.CustomEventParams{
			EventKey: "event-key",
			Context:  context,
		})
		// Sending a custom event would also generate an index event for the user,
		// if we hadn't already seen that user
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			IsIdentifyEventForUserKey(context.Key()),
			IsCustomEvent(),
		))
	})
}
