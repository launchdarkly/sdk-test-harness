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
	if inlineUser {
		o.Set("user", eventUser.AsValue())
		o.Set("userKey", ldvalue.Null())
	} else {
		o.Set("user", ldvalue.Null())
		o.Set("userKey", ldvalue.String(eventUser.GetKey()))
	}
	o.Set("data", data)
	o.Set("metricValue", ldvalue.CopyArbitraryValue(metricValue))
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
) m.Matcher {
	o := ldvalue.ObjectBuild()
	o.Set("kind", ldvalue.String("feature"))
	o.Set("key", ldvalue.String(flagKey))
	o.Set("version", flagVersion.AsValue())
	if inlineUser {
		o.Set("user", eventUser.AsValue())
		o.Set("userKey", ldvalue.Null())
	} else {
		o.Set("user", ldvalue.Null())
		o.Set("userKey", ldvalue.String(eventUser.GetKey()))
	}
	o.Set("value", value)
	o.Set("variation", variation.AsValue())
	o.Set("default", defaultValue)
	if reason.IsDefined() {
		o.Set("reason", ldvalue.Raw(jsonhelpers.ToJSON(reason)))
	} else {
		o.Set("reason", ldvalue.Null())
	}
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
