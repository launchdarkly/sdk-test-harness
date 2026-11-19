package sdktests

import (
	"fmt"
	"time"

	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

func doServerSideSummaryEventTests(t *ldtest.T) {
	t.Run("basic counter behavior", doServerSideSummaryEventBasicTest)
	t.Run("sampling ratio", doServerSideSummarySamplingRatioTests)
	t.Run("contextKinds", doServerSideSummaryEventContextKindsTest)
	t.Run("unknown flag", doServerSideSummaryEventUnknownFlagTest)
	t.Run("reset after each flush", doServerSideSummaryEventResetTest)
	t.Run("start and end dates", doSummaryEventDatesTest)
	t.Run("flag value and default value are counted separately", doServerSideSummaryEventValueAndDefaultTest)
	t.Run("prerequisites", doServerSideSummaryEventPrerequisitesTest)
	t.Run("flag versions", doServerSideSummaryEventVersionTest)
}

func doServerSideSummaryEventBasicTest(t *ldtest.T) {
	flag1 := ldbuilders.NewFlagBuilder("flag1").Version(100).
		Variations(ldvalue.String("value1a"), ldvalue.String("value1b")).
		On(true).FallthroughVariation(0).
		AddTarget(1, "user-b").
		Build()

	flag2 := ldbuilders.NewFlagBuilder("flag2").Version(200).
		Variations(ldvalue.String("value2a"), ldvalue.String("value2b")).
		On(true).FallthroughVariation(0).
		AddTarget(1, "user-b").
		Build()

	userA := ldcontext.New("user-a")
	userB := ldcontext.New("user-b")
	default1 := ldvalue.String("default1")
	default2 := ldvalue.String("default2")

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2)

	dataSystem := NewSDKDataSystem(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSystem, events)

	// evaluations for flag1: two for o.Some(userA) producing value1a, one for o.Some(userB) producing value1b
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1.Key,
		Context: o.Some(userA), DefaultValue: default1})
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1.Key,
		Context: o.Some(userB), DefaultValue: default1})
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1.Key,
		Context: o.Some(userA), DefaultValue: default1})

	// evaluations for flag2: one for o.Some(userA) producing value2a
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag2.Key,
		Context: o.Some(userA), DefaultValue: default2})

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIndexEvent(),
		IsIndexEvent(),
		IsValidSummaryEventWithFlags(
			false,
			m.KV(flag1.Key, m.MapOf(
				m.KV("default", m.JSONEqual(default1)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter("value1a", 0, flag1.Version, 2),
					flagCounter("value1b", 1, flag1.Version, 1),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
			m.KV(flag2.Key, m.MapOf(
				m.KV("default", m.JSONEqual(default2)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter("value2a", 0, flag2.Version, 1),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		)),
	)
}

func doServerSideSummarySamplingRatioTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityEventSampling)

	t.Run("flag can be excluded", flagIsExcludedFromSummaries)
	t.Run("prereq can be excluded", flagPreqIsExcludedFromSummaries)
}

func flagIsExcludedFromSummaries(t *ldtest.T) {
	flag1 := ldbuilders.NewFlagBuilder("flag1").
		On(true).
		Variations(ldvalue.String("value1a"), ldvalue.String("value1b")).
		FallthroughVariation(0).
		ExcludeFromSummaries(true).
		Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").
		On(true).
		Variations(ldvalue.String("value2a"), ldvalue.String("value2b")).
		FallthroughVariation(0).
		Build()

	userA := ldcontext.New("user-a")
	userB := ldcontext.New("user-b")
	default1 := ldvalue.String("default1")
	default2 := ldvalue.String("default2")

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2)

	dataSystem := NewSDKDataSystem(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSystem, events)

	// evaluations for flag1: two for o.Some(userA) producing value1a, one for o.Some(userB) producing value1b
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1.Key,
		Context: o.Some(userA), DefaultValue: default1})
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1.Key,
		Context: o.Some(userB), DefaultValue: default1})
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1.Key,
		Context: o.Some(userA), DefaultValue: default1})

	// evaluations for flag2: one for o.Some(userA) producing value2a
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag2.Key,
		Context: o.Some(userA), DefaultValue: default2})

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIndexEvent(),
		IsIndexEvent(),
		IsValidSummaryEventWithFlags(
			false,
			m.KV(flag2.Key, m.MapOf(
				m.KV("default", m.JSONEqual(default2)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter("value2a", 0, flag2.Version, 1),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		)),
	)
}

func flagPreqIsExcludedFromSummaries(t *ldtest.T) {
	flag1 := ldbuilders.NewFlagBuilder("flag1").
		On(true).
		Variations(ldvalue.Bool(true), ldvalue.Bool(false)).
		FallthroughVariation(0).
		ExcludeFromSummaries(true).
		Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").
		On(true).
		Variations(ldvalue.Bool(true), ldvalue.Bool(false)).
		FallthroughVariation(0).
		AddPrerequisite(flag1.Key, 0).
		Build()

	context := ldcontext.New("user-a")

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2)

	dataSystem := NewSDKDataSystem(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSystem, events)

	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag2.Key,
		Context: o.Some(context), DefaultValue: ldvalue.Bool(false)})

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIndexEvent(),
		IsValidSummaryEventWithFlags(
			false,
			m.KV(flag2.Key, m.MapOf(
				m.KV("default", m.JSONEqual(false)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter(true, 0, flag2.Version, 1),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		)),
	)
}

func doServerSideSummaryEventContextKindsTest(t *ldtest.T) {
	flag1 := ldbuilders.NewFlagBuilder("flag1").Version(100).
		Variations(ldvalue.String("value1a"), ldvalue.String("value1b")).
		On(true).FallthroughVariation(0).
		AddTarget(1, "user-b").
		Build()

	flag2 := ldbuilders.NewFlagBuilder("flag2").Version(200).
		Variations(ldvalue.String("value2a"), ldvalue.String("value2b")).
		On(true).FallthroughVariation(0).
		AddTarget(1, "user-b").
		Build()

	kind1, kind2, kind3 := ldcontext.Kind("kind1"), ldcontext.Kind("kind2"), ldcontext.Kind("kind3")
	context1a := ldcontext.NewWithKind(kind1, "key1")
	context1b := ldcontext.NewWithKind(kind1, "key2")
	context2 := ldcontext.NewWithKind(kind2, "key1")
	context3 := ldcontext.NewWithKind(kind3, "key2")

	defaultValue := ldvalue.String("default")

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag1, flag2)

	dataSystem := NewSDKDataSystem(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSystem, events)

	// evaluations for flag1: two for o.Some(userA) producing value1a, one for o.Some(userB) producing value1b
	for _, flagAndContext := range []struct {
		flag    ldmodel.FeatureFlag
		context ldcontext.Context
	}{
		{flag1, context1a},
		{flag2, context2},
		{flag1, context2},
		{flag2, context3},
		{flag1, context1b},
	} {
		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey: flagAndContext.flag.Key, Context: o.Some(flagAndContext.context), DefaultValue: defaultValue})
	}

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIndexEvent(),
		IsIndexEvent(),
		IsIndexEvent(),
		IsIndexEvent(),
		IsValidSummaryEventWithFlags(
			false,
			m.KV(flag1.Key, m.MapOf(
				m.KV("default", m.Not(m.BeNil())),
				m.KV("counters", m.JSONArray().Should(m.Not(m.BeNil()))),
				m.KV("contextKinds", contextKindsList(kind1, kind2)),
			)),
			m.KV(flag2.Key, m.MapOf(
				m.KV("default", m.Not(m.BeNil())),
				m.KV("counters", m.JSONArray().Should(m.Not(m.BeNil()))),
				m.KV("contextKinds", contextKindsList(kind2, kind3)),
			)),
		)),
	)
}

func doServerSideSummaryEventUnknownFlagTest(t *ldtest.T) {
	unknownKey := "flag-x"
	context := ldcontext.New("user-key")
	default1 := ldvalue.String("default1")

	dataBuilder := mockld.NewServerSDKDataBuilder()

	dataSystem := NewSDKDataSystem(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSystem, events)

	// evaluate the unknown flag twice
	_ = client.EvaluateFlag(t,
		servicedef.EvaluateFlagParams{FlagKey: unknownKey, Context: o.Some(context), DefaultValue: default1})
	_ = client.EvaluateFlag(t,
		servicedef.EvaluateFlagParams{FlagKey: unknownKey, Context: o.Some(context), DefaultValue: default1})

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIndexEventForContext(context),
		IsValidSummaryEventWithFlags(
			false,
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

func doServerSideSummaryEventResetTest(t *ldtest.T) {
	flag := ldbuilders.NewFlagBuilder("flag1").Version(100).
		Variations(ldvalue.String("value-a"), ldvalue.String("value-b")).
		On(true).FallthroughVariation(0).
		AddTarget(1, "user-b").
		Build()

	userA := ldcontext.New("user-a")
	userB := ldcontext.New("user-b")
	defaultValue := ldvalue.String("default1")

	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(flag)

	dataSystem := NewSDKDataSystem(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSystem, events)

	// evaluate flag 10 times for o.Some(userA) producing value-a, 3 times for o.Some(userB) producing value-b
	for i := 0; i < 10; i++ {
		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey: flag.Key, Context: o.Some(userA), DefaultValue: defaultValue})
	}
	for i := 0; i < 3; i++ {
		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey: flag.Key, Context: o.Some(userB), DefaultValue: defaultValue})
	}

	client.FlushEvents(t)
	payload1 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload1, m.ItemsInAnyOrder(
		IsIndexEvent(),
		IsIndexEvent(),
		IsValidSummaryEventWithFlags(
			false,
			m.KV(flag.Key, m.MapOf(
				m.KV("default", m.JSONEqual(defaultValue)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter("value-a", 0, flag.Version, 10),
					flagCounter("value-b", 1, flag.Version, 3),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		)),
	)

	// Now do 2 evaluations for value-b and verify that the summary shows only those, not the previous counts
	for i := 0; i < 2; i++ {
		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey: flag.Key, Context: o.Some(userB), DefaultValue: defaultValue})
	}

	client.FlushEvents(t)
	payload2 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload2, m.Items(
		IsValidSummaryEventWithFlags(
			false,
			m.KV(flag.Key, m.MapOf(
				m.KV("default", m.JSONEqual(defaultValue)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter("value-b", 1, flag.Version, 2),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		)))
}

func doServerSideSummaryEventPrerequisitesTest(t *ldtest.T) {
	context := ldcontext.New("user-key")
	expectedValue1 := ldvalue.String("value1")
	expectedPrereqValue2 := ldvalue.String("ok2")
	expectedPrereqValue3 := ldvalue.String("ok3")
	defaultValue := ldvalue.String("default1")
	flag1 := ldbuilders.NewFlagBuilder("flag1").
		On(true).OffVariation(0).FallthroughVariation(1).
		AddPrerequisite("flag2", 2).
		Variations(dummyValue0, expectedValue1).
		Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").
		On(true).OffVariation(0).FallthroughVariation(0).
		AddPrerequisite("flag3", 3).
		AddTarget(2, "user-key"). // this 2 matches the 2 in flag1's prerequisites
		Variations(dummyValue0, dummyValue1, expectedPrereqValue2).
		Build()
	flag3 := ldbuilders.NewFlagBuilder("flag3").
		On(true).OffVariation(0).FallthroughVariation(0).
		AddRule(ldbuilders.NewRuleBuilder().ID("rule1").
			Variation(3). // this 3 matches the 3 in flag2's prerequisites
			Clauses(ldbuilders.Clause(ldattr.KeyAttr, ldmodel.OperatorIn, ldvalue.String(context.Key())))).
		Variations(dummyValue0, dummyValue1, dummyValue2, expectedPrereqValue3).
		Build()

	data := mockld.NewServerSDKDataBuilder().Flag(flag1, flag2, flag3).Build()
	dataSystem := NewSDKDataSystem(t, data)
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSystem, events)

	// evaluate flag1 3 times, which should cause flag2 and flag3 to also be evaluated 3 times
	for i := 0; i < 3; i++ {
		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey: flag1.Key, Context: o.Some(context), DefaultValue: defaultValue})
	}

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIndexEvent(),
		IsValidSummaryEventWithFlags(
			false,
			m.KV(flag1.Key, m.MapOf(
				m.KV("default", m.JSONEqual(defaultValue)),
				m.KV("counters", m.Items(
					flagCounter(expectedValue1, 1, flag1.Version, 3),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
			m.KV(flag2.Key, m.MapIncluding(
				// "default" may or may not be present here since the default for a prerequisite is always null
				m.KV("counters", m.Items(
					flagCounter(expectedPrereqValue2, 2, flag2.Version, 3),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
			m.KV(flag3.Key, m.MapIncluding(
				m.KV("counters", m.Items(
					flagCounter(expectedPrereqValue3, 3, flag3.Version, 3),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		)))
}

func doServerSideSummaryEventVersionTest(t *ldtest.T) {
	// This test verifies that if the version of a flag changes within the timespan of one event payload,
	// evaluations for each version are tracked separately. We do this by evaluating the flag in its
	// original version, then pushing a stream update and polling until the SDK reports the updated
	// value, and then checking that both versions appear in the summary event. More detailed testing of
	// update behavior is covered in server_side_data_store_updates.go.

	flagKey := "flagkey"
	versionBefore, versionAfter := 100, 200
	valueBefore, valueAfter := ldvalue.String("a"), ldvalue.String("b")
	flagBefore, flagAfter := makeFlagVersionsWithValues(flagKey, versionBefore, versionAfter, valueBefore, valueAfter)
	defaultValue := ldvalue.String("default")
	context := ldcontext.New("user-key")

	data := mockld.NewServerSDKDataBuilder().Flag(flagBefore).Build()
	dataSystem := NewSDKDataSystem(t, data)
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSystem, events)

	initialValue := basicEvaluateFlag(t, client, flagKey, context, defaultValue)
	m.In(t).Require(initialValue, m.JSONEqual(valueBefore))

	dataSystem.Synchronizers[0].streaming.PushUpdate(
		"flag", flagKey, flagAfter.Version, jsonhelpers.ToJSON(flagAfter))
	//nolint:godox
	// TODO: Need to determine which version this should be, and also what the state should be
	dataSystem.Synchronizers[0].streaming.PushPayloadTransferred("state", 2)

	h.RequireEventually(
		t,
		checkForUpdatedValue(t, client, flagKey, context, valueBefore, valueAfter, defaultValue),
		time.Second,
		time.Millisecond*20,
		"timed out waiting for evaluation to return updated value",
	)

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIndexEvent(),
		IsValidSummaryEventWithFlags(
			false,
			m.KV(flagKey, m.MapOf(
				m.KV("default", m.JSONEqual(defaultValue)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounterWithAnyCount(valueBefore, 0, versionBefore),
					flagCounter(valueAfter, 1, versionAfter, 1),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		)))
}

func flagCounter(value interface{}, variation int, version int, count int) m.Matcher {
	return m.MapOf(
		m.KV("value", m.JSONEqual(value)),
		m.KV("variation", m.Equal(variation)),
		m.KV("version", m.Equal(version)),
		m.KV("count", m.Equal(count)),
	)
}

// overrideAffectedFlagCounter is the same as flagCounter, but for evaluations that the flag
// overrides feature marked as override-affected. The counter carries "overrideAffected": true.
// The marker is part of the aggregation key, so the SDK keeps these evaluations in a separate
// counter from ordinary evaluations of the same flag, variation, and version.
func overrideAffectedFlagCounter(value interface{}, variation int, version int, count int) m.Matcher {
	return m.MapOf(
		m.KV("value", m.JSONEqual(value)),
		m.KV("variation", m.Equal(variation)),
		m.KV("version", m.Equal(version)),
		m.KV("count", m.Equal(count)),
		m.KV("overrideAffected", m.Equal(true)),
	)
}

func flagCounterWithAnyCount(value interface{}, variation int, version int) m.Matcher {
	return m.MapOf(
		m.KV("value", m.JSONEqual(value)),
		m.KV("variation", m.Equal(variation)),
		m.KV("version", m.Equal(version)),
		m.KV("count", ValueIsPositiveNonZeroInteger()),
	)
}

func unknownFlagCounter(defaultValue interface{}, count int) m.Matcher {
	return m.MapOf(
		m.KV("value", m.JSONEqual(defaultValue)),
		m.KV("unknown", m.Equal(true)),
		m.KV("count", m.Equal(count)),
	)
}

func anyContextKindsList() m.Matcher {
	return m.JSONArray().Should(m.Not(m.BeNil()))
}

func contextKindsList(kinds ...ldcontext.Kind) m.Matcher {
	matchers := make([]m.Matcher, 0, len(kinds))
	for _, kind := range kinds {
		matchers = append(matchers, m.Equal(string(kind)))
	}
	return m.ItemsInAnyOrder(matchers...)
}

// doSummaryEventDatesTest verifies the summary event's time range: startDate and endDate are the
// times of the earliest and latest evaluations since the last flush. The SDK and the test harness
// run on the same host, so their clocks agree.
func doSummaryEventDatesTest(t *ldtest.T) {
	c := NewCommonEventTests(t, "doSummaryEventDatesTest")
	dataSystem := NewSDKDataSystem(t, nil)
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem, events)...)

	context := ldcontext.New("user-key")
	evaluate := func() {
		params := servicedef.EvaluateFlagParams{
			FlagKey:      "arbitrary-flag-key",
			ValueType:    servicedef.ValueTypeBool,
			DefaultValue: ldvalue.Bool(false),
		}
		if !c.isClientSide {
			params.Context = o.Some(context)
		}
		_ = client.EvaluateFlag(t, params)
	}
	// Allows for rounding to milliseconds.
	const slack = 5
	checkSummaryDates := func(desc string, payload mockld.Events, earliest, latest ldtime.UnixMillisecondTime) {
		for _, e := range payload {
			if e.Kind() != "summary" {
				continue
			}
			startDate := e.AsValue().GetByKey("startDate")
			endDate := e.AsValue().GetByKey("endDate")
			m.In(t).For(desc+" startDate").Require(startDate.IsNumber(), m.Equal(true))
			m.In(t).For(desc+" endDate").Require(endDate.IsNumber(), m.Equal(true))
			start, end := ldtime.UnixMillisecondTime(startDate.Float64Value()),
				ldtime.UnixMillisecondTime(endDate.Float64Value())
			t.Debug("%s: startDate %d, endDate %d, evaluations between %d and %d", desc, start, end, earliest, latest)
			m.In(t).For(desc+": startDate <= endDate").Assert(start <= end, m.Equal(true))
			m.In(t).For(fmt.Sprintf("%s: startDate (%d) >= time of the first evaluation since the last flush (%d)",
				desc, start, earliest)).Assert(start+slack >= earliest, m.Equal(true))
			m.In(t).For(fmt.Sprintf("%s: endDate (%d) <= time of the last evaluation (%d)", desc, end, latest)).
				Assert(end <= latest+slack, m.Equal(true))
			return
		}
		require.Fail(t, "no summary event in payload", "payload: %s", payload.JSONString())
	}

	if c.isClientSide {
		client.FlushEvents(t)
		_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event
	}

	before := ldtime.UnixMillisNow()
	evaluate()
	time.Sleep(time.Millisecond * 20)
	evaluate()
	after := ldtime.UnixMillisNow()
	client.FlushEvents(t)
	checkSummaryDates("first summary", events.ExpectAnalyticsEvents(t, defaultEventTimeout), before, after)

	time.Sleep(time.Millisecond * 20)
	before = ldtime.UnixMillisNow()
	evaluate()
	after = ldtime.UnixMillisNow()
	client.FlushEvents(t)
	checkSummaryDates("second summary", events.ExpectAnalyticsEvents(t, defaultEventTimeout), before, after)
}

func doServerSideSummaryEventValueAndDefaultTest(t *ldtest.T) {
	// One evaluation returns the first variation of the flag. Another evaluation returns the default
	// value because of a wrong-type error. The summary must count the two results separately.
	t.RequireCapability(servicedef.CapabilityStronglyTyped)

	flag := ldbuilders.NewFlagBuilder("flag").Version(1).
		On(true).FallthroughVariation(0).Variations(ldvalue.Bool(true), ldvalue.Bool(false)).Build()
	dataSystem := NewSDKDataSystem(t, mockld.NewServerSDKDataBuilder().Flag(flag).Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSystem, events)
	context := ldcontext.New("user-key")

	boolResult := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey: flag.Key, Context: o.Some(context), ValueType: servicedef.ValueTypeBool, DefaultValue: ldvalue.Bool(false)})
	m.In(t).Assert(boolResult.Value, m.JSONEqual(true))
	stringDefault := ldvalue.String("default")
	stringResult := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey: flag.Key, Context: o.Some(context), ValueType: servicedef.ValueTypeString, DefaultValue: stringDefault})
	m.In(t).Assert(stringResult.Value, m.JSONEqual(stringDefault))

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIndexEvent(),
		IsValidSummaryEventWithFlags(
			false,
			m.KV(flag.Key, m.MapIncluding(
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter(true, 0, flag.Version, 1),
					m.AllOf(
						m.MapIncluding(
							m.KV("value", m.JSONEqual(stringDefault)),
							m.KV("version", m.Equal(flag.Version)),
							m.KV("count", m.Equal(1)),
						),
						JSONPropertyNullOrAbsent("variation"),
						JSONPropertyNullOrAbsentOrEqualTo("unknown", false),
					),
				)),
			)),
		),
	))
}
