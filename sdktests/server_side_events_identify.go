package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func doServerSideIdentifyEventTests(t *ldtest.T) {
	// These do not include detailed tests of the encoding of context attributes in identify events,
	// which are in server_side_events_contexts.go.

	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)
	contextCategories := data.NewContextFactoriesForSingleAndMultiKind("doServerSideIdentifyEventTests")

	t.Run("basic properties", func(t *ldtest.T) {
		for _, contexts := range contextCategories {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				context := contexts.NextUniqueContext()
				client.SendIdentifyEvent(t, context)
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					m.AllOf(
						JSONPropertyKeysCanOnlyBe("kind", "creationDate", "key", "context"),
						IsIdentifyEventForContext(context),
						HasAnyCreationDate(),
					),
				))
			})
		}
	})

	t.Run("identify event makes index event for same context unnecessary", func(t *ldtest.T) {
		for _, contexts := range contextCategories {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				context := contexts.NextUniqueContext()
				client.SendIdentifyEvent(t, context)
				client.SendCustomEvent(t, servicedef.CustomEventParams{
					EventKey: "event-key",
					Context:  o.Some(context),
				})
				// Sending a custom event would also generate an index event for the context,
				// if we hadn't already seen that context
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					IsIdentifyEventForContext(context),
					IsCustomEvent(),
				))
			})
		}
	})
}
