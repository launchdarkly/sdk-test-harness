package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v3/data"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

func (c CommonEventTests) IdentifyEvents(t *ldtest.T) {
	// These do not include detailed tests of the encoding of user attributes in identify events,
	// which are in server_side_events_users.go.

	dataSource := NewSDKDataSource(t, nil)
	contextCategories := data.NewContextFactoriesForSingleAndMultiKind(c.contextFactory.Prefix())

	t.Run("basic properties", func(t *ldtest.T) {
		for _, contexts := range contextCategories {
			t.Run(contexts.Description(), func(t *ldtest.T) {
				events := NewSDKEventSink(t)
				client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

				context := contexts.NextUniqueContext()
				client.SendIdentifyEvent(t, context)
				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.Items(
					append(c.initialEventPayloadExpectations(),
						m.AllOf(
							JSONPropertyKeysCanOnlyBe("kind", "creationDate", "key", "context"),
							IsIdentifyEventForContext(context),
							HasAnyCreationDate(),
						),
					)...,
				))
			})
		}
	})

	if !c.isClientSide && !c.isPHP {
		t.Run("identify event makes index event for same user unnecessary", func(t *ldtest.T) {
			// This test is only done for server-side SDKs (excluding PHP), because client-side ones and PHP
			// do not do index events.
			for _, contexts := range contextCategories {
				t.Run(contexts.Description(), func(t *ldtest.T) {
					events := NewSDKEventSink(t)
					client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource, events)...)

					context := c.contextFactory.NextUniqueContext()
					client.SendIdentifyEvent(t, context)
					client.SendCustomEvent(t, servicedef.CustomEventParams{
						EventKey: "event-key",
						Context:  o.Some(context),
					})
					// Sending a custom event would also generate an index event for the user,
					// if we hadn't already seen that user
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
}
