package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"

	"github.com/stretchr/testify/require"
)

func doServerSideIndexEventTests(t *ldtest.T) {
	// These do not include detailed tests of the properties within the context object, which are in
	// server_side_events_contexts.go.

	contexts := data.NewContextFactory("doServerSideIndexEventTests")
	matchIndexEvent := func(context ldcontext.Context) m.Matcher {
		return m.AllOf(
			JSONPropertyKeysCanOnlyBe("kind", "creationDate", "context"),
			IsIndexEvent(),
			HasAnyCreationDate(),
			HasContextObjectWithMatchingKeys(context),
		)
	}

	t.Run("basic properties", func(t *ldtest.T) {
		// Details of the JSON representation of the context are tested in server_side_events_contexts.go.
		dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())
		events := NewSDKEventSink(t)
		client := NewSDKClient(t, dataSource, events)

		context := contexts.NextUniqueContext()

		basicEvaluateFlag(t, client, "arbitrary-flag-key", context, ldvalue.Null())

		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			matchIndexEvent(context),
			IsSummaryEvent(),
		))
	})

	t.Run("only one index event per evaluation context", func(t *ldtest.T) {
		dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())

		// Contexts are supposed to be deduplicated not just by key, but by the fully qualified key which
		// is different for different kinds, and is a composite key for multi-kind contexts. So, here we
		// will use NewContextFactoriesForSingleAndMultiKind to give us some factories for those different
		// cases, and we will deliberately override the "no key collisions between factories" behavior so
		// that they will produce some identical keys for different kinds.
		makeContextsAndIndexEventMatchers := func(t *ldtest.T) ([]ldcontext.Context, []m.Matcher) {
			contextCategories := data.NewContextFactoriesForSingleAndMultiKind("doServerSideIndexEventTests.deduplication")
			for i := 1; i < len(contextCategories); i++ {
				contextCategories[i].SetKeyDisambiguatorValueSameAs(contextCategories[0])
			}
			cs := make([]ldcontext.Context, 0, len(contextCategories)*2)
			ms := make([]m.Matcher, 0, len(contextCategories)*2)
			for _, factory := range contextCategories {
				cs = append(cs, factory.NextUniqueContext())
				cs = append(cs, factory.NextUniqueContext())
			}

			// Verify that we did indeed produce some duplicate keys (but not duplicate fully-qualified keys)
			individualKeysUsed, fullyQualifiedKeysUsed := make(map[string]bool), make(map[string]bool)
			atLeastOneIndividualKeyReused := false
			for _, c := range cs {
				atLeastOneIndividualKeyReused = atLeastOneIndividualKeyReused || individualKeysUsed[c.Key()]
				individualKeysUsed[c.Key()] = true
				require.NotContains(t, fullyQualifiedKeysUsed, c.FullyQualifiedKey(), "failure in input data generation logic")
				fullyQualifiedKeysUsed[c.FullyQualifiedKey()] = true
			}
			require.True(t, atLeastOneIndividualKeyReused, "failure in input data generation logic")

			for _, c := range cs {
				ms = append(ms, matchIndexEvent(c))
			}
			return cs, ms
		}

		t.Run("from feature event", func(t *ldtest.T) {
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, dataSource, events)

			uniqueContexts, matchers := makeContextsAndIndexEventMatchers(t)

			flagKey := "arbitrary-flag-key"
			for i := 0; i < 3; i++ { // 3 = arbitrary number of repetitions to prove we're deduplicating
				for _, c := range uniqueContexts {
					basicEvaluateFlag(t, client, flagKey, c, ldvalue.Null())
				}
			}

			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			matchers = append(matchers, IsSummaryEvent())
			m.In(t).Assert(payload, m.ItemsInAnyOrder(matchers...))
		})

		t.Run("from custom event", func(t *ldtest.T) {
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, dataSource, events)

			uniqueContexts, matchers := makeContextsAndIndexEventMatchers(t)
			for i := 0; i < 3; i++ { // 3 = arbitrary number of repetitions to prove we're deduplicating
				for _, c := range uniqueContexts {
					client.SendCustomEvent(t, servicedef.CustomEventParams{EventKey: "event1", Context: c})
					matchers = append(matchers, IsCustomEvent())
				}
			}

			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			m.In(t).Assert(payload, m.ItemsInAnyOrder(matchers...))
		})
	})
}
