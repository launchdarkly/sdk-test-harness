package sdktests

import (
	"encoding/json"

	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

// The functions in this file are for convenient use of the matchers API with complex
// types. For more information, see matchers.Transform.

func EvalResponseValue() m.MatcherTransform {
	return m.Transform(
		"result value",
		func(value interface{}) (interface{}, error) {
			r := value.(servicedef.EvaluateFlagResponse)
			return r.Value, nil
		}).
		EnsureInputValueType(servicedef.EvaluateFlagResponse{})
}

func EvalResponseVariation() m.MatcherTransform {
	return m.Transform(
		"result variation index",
		func(value interface{}) (interface{}, error) {
			r := value.(servicedef.EvaluateFlagResponse)
			return ldvalue.NewOptionalIntFromPointer(r.VariationIndex), nil
		}).
		EnsureInputValueType(servicedef.EvaluateFlagResponse{})
}

func EvalResponseReason() m.MatcherTransform {
	return m.Transform(
		"result reason",
		func(value interface{}) (interface{}, error) {
			r := value.(servicedef.EvaluateFlagResponse)
			if r.Reason == nil {
				return nil, nil
			}
			return *r.Reason, nil
		}).
		EnsureInputValueType(servicedef.EvaluateFlagResponse{})
}

func EvalAllFlagsStateMap() m.MatcherTransform {
	return m.Transform(
		"result reason",
		func(value interface{}) (interface{}, error) {
			return value.(servicedef.EvaluateAllFlagsResponse).State, nil
		}).
		EnsureInputValueType(servicedef.EvaluateAllFlagsResponse{})
}

func EvalAllFlagsValueForKeyShouldEqual(key string, value ldvalue.Value) m.Matcher {
	return EvalAllFlagsStateMap().Should(m.ValueForKey(key).Should(m.JSONEqual(value)))
}

func CanonicalizedEventJSON() m.MatcherTransform {
	return m.Transform(
		"event",
		func(value interface{}) (interface{}, error) {
			e := value.(mockld.Event)
			return json.RawMessage(e.CanonicalizedJSONString()), nil
		}).
		EnsureInputValueType(mockld.Event{})
}

func EventHasKind(kind string) m.Matcher {
	return m.JSONProperty("kind").Should(m.Equal(kind))
}

func EventIsCustomEvent(
	eventKey string,
	eventUser mockld.EventUser,
	inlineUser bool,
	data ldvalue.Value,
	metricValue *float64,
) m.Matcher {
	o := ldvalue.ObjectBuild()
	o.Set("kind", ldvalue.String("custom"))
	o.Set("key", ldvalue.String(eventKey))
	setPropertyConditionally(o, inlineUser, "user", eventUser.AsValue())
	setPropertyConditionally(o, !inlineUser, "userKey", ldvalue.String(eventUser.GetKey()))
	setPropertyConditionally(o, !data.IsNull(), "data", data)
	setPropertyConditionally(o, metricValue != nil, "metricValue", ldvalue.CopyArbitraryValue(metricValue))
	return CanonicalizedEventJSON().Should(m.JSONEqual(o.Build()))
}

func EventIsCustomEventForParams(
	params servicedef.CustomEventParams,
	eventConfig servicedef.SDKConfigEventParams,
) m.Matcher {
	return EventIsCustomEvent(
		params.EventKey,
		mockld.ExpectedEventUserFromUser(*params.User, eventConfig),
		eventConfig.InlineUsers,
		params.Data,
		params.MetricValue,
	)
}

func EventIsFeatureEvent(
	flagKey string,
	eventUser mockld.EventUser,
	inlineUser bool,
	flagVersion ldvalue.OptionalInt,
	value ldvalue.Value,
	variation ldvalue.OptionalInt,
	reason ldreason.EvaluationReason,
	defaultValue ldvalue.Value,
	prereqOfFlagKey string,
) m.Matcher {
	return eventIsFeatureOrDebugEvent("feature",
		flagKey, eventUser, inlineUser, flagVersion, value, variation, reason, defaultValue, prereqOfFlagKey)
}

func EventIsDebugEvent(
	flagKey string,
	eventUser mockld.EventUser,
	inlineUser bool,
	flagVersion ldvalue.OptionalInt,
	value ldvalue.Value,
	variation ldvalue.OptionalInt,
	reason ldreason.EvaluationReason,
	defaultValue ldvalue.Value,
	prereqOfFlagKey string,
) m.Matcher {
	return eventIsFeatureOrDebugEvent("debug",
		flagKey, eventUser, inlineUser, flagVersion, value, variation, reason, defaultValue, prereqOfFlagKey)
}

func eventIsFeatureOrDebugEvent(
	kind string,
	flagKey string,
	eventUser mockld.EventUser,
	inlineUser bool,
	flagVersion ldvalue.OptionalInt,
	value ldvalue.Value,
	variation ldvalue.OptionalInt,
	reason ldreason.EvaluationReason,
	defaultValue ldvalue.Value,
	prereqOfFlagKey string,
) m.Matcher {
	o := ldvalue.ObjectBuild()
	// For all nullable properties, we deliberately omit the property here if the value would be null,
	// even though an SDK might or might not do so, because we're comparing against the result of
	// CanonicalizedEventJSON().
	o.Set("kind", ldvalue.String(kind))
	o.Set("key", ldvalue.String(flagKey))
	o.Set("version", flagVersion.AsValue())
	setPropertyConditionally(o, inlineUser, "user", eventUser.AsValue())
	setPropertyConditionally(o, !inlineUser, "userKey", ldvalue.String(eventUser.GetKey()))
	setPropertyConditionally(o, eventUser.IsAnonymous(), "contextKind", ldvalue.String("anonymousUser"))
	// Note that we expect SDKs to omit contextKind in the usual case where its value would have been "user"
	setPropertyConditionally(o, !value.IsNull(), "value", value)
	setPropertyConditionally(o, variation.IsDefined(), "variation", variation.AsValue())
	setPropertyConditionally(o, defaultValue.IsDefined(), "default", defaultValue)
	setPropertyConditionally(o, reason.IsDefined(), "reason", ldvalue.Raw(jsonhelpers.ToJSON(reason)))
	setPropertyConditionally(o, prereqOfFlagKey != "", "prereqOf", ldvalue.String(prereqOfFlagKey))
	return CanonicalizedEventJSON().Should(m.JSONEqual(o.Build()))
}

func EventIsIdentifyEvent(eventUser mockld.EventUser) m.Matcher {
	return CanonicalizedEventJSON().Should(
		m.JSONEqual(ldvalue.ObjectBuild().
			Set("kind", ldvalue.String("identify")).
			Set("key", ldvalue.String(eventUser.GetKey())).
			Set("user", eventUser.AsValue()).
			Build()))
}

func EventIsIndexEvent(eventUser mockld.EventUser) m.Matcher {
	return CanonicalizedEventJSON().Should(
		m.JSONEqual(ldvalue.ObjectBuild().
			Set("kind", ldvalue.String("index")).
			Set("user", eventUser.AsValue()).
			Build()))
}

func EventIsSummaryEvent() m.Matcher {
	return EventHasKind("summary")
}

func ValueIsPositiveNonZeroInteger() m.Matcher {
	return m.New(
		func(value interface{}) bool {
			v := ldvalue.Parse(jsonhelpers.ToJSON(value))
			return v.IsInt() && v.IntValue() > 0
		},
		func() string {
			return "is an int > 0"
		},
		func(value interface{}) string {
			return "was not an int or was negative"
		},
	)
}
