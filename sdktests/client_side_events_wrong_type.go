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
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

// doClientSideWrongTypeEventTests is the client-side version of doServerSideWrongTypeEventTests.
// The events for a typed evaluation that returns the default value because the flag value has the
// wrong type must describe that result: the default value, no variation, and a WRONG_TYPE reason.
func doClientSideWrongTypeEventTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityStronglyTyped)

	const flagVersion = 5
	expectedReason := ldreason.NewEvalReasonError(ldreason.EvalErrorWrongType)
	context := data.NewContextFactory("doClientSideWrongTypeEventTests").NextUniqueContext()

	dataBuilder := mockld.NewClientSDKDataBuilder()
	for _, value := range []ldvalue.Value{ldvalue.Bool(true), ldvalue.Int(1), ldvalue.String("a")} {
		dataBuilder.Flag(wrongTypeFlagKey(value), mockld.ClientSDKFlag{
			Value:       value,
			Variation:   o.Some(0),
			Reason:      o.Some(ldreason.NewEvalReasonFallthrough()),
			Version:     50,
			FlagVersion: o.Some(flagVersion),
			TrackEvents: true,
		})
	}

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
	client := NewSDKClient(t,
		WithClientSideInitialContext(context),
		dataSource, events)

	client.FlushEvents(t)
	_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event

	perContextSummaries := t.Capabilities().Has(servicedef.CapabilityClientPerContextSummaries)

	for _, withReason := range []bool{false, true} {
		t.Run(h.IfElse(withReason, "with reasons", "without reasons"), func(t *ldtest.T) {
			for _, eval := range wrongTypeEvaluations() {
				t.Run(eval.name, func(t *ldtest.T) {
					flagKey := wrongTypeFlagKey(eval.flagValue)

					resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
						FlagKey:      flagKey,
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

					flagSummary := m.KV(flagKey, m.MapOf(
						m.KV("default", m.JSONEqual(eval.defaultValue)),
						m.KV("counters", m.Items(
							defaultValueFlagCounter(eval.defaultValue, flagVersion, 1),
						)),
						m.KV("contextKinds", anyContextKindsList()),
					))

					m.In(t).Assert(payload, m.ItemsInAnyOrder(
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
						h.IfElse(perContextSummaries,
							IsValidSummaryEventWithContextAndFlags(context, flagSummary),
							IsValidSummaryEventWithFlags(false, flagSummary)),
					))
				})
			}
		})
	}
}
