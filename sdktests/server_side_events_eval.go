package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/sdktests/expect"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"github.com/stretchr/testify/require"
)

func baseEventsConfig() servicedef.SDKConfigEventParams {
	return servicedef.SDKConfigEventParams{
		// Set a very long flush interval so event payloads will only be flushed when we force a flush
		FlushIntervalMS: 1000000,
	}
}

func getValueTypesToTest(t *ldtest.T) []servicedef.ValueType {
	// For strongly-typed SDKs, make sure we test each of the typed Variation methods to prove
	// that they all correctly copy the flag value and default value into the event data. For
	// weakly-typed SDKs, we can just use the universal Variation method.
	var ret []servicedef.ValueType
	if t.Capabilities().Has("strongly-typed") {
		ret = append(ret,
			servicedef.ValueTypeBool,
			servicedef.ValueTypeInt,
			servicedef.ValueTypeDouble,
			servicedef.ValueTypeString,
		)
	}
	return append(ret, servicedef.ValueTypeAny)
}

func doServerSideFeatureEventTests(t *ldtest.T) {
	eventsConfig := baseEventsConfig()
	flagValues := FlagValueByTypeFactory()
	defaultValues := DefaultValueByTypeFactory()
	users := NewUserFactory("doServerSideEvaluationBasicEventTests")
	expectedReason := ldreason.NewEvalReasonFallthrough()
	untrackedFlags := FlagFactoryForValueTypes{
		KeyPrefix:    "untracked-flag-",
		ValueFactory: flagValues,
	}
	trackedFlags := FlagFactoryForValueTypes{
		KeyPrefix:      "tracked-flag-",
		ValueFactory:   flagValues,
		BuilderActions: func(b *ldbuilders.FlagBuilder) { b.TrackEvents(true) },
		Reason:         expectedReason,
	}
	dataBuilder := mockld.NewServerSDKDataBuilder()
	for _, valueType := range getValueTypesToTest(t) {
		dataBuilder.Flag(untrackedFlags.ForType(valueType))
		dataBuilder.Flag(trackedFlags.ForType(valueType))
	}
	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)

	makeEvalParams := func(
		flag ldmodel.FeatureFlag,
		user lduser.User,
		valueType servicedef.ValueType,
		detail bool,
	) servicedef.EvaluateFlagParams {
		return servicedef.EvaluateFlagParams{
			FlagKey:      flag.Key,
			User:         &user,
			ValueType:    valueType,
			DefaultValue: defaultValues(valueType),
			Detail:       detail,
		}
	}

	client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{Events: &eventsConfig}), dataSource, events)

	t.Run("only index + summary event for untracked flag", func(t *ldtest.T) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run("type: "+string(valueType), func(t *ldtest.T) {
				flag := untrackedFlags.ForType(valueType)
				user := users.NextUniqueUser()
				eventUser := mockld.ExpectedEventUserFromUser(user, eventsConfig)
				resp := client.EvaluateFlag(t, makeEvalParams(flag, user, valueType, false))
				// If the evaluation didn't return the expected value, then the rest of the test is moot
				if !expect.Value.Equals(flag.Variations[0]).Check(t, resp.Value) {
					require.Fail(t, "evaluation unexpectedly returned wrong value")
				}

				client.FlushEvents(t)
				events.ExpectAnalyticsEvents(t, defaultEventTimeout,
					expect.Event.IsIndexEvent(eventUser),
					expect.Event.HasKind("summary"),
				)
			})
		}
	})

	t.Run("full feature event for tracked flag, without reason", func(t *ldtest.T) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run("type: "+string(valueType), func(t *ldtest.T) {
				flag := trackedFlags.ForType(valueType)
				user := users.NextUniqueUser()
				eventUser := mockld.ExpectedEventUserFromUser(user, eventsConfig)
				resp := client.EvaluateFlag(t, makeEvalParams(flag, user, valueType, false))
				// If the evaluation didn't return the expected value, then the rest of the test is moot
				if !expect.Value.Equals(flagValues(valueType)).Check(t, resp.Value) {
					require.Fail(t, "evaluation unexpectedly returned wrong value")
				}

				client.FlushEvents(t)

				expectFeatureEvent := expect.Event.IsFeatureEvent(
					flag.Key,
					eventUser,
					false,
					ldvalue.NewOptionalInt(flag.Version),
					flagValues(valueType),
					ldvalue.NewOptionalInt(0),
					ldreason.EvaluationReason{},
					defaultValues(valueType),
				)

				events.ExpectAnalyticsEvents(t, defaultEventTimeout,
					expect.Event.IsIndexEvent(eventUser),
					expectFeatureEvent,
					expect.Event.HasKind("summary"),
				)
			})
		}
	})

	t.Run("full feature event for tracked flag, with reason", func(t *ldtest.T) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run("type: "+string(valueType), func(t *ldtest.T) {
				flag := trackedFlags.ForType(valueType)
				user := users.NextUniqueUser()
				eventUser := mockld.ExpectedEventUserFromUser(user, eventsConfig)
				resp := client.EvaluateFlag(t, makeEvalParams(flag, user, valueType, true))
				// If the evaluation didn't return the expected value, then the rest of the test is moot
				if !expect.Value.Equals(flagValues(valueType)).Check(t, resp.Value) {
					require.Fail(t, "evaluation unexpectedly returned wrong value")
				}

				client.FlushEvents(t)

				expectFeatureEvent := expect.Event.IsFeatureEvent(
					flag.Key,
					eventUser,
					false,
					ldvalue.NewOptionalInt(flag.Version),
					flagValues(valueType),
					ldvalue.NewOptionalInt(0),
					expectedReason,
					defaultValues(valueType),
				)

				events.ExpectAnalyticsEvents(t, defaultEventTimeout,
					expect.Event.IsIndexEvent(eventUser),
					expectFeatureEvent,
					expect.Event.HasKind("summary"),
				)
			})
		}
	})
}
