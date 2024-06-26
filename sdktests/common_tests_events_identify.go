package sdktests

import (
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

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

	t.Run("can omit anonymous contexts from index events", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityOmitAnonymousContexts)

		setup := func() (*SDKClient, *SDKEventSink) {
			dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
			eventsConfig := baseEventsConfig()
			eventsConfig.OmitAnonymousContexts = true
			events := NewSDKEventSink(t)
			eventsConfig.BaseURI = events.eventsEndpoint.BaseURL()

			return NewSDKClient(t, dataSource, WithEventsConfig(eventsConfig)), events
		}

		t.Run("does not emit any events for single context which is anonymous", func(t *ldtest.T) {
			client, events := setup()
			anonSingleContext := ldcontext.NewBuilder("anon-context1").Kind("user").Anonymous(true).Build()
			client.SendIdentifyEvent(t, anonSingleContext)
			client.FlushEvents(t)
			events.ExpectNoAnalyticsEvents(t, time.Millisecond*200)
		})

		t.Run("does not emit any events for a multi-context where all contexts are anonymous", func(t *ldtest.T) {
			client, events := setup()
			anonSingleContextA := ldcontext.NewBuilder("anon-context1").Kind("user").Anonymous(true).Build()
			anonSingleContextB := ldcontext.NewBuilder("other-context1").Kind("other").Anonymous(true).Build()
			anonMultiContext := ldcontext.NewMulti(anonSingleContextA, anonSingleContextB)
			client.SendIdentifyEvent(t, anonMultiContext)
			client.FlushEvents(t)
			events.ExpectNoAnalyticsEvents(t, time.Millisecond*200)
		})

		t.Run("omits the anonymous contexts from a multi-context", func(t *ldtest.T) {
			client, events := setup()
			anonSingleContext := ldcontext.NewBuilder("anon-context2").Kind("user").Anonymous(true).Build()
			nonAnonSingleContext := ldcontext.NewBuilder("other-context2").Kind("other").Build()
			multiContext := ldcontext.NewMulti(anonSingleContext, nonAnonSingleContext)
			client.SendIdentifyEvent(t, multiContext)
			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			identifyEventMatcher := m.AllOf(
				JSONPropertyKeysCanOnlyBe("kind", "creationDate", "context"),
				IsIdentifyEvent(),
				HasAnyCreationDate(),
				HasContextObjectWithMatchingKeys(nonAnonSingleContext),
			)

			m.In(t).Assert(payload, m.Items(identifyEventMatcher))
		})
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
