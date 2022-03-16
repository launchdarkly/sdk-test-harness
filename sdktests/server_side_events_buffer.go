package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldcontext"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v2/ldbuilders"

	"github.com/stretchr/testify/assert"
)

func doServerSideEventBufferTests(t *ldtest.T) {
	capacity := 20
	extraItemsOverCapacity := 3 // arbitrary non-zero value for how many events to try to add past the limit
	eventsConfig := baseEventsConfig()
	eventsConfig.Capacity = ldvalue.NewOptionalInt(capacity)

	contextFactory := data.NewContextFactory(
		"doServerSideEventBufferTests",
		func(b *ldcontext.Builder) { b.Name("my favorite user") },
	)
	contexts := make([]ldcontext.Context, 0)
	for i := 0; i < capacity+extraItemsOverCapacity; i++ {
		contexts = append(contexts, contextFactory.NextUniqueContext())
	}

	makeIdentifyEventExpectations := func(count int) []m.Matcher {
		ret := make([]m.Matcher, 0, count)
		for i := 0; i < count; i++ {
			ret = append(ret, IsIdentifyEventForContext(contexts[i]))
		}
		return ret
	}

	flag := ldbuilders.NewFlagBuilder("flag-key").Version(1).
		On(false).OffVariation(0).Variations(ldvalue.Bool(true)).Build()
	dataBuilder := mockld.NewServerSDKDataBuilder().Flag(flag)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, WithEventsConfig(eventsConfig), dataSource, events)

	t.Run("capacity is enforced", func(t *ldtest.T) {
		for _, context := range contexts {
			client.SendIdentifyEvent(t, context)
		}
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		m.In(t).Assert(payload, m.ItemsInAnyOrder(makeIdentifyEventExpectations(capacity)...))
	})

	t.Run("buffer is reset after flush", func(t *ldtest.T) {
		for _, context := range contexts {
			client.SendIdentifyEvent(t, context)
		}
		client.FlushEvents(t)
		payload1 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		assert.Len(t, payload1, capacity)

		anotherContext := contextFactory.NextUniqueContext()
		client.SendIdentifyEvent(t, anotherContext)
		client.FlushEvents(t)
		payload2 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		m.In(t).Assert(payload2, m.Items(IsIdentifyEventForContext(anotherContext)))
	})

	t.Run("summary event is still included even if buffer was full", func(t *ldtest.T) {
		for _, context := range contexts {
			client.SendIdentifyEvent(t, context)
		}

		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      flag.Key,
			Context:      contexts[0],
			ValueType:    servicedef.ValueTypeBool,
			DefaultValue: ldvalue.Bool(false),
		})

		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		expectations := append(makeIdentifyEventExpectations(capacity),
			IsSummaryEvent())
		m.In(t).Assert(payload, m.ItemsInAnyOrder(expectations...))
	})
}
