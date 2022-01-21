package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doServerSideFeatureEventTests(t *ldtest.T) {
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
	malformedFlag := ldbuilders.NewFlagBuilder("bad-flag").Version(1).
		On(false).OffVariation(-1).TrackEvents(true).Build()

	dataBuilder := mockld.NewServerSDKDataBuilder()
	for _, valueType := range getValueTypesToTest(t) {
		dataBuilder.Flag(untrackedFlags.ForType(valueType))
		dataBuilder.Flag(trackedFlags.ForType(valueType))
	}
	dataBuilder.Flag(malformedFlag)

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

	client := NewSDKClient(t, dataSource, events)

	t.Run("only index + summary event for untracked flag", func(t *ldtest.T) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				flag := untrackedFlags.ForType(valueType)
				user := users.NextUniqueUser()
				eventUser := mockld.SimpleEventUser(user)
				resp := client.EvaluateFlag(t, makeEvalParams(flag, user, valueType, false))
				// If the evaluation didn't return the expected value, then the rest of the test is moot
				if !m.In(t).Assert(flag.Variations[0], m.JSONEqual(resp.Value)) {
					require.Fail(t, "evaluation unexpectedly returned wrong value")
				}

				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					EventIsIndexEvent(eventUser),
					EventHasKind("summary"),
				))
			})
		}
	})

	t.Run("full feature event for tracked flag, without reason", func(t *ldtest.T) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				flag := trackedFlags.ForType(valueType)
				user := users.NextUniqueUser()
				eventUser := mockld.SimpleEventUser(user)
				resp := client.EvaluateFlag(t, makeEvalParams(flag, user, valueType, false))
				// If the evaluation didn't return the expected value, then the rest of the test is moot
				if !m.In(t).Assert(flagValues(valueType), m.JSONEqual(resp.Value)) {
					require.Fail(t, "evaluation unexpectedly returned wrong value")
				}

				client.FlushEvents(t)

				matchFeatureEvent := EventIsFeatureEvent(
					flag.Key,
					eventUser,
					false,
					ldvalue.NewOptionalInt(flag.Version),
					flagValues(valueType),
					ldvalue.NewOptionalInt(0),
					ldreason.EvaluationReason{},
					defaultValues(valueType),
					"",
				)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					EventIsIndexEvent(eventUser),
					matchFeatureEvent,
					EventHasKind("summary"),
				))
			})
		}
	})

	t.Run("full feature event for tracked flag, with reason", func(t *ldtest.T) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				flag := trackedFlags.ForType(valueType)
				user := users.NextUniqueUser()
				eventUser := mockld.SimpleEventUser(user)
				resp := client.EvaluateFlag(t, makeEvalParams(flag, user, valueType, true))
				// If the evaluation didn't return the expected value, then the rest of the test is moot
				if !m.In(t).Assert(flagValues(valueType), m.JSONEqual(resp.Value)) {
					require.Fail(t, "evaluation unexpectedly returned wrong value")
				}

				client.FlushEvents(t)

				matchFeatureEvent := EventIsFeatureEvent(
					flag.Key,
					eventUser,
					false,
					ldvalue.NewOptionalInt(flag.Version),
					flagValues(valueType),
					ldvalue.NewOptionalInt(0),
					expectedReason,
					defaultValues(valueType),
					"",
				)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					EventIsIndexEvent(eventUser),
					matchFeatureEvent,
					EventHasKind("summary"),
				))
			})
		}
	})

	t.Run("full feature event for failed tracked flag, with reason", func(t *ldtest.T) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				defaultValue := defaultValues(valueType)
				user := users.NextUniqueUser()
				eventUser := mockld.SimpleEventUser(user)
				resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      malformedFlag.Key,
					User:         &user,
					ValueType:    valueType,
					DefaultValue: defaultValue,
					Detail:       true,
				})
				m.In(t).Assert(resp.Value, m.JSONEqual(defaultValue))

				client.FlushEvents(t)

				matchFeatureEvent := EventIsFeatureEvent(
					malformedFlag.Key,
					eventUser,
					false,
					ldvalue.NewOptionalInt(malformedFlag.Version),
					defaultValue,
					ldvalue.OptionalInt{},
					ldreason.NewEvalReasonError(ldreason.EvalErrorMalformedFlag),
					defaultValue,
					"",
				)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					EventIsIndexEvent(eventUser),
					matchFeatureEvent,
					EventHasKind("summary"),
				))
			})
		}
	})
}

func doServerSideFeaturePrerequisiteEventTests(t *ldtest.T) {
	user := lduser.NewUser("user-key")
	eventUser := mockld.SimpleEventUser(user)

	expectedValue1 := ldvalue.String("value1")
	expectedPrereqValue2 := ldvalue.String("ok2")
	expectedPrereqValue3 := ldvalue.String("ok3")
	flag1 := ldbuilders.NewFlagBuilder("flag1").
		On(true).OffVariation(0).FallthroughVariation(1).
		AddPrerequisite("flag2", 2).
		Variations(dummyValue0, expectedValue1).
		TrackEvents(true).
		Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").
		On(true).OffVariation(0).FallthroughVariation(0).
		AddPrerequisite("flag3", 3).
		AddTarget(2, "user-key"). // this 2 matches the 2 in flag1's prerequisites
		Variations(dummyValue0, dummyValue1, expectedPrereqValue2).
		TrackEvents(true).
		Build()
	flag3 := ldbuilders.NewFlagBuilder("flag3").
		On(true).OffVariation(0).FallthroughVariation(0).
		AddRule(ldbuilders.NewRuleBuilder().ID("rule1").
			Variation(3). // this 3 matches the 3 in flag2's prerequisites
			Clauses(ldbuilders.Clause(lduser.KeyAttribute, ldmodel.OperatorIn, ldvalue.String(user.GetKey())))).
		Variations(dummyValue0, dummyValue1, dummyValue2, expectedPrereqValue3).
		TrackEvents(true).
		Build()

	for _, withReason := range []bool{false, true} {
		t.Run(testDescWithOrWithoutReason(withReason), func(t *ldtest.T) {
			dataBuilder := mockld.NewServerSDKDataBuilder()
			dataBuilder.Flag(flag1, flag2, flag3)

			dataSource := NewSDKDataSource(t, dataBuilder.Build())
			events := NewSDKEventSink(t)
			client := NewSDKClient(t, dataSource, events)

			var expectedReason1, expectedReason2, expectedReason3 ldreason.EvaluationReason
			if withReason {
				expectedReason1 = ldreason.NewEvalReasonFallthrough()
				expectedReason2 = ldreason.NewEvalReasonTargetMatch()
				expectedReason3 = ldreason.NewEvalReasonRuleMatch(0, "rule1")
			}

			result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
				FlagKey:      flag1.Key,
				User:         &user,
				ValueType:    servicedef.ValueTypeString,
				DefaultValue: ldvalue.String("default"),
				Detail:       withReason,
			})
			m.In(t).Assert(result.Value, m.JSONEqual(expectedValue1))
			if withReason {
				assert.Equal(t, ldvalue.NewOptionalInt(1), ldvalue.NewOptionalIntFromPointer(result.VariationIndex))
				m.In(t).Assert(result.Reason, m.JSONEqual(expectedReason1))
			}

			client.FlushEvents(t)
			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				EventIsIndexEvent(eventUser),
				EventIsFeatureEvent(
					flag1.Key,
					eventUser,
					false,
					ldvalue.NewOptionalInt(flag1.Version),
					ldvalue.String("value1"),
					ldvalue.NewOptionalInt(1),
					expectedReason1,
					ldvalue.String("default"),
					"",
				),
				EventIsFeatureEvent(
					flag2.Key,
					eventUser,
					false,
					ldvalue.NewOptionalInt(flag2.Version),
					ldvalue.String("ok2"),
					ldvalue.NewOptionalInt(2),
					expectedReason2,
					ldvalue.Null(),
					"flag1",
				),
				EventIsFeatureEvent(
					flag3.Key,
					eventUser,
					false,
					ldvalue.NewOptionalInt(flag3.Version),
					ldvalue.String("ok3"),
					ldvalue.NewOptionalInt(3),
					expectedReason3,
					ldvalue.Null(),
					"flag2",
				),
				EventIsSummaryEvent(),
			))
		})
	}
}
