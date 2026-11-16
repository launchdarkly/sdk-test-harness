package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

// wrongTypeEvaluation describes a typed evaluation whose flag value does not have the requested type.
type wrongTypeEvaluation struct {
	name         string
	flagValue    ldvalue.Value
	valueType    servicedef.ValueType
	defaultValue ldvalue.Value
}

// wrongTypeEvaluations lists the type mismatches to test. Numeric flags are not tested with the
// int and double methods, because SDKs differ in how they convert between those two types.
func wrongTypeEvaluations() []wrongTypeEvaluation {
	return []wrongTypeEvaluation{
		{"want bool, got string", ldvalue.String("a"), servicedef.ValueTypeBool, ldvalue.Bool(true)},
		{"want bool, got number", ldvalue.Int(1), servicedef.ValueTypeBool, ldvalue.Bool(true)},
		{"want int, got bool", ldvalue.Bool(true), servicedef.ValueTypeInt, ldvalue.Int(3)},
		{"want int, got string", ldvalue.String("a"), servicedef.ValueTypeInt, ldvalue.Int(3)},
		{"want double, got bool", ldvalue.Bool(true), servicedef.ValueTypeDouble, ldvalue.Float64(2.5)},
		{"want double, got string", ldvalue.String("a"), servicedef.ValueTypeDouble, ldvalue.Float64(2.5)},
		{"want string, got bool", ldvalue.Bool(true), servicedef.ValueTypeString, ldvalue.String("default")},
		{"want string, got number", ldvalue.Int(1), servicedef.ValueTypeString, ldvalue.String("default")},
	}
}

// wrongTypeFlagKey returns the key of the test flag that has the specified value.
func wrongTypeFlagKey(value ldvalue.Value) string {
	return "wrong-type-flag-" + value.Type().String()
}

// defaultValueFlagCounter matches a summary counter for evaluations that returned the default
// value for a flag that exists. The counter has the flag version but no variation.
func defaultValueFlagCounter(value interface{}, version int, count int) m.Matcher {
	return m.MapOf(
		m.KV("value", m.JSONEqual(value)),
		m.KV("version", m.Equal(version)),
		m.KV("count", m.Equal(count)),
	)
}

// doServerSideWrongTypeEventTests verifies the analytics events for a typed evaluation that returns
// the default value because the flag value has the wrong type. The events must describe the result
// that the application received: the default value, no variation, and a WRONG_TYPE reason.
func doServerSideWrongTypeEventTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityStronglyTyped)

	const flagVersion = 5
	expectedReason := ldreason.NewEvalReasonError(ldreason.EvalErrorWrongType)
	contexts := data.NewContextFactory("doServerSideWrongTypeEventTests")

	dataBuilder := mockld.NewServerSDKDataBuilder()
	for _, value := range []ldvalue.Value{ldvalue.Bool(true), ldvalue.Int(1), ldvalue.String("a")} {
		dataBuilder.Flag(ldbuilders.NewFlagBuilder(wrongTypeFlagKey(value)).Version(flagVersion).
			Variations(value).On(true).FallthroughVariation(0).TrackEvents(true).Build())
	}

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

	for _, withReason := range []bool{false, true} {
		t.Run(h.IfElse(withReason, "with reasons", "without reasons"), func(t *ldtest.T) {
			for _, eval := range wrongTypeEvaluations() {
				t.Run(eval.name, func(t *ldtest.T) {
					flagKey := wrongTypeFlagKey(eval.flagValue)
					context := contexts.NextUniqueContext()

					resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
						FlagKey:      flagKey,
						Context:      o.Some(context),
						ValueType:    eval.valueType,
						DefaultValue: eval.defaultValue,
						Detail:       withReason,
					})

					// The returned value is covered by the evaluation tests. If it is wrong here,
					// the event checks are moot.
					if !m.In(t).Assert(resp.Value, m.JSONEqual(eval.defaultValue)) {
						require.Fail(t, "evaluation did not return the default value")
					}

					client.FlushEvents(t)
					payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						IsIndexEventForContext(context),
						IsValidFeatureEventWithConditions(
							t, false, context,
							m.JSONProperty("key").Should(m.Equal(flagKey)),
							m.JSONProperty("version").Should(m.Equal(flagVersion)),
							m.JSONProperty("value").Should(m.JSONEqual(eval.defaultValue)),
							JSONPropertyNullOrAbsent("variation"),
							maybeReason(withReason, expectedReason),
							m.JSONProperty("default").Should(m.JSONEqual(eval.defaultValue)),
							JSONPropertyNullOrAbsent("prereqOf"),
						),
						IsValidSummaryEventWithFlags(
							false,
							m.KV(flagKey, m.MapOf(
								m.KV("default", m.JSONEqual(eval.defaultValue)),
								m.KV("counters", m.Items(
									defaultValueFlagCounter(eval.defaultValue, flagVersion, 1),
								)),
								m.KV("contextKinds", anyContextKindsList()),
							)),
						),
					))
				})
			}
		})
	}
}
