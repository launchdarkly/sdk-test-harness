package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// This file is very similar to server_side_events_summary.go, except that the preconditions have to be set up
// differently because of the single-current-user model. That is, we can't do a bunch of evaluations for flag 1
// with user A getting one value and mix them in with evaluations for flag 1 with user B getting a different
// value, because there is just one current value for the flag at a time depending on the current user.

func doClientSideSummaryEventTests(t *ldtest.T) {
	t.Run("basic counter behavior", doClientSideSummaryEventBasicTest)
	t.Run("context kinds", doClientSideSummaryEventContextKindsTest)
	t.Run("unknown flag", doClientSideSummaryEventUnknownFlagTest)
	t.Run("reset after each flush", doClientSideSummaryEventResetTest)
}

func doClientSideSummaryEventBasicTest(t *ldtest.T) {
	flag1Key := "flag1"
	flag1Result1 := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value1-a"),
		Variation: o.Some(0),
		Version:   1,
	}
	flag1Result2 := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value1-b"),
		Variation: o.Some(2),
		Version:   2,
	}
	flag2Key := "flag2"
	flag2Result := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value-b"),
		Variation: o.Some(2),
		Version:   2,
	}

	contextA := ldcontext.New("user-a")
	contextB := ldcontext.New("user-b")
	default1 := ldvalue.String("default1")
	default2 := ldvalue.String("default2")

	dataBuilder := mockld.NewClientSDKDataBuilder()
	dataBuilder.Flag(flag1Key, flag1Result1).Flag(flag2Key, flag2Result)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t,
		WithClientSideConfig(servicedef.SDKConfigClientSideParams{
			InitialContext: contextA,
		}),
		dataSource, events)

	// flag1: 2 evaluations for contextA
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1Key, DefaultValue: default1})
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1Key, DefaultValue: default1})

	// flag2: 1 evaluation for contextA
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag2Key, DefaultValue: default2})

	// Now change the user to contextB, causing a flag data update, and do 1 more evaluation of flag1
	dataBuilder.Flag(flag1Key, flag1Result2)
	dataSource.streamingService.SetInitialData(dataBuilder.Build())
	client.SendIdentifyEvent(t, contextB)

	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1Key, DefaultValue: default1})

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIdentifyEventForContext(contextA),
		IsIdentifyEventForContext(contextB),
		IsValidSummaryEventWithFlags(
			m.KV(flag1Key, m.MapOf(
				m.KV("default", m.JSONEqual(default1)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter(flag1Result1.Value, flag1Result1.Variation.Value(), flag1Result1.Version, 2),
					flagCounter(flag1Result2.Value, flag1Result2.Variation.Value(), flag1Result2.Version, 1),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
			m.KV(flag2Key, m.MapOf(
				m.KV("default", m.JSONEqual(default2)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter(flag2Result.Value, flag2Result.Variation.Value(), flag2Result.Version, 1),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		)),
	)
}

func doClientSideSummaryEventContextKindsTest(t *ldtest.T) {
	flag1Key := "flag1"
	flag1Result := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value1-a"),
		Variation: o.Some(0),
		Version:   1,
	}
	flag2Key := "flag2"
	flag2Result := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value-b"),
		Variation: o.Some(2),
		Version:   2,
	}

	kind1, kind2, kind3 := ldcontext.Kind("kind1"), ldcontext.Kind("kind2"), ldcontext.Kind("kind3")
	initialContext := ldcontext.NewWithKind("other", "unimportant")
	context1a := ldcontext.NewWithKind(kind1, "key1")
	context1b := ldcontext.NewWithKind(kind1, "key2")
	context2 := ldcontext.NewWithKind(kind2, "key1")
	context3 := ldcontext.NewWithKind(kind3, "key2")

	defaultValue := ldvalue.String("default")

	dataBuilder := mockld.NewClientSDKDataBuilder()
	dataBuilder.Flag(flag1Key, flag1Result).Flag(flag2Key, flag2Result)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t,
		WithClientSideConfig(servicedef.SDKConfigClientSideParams{
			InitialContext: initialContext,
		}),
		dataSource, events)

	for _, contextAndFlags := range []struct {
		context  ldcontext.Context
		flagKeys []string
	}{
		{context1a, []string{flag1Key}},
		{context1b, []string{flag1Key}},
		{context2, []string{flag1Key, flag2Key}},
		{context3, []string{flag2Key}},
	} {
		client.SendIdentifyEvent(t, contextAndFlags.context)
		for _, flagKey := range contextAndFlags.flagKeys {
			_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flagKey, DefaultValue: defaultValue})
		}
	}

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIdentifyEvent(),
		IsIdentifyEvent(),
		IsIdentifyEvent(),
		IsIdentifyEvent(),
		IsIdentifyEvent(),
		IsValidSummaryEventWithFlags(
			m.KV(flag1Key, m.MapOf(
				m.KV("default", m.Not(m.BeNil())),
				m.KV("counters", m.JSONArray().Should(m.Not(m.BeNil()))),
				m.KV("contextKinds", contextKindsList(kind1, kind2)),
			)),
			m.KV(flag2Key, m.MapOf(
				m.KV("default", m.Not(m.BeNil())),
				m.KV("counters", m.JSONArray().Should(m.Not(m.BeNil()))),
				m.KV("contextKinds", contextKindsList(kind2, kind3)),
			)),
		)),
	)
}

func doClientSideSummaryEventUnknownFlagTest(t *ldtest.T) {
	unknownKey := "flag-x"
	context := ldcontext.New("user-key")
	default1 := ldvalue.String("default1")

	dataBuilder := mockld.NewClientSDKDataBuilder()

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t,
		WithClientSideConfig(servicedef.SDKConfigClientSideParams{
			InitialContext: context,
		}),
		dataSource, events)

	// evaluate the unknown flag twice
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: unknownKey, DefaultValue: default1})
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: unknownKey, DefaultValue: default1})

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIdentifyEventForContext(context),
		IsValidSummaryEventWithFlags(
			m.KV(unknownKey, m.MapOf(
				m.KV("default", m.JSONEqual(default1)),
				m.KV("counters", m.ItemsInAnyOrder(
					unknownFlagCounter(default1, 2),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		)),
	)
}

func doClientSideSummaryEventResetTest(t *ldtest.T) {
	flagKey := "flag1"
	flag1Result1 := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value-a"),
		Variation: o.Some(0),
		Version:   1,
	}
	flag1Result2 := mockld.ClientSDKFlag{
		Value:     ldvalue.String("value-b"),
		Variation: o.Some(2),
		Version:   2,
	}

	contextA := ldcontext.New("user-a")
	contextB := ldcontext.New("user-b")
	defaultValue := ldvalue.String("default1")

	dataBuilder := mockld.NewClientSDKDataBuilder()
	dataBuilder.Flag(flagKey, flag1Result1)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t,
		WithClientSideConfig(servicedef.SDKConfigClientSideParams{
			InitialContext: contextA,
		}),
		dataSource, events)

	// evaluate flag 10 times for contextA producing value-a, 3 times for contextB producing value-b
	for i := 0; i < 10; i++ {
		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flagKey, DefaultValue: defaultValue})
	}

	client.FlushEvents(t)
	payload1 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload1, m.ItemsInAnyOrder(
		IsIdentifyEventForContext(contextA),
		IsValidSummaryEventWithFlags(
			m.KV(flagKey, m.MapOf(
				m.KV("default", m.JSONEqual(defaultValue)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter("value-a", flag1Result1.Variation.Value(), flag1Result1.Version, 10),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		)),
	)

	dataBuilder.Flag(flagKey, flag1Result2)
	dataSource.streamingService.SetInitialData(dataBuilder.Build())
	client.SendIdentifyEvent(t, contextB)

	for i := 0; i < 3; i++ {
		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flagKey, DefaultValue: defaultValue})
	}

	client.FlushEvents(t)
	payload2 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload2, m.ItemsInAnyOrder(
		IsIdentifyEventForContext(contextB),
		IsValidSummaryEventWithFlags(
			m.KV(flagKey, m.MapOf(
				m.KV("default", m.JSONEqual(defaultValue)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter("value-b", flag1Result2.Variation.Value(), flag1Result2.Version, 3),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		)),
	)
}
