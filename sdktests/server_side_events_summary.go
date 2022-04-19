package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldmodel"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

func doServerSideSummaryEventTests(t *ldtest.T) {
	t.Run("basic counter behavior", doServerSideSummaryEventBasicTest)
	t.Run("contextKinds", doServerSideSummaryEventContextKindsTest)
	t.Run("unknown flag", doServerSideSummaryEventUnknownFlagTest)
	t.Run("reset after each flush", doServerSideSummaryEventResetTest)
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

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

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

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

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

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

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

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

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
	dataSource := NewSDKDataSource(t, data)
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

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
	dataSource := NewSDKDataSource(t, data)
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

	initialValue := basicEvaluateFlag(t, client, flagKey, context, defaultValue)
	m.In(t).Require(initialValue, m.JSONEqual(valueBefore))

	dataSource.StreamingService().PushUpdate("flags", flagKey, jsonhelpers.ToJSON(flagAfter))

	require.Eventually(
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
