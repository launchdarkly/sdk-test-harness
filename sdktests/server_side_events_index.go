package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"
)

func doServerSideIndexEventTests(t *ldtest.T) {
	// These do not include detailed tests of the properties within the user object, which are in
	// server_side_events_users.go.

	contexts := data.NewContextFactory("doServerSideIndexEventTests")
	matchIndexEvent := func(user ldcontext.Context) m.Matcher {
		return m.AllOf(
			JSONPropertyKeysCanOnlyBe("kind", "creationDate", "context"),
			IsIndexEvent(),
			HasAnyCreationDate(),
			HasUserObjectWithKey(user.Key()),
		)
	}

	t.Run("basic properties", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, dataSource, events)

		for _, contextCategory := range data.NewContextFactoriesForAnonymousAndNonAnonymous("doServerSideIndexEventTests") {
			t.Run(contextCategory.Description(), func(t *ldtest.T) {
				context := contextCategory.NextUniqueContext()

				basicEvaluateFlag(t, client, "arbitrary-flag-key", context, ldvalue.Null())

				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					matchIndexEvent(context),
					IsSummaryEvent(),
				))
			})
		}
	})

	t.Run("only one index event per user", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())

		t.Run("from feature event", func(t *ldtest.T) {
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, dataSource, events)

			context1, context2 := contexts.NextUniqueContext(), contexts.NextUniqueContext()
			flagKey := "arbitrary-flag-key"

			basicEvaluateFlag(t, client, flagKey, context1, ldvalue.Null())
			basicEvaluateFlag(t, client, flagKey, context1, ldvalue.Null())
			basicEvaluateFlag(t, client, flagKey, context2, ldvalue.Null())
			basicEvaluateFlag(t, client, flagKey, context1, ldvalue.Null())
			basicEvaluateFlag(t, client, flagKey, context2, ldvalue.Null())

			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				matchIndexEvent(context1),
				matchIndexEvent(context2),
				IsSummaryEvent(),
			))
		})

		t.Run("from custom event", func(t *ldtest.T) {
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, dataSource, events)

			context1, context2 := contexts.NextUniqueContext(), contexts.NextUniqueContext()
			params1 := servicedef.CustomEventParams{EventKey: "event1", Context: context1}
			params2 := servicedef.CustomEventParams{EventKey: "event1", Context: context2}

			client.SendCustomEvent(t, params1)
			client.SendCustomEvent(t, params1)
			client.SendCustomEvent(t, params2)
			client.SendCustomEvent(t, params1)
			client.SendCustomEvent(t, params2)

			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				matchIndexEvent(context1),
				matchIndexEvent(context2),
				IsCustomEvent(), IsCustomEvent(), IsCustomEvent(), IsCustomEvent(), IsCustomEvent(),
			))
		})
	})
}
