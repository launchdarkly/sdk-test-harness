package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func doClientSideSummaryEventTests(t *ldtest.T) {
	t.Run("basic counter behavior", doClientSideSummaryEventBasicTest)
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

	userA := lduser.NewUser("user-a")
	userB := lduser.NewUser("user-b")
	default1 := ldvalue.String("default1")
	default2 := ldvalue.String("default2")

	dataBuilder := mockld.NewClientSDKDataBuilder()
	dataBuilder.Flag(flag1Key, flag1Result1).Flag(flag2Key, flag2Result)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t,
		WithClientSideConfig(servicedef.SDKConfigClientSideParams{
			InitialUser: userA,
		}),
		dataSource, events)

	// flag1: 2 evaluations for userA
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1Key, DefaultValue: default1})
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1Key, DefaultValue: default1})

	// flag2: 1 evaluation for userA
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag2Key, DefaultValue: default2})

	// Now change the user to userB, causing a flag data update, and do 1 more evaluation of flag1
	dataBuilder.Flag(flag1Key, flag1Result2)
	dataSource.streamingService.SetInitialData(dataBuilder.Build())
	client.SendIdentifyEvent(t, userB)

	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flag1Key, DefaultValue: default1})

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIdentifyEventForUserKey(userA.GetKey()),
		IsIdentifyEventForUserKey(userB.GetKey()),
		IsValidSummaryEventWithFlags(
			m.KV(flag1Key, m.MapOf(
				m.KV("default", m.JSONEqual(default1)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter(flag1Result1.Value, flag1Result1.Variation.Value(), flag1Result1.Version, 2),
					flagCounter(flag1Result2.Value, flag1Result2.Variation.Value(), flag1Result2.Version, 1),
				)),
			)),
			m.KV(flag2Key, m.MapOf(
				m.KV("default", m.JSONEqual(default2)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter(flag2Result.Value, flag2Result.Variation.Value(), flag2Result.Version, 1),
				)),
			)),
		)),
	)
}

func doClientSideSummaryEventUnknownFlagTest(t *ldtest.T) {
	unknownKey := "flag-x"
	user := lduser.NewUser("user-key")
	default1 := ldvalue.String("default1")

	dataBuilder := mockld.NewClientSDKDataBuilder()

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t,
		WithClientSideConfig(servicedef.SDKConfigClientSideParams{
			InitialUser: user,
		}),
		dataSource, events)

	// evaluate the unknown flag twice
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: unknownKey,
		User: o.Some(user), DefaultValue: default1})
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: unknownKey,
		User: o.Some(user), DefaultValue: default1})

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIdentifyEventForUserKey(user.GetKey()),
		IsValidSummaryEventWithFlags(
			m.KV(unknownKey, m.MapOf(
				m.KV("default", m.JSONEqual(default1)),
				m.KV("counters", m.ItemsInAnyOrder(
					unknownFlagCounter(default1, 2),
				)),
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

	userA := lduser.NewUser("user-a")
	userB := lduser.NewUser("user-b")
	defaultValue := ldvalue.String("default1")

	dataBuilder := mockld.NewClientSDKDataBuilder()
	dataBuilder.Flag(flagKey, flag1Result1)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t,
		WithClientSideConfig(servicedef.SDKConfigClientSideParams{
			InitialUser: userA,
		}),
		dataSource, events)

	// evaluate flag 10 times for userA producing value-a, 3 times for userB producing value-b
	for i := 0; i < 10; i++ {
		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flagKey,
			User: o.Some(userA), DefaultValue: defaultValue})
	}

	client.FlushEvents(t)
	payload1 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload1, m.ItemsInAnyOrder(
		IsIdentifyEventForUserKey(userA.GetKey()),
		IsValidSummaryEventWithFlags(
			m.KV(flagKey, m.MapOf(
				m.KV("default", m.JSONEqual(defaultValue)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter("value-a", flag1Result1.Variation.Value(), flag1Result1.Version, 10),
				)),
			)),
		)),
	)

	dataBuilder.Flag(flagKey, flag1Result2)
	dataSource.streamingService.SetInitialData(dataBuilder.Build())
	client.SendIdentifyEvent(t, userB)

	for i := 0; i < 3; i++ {
		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: flagKey,
			User: o.Some(userB), DefaultValue: defaultValue})
	}

	client.FlushEvents(t)
	payload2 := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload2, m.ItemsInAnyOrder(
		IsIdentifyEventForUserKey(userB.GetKey()),
		IsValidSummaryEventWithFlags(
			m.KV(flagKey, m.MapOf(
				m.KV("default", m.JSONEqual(defaultValue)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter("value-b", flag1Result2.Variation.Value(), flag1Result2.Version, 3),
				)),
			)),
		)),
	)
}
