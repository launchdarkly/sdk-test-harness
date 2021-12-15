package expect

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/mockld"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	"github.com/stretchr/testify/assert"
)

var Event EventExpectationFactory //nolint:gochecknoglobals

type EventExpectationFactory struct{}

type EventExpectation struct {
	base helpers.Expectation
}

func (x EventExpectation) For(t assert.TestingT, ev mockld.Event) bool {
	return x.base.For(t, ev)
}

func (x EventExpectation) And(other EventExpectation) EventExpectation {
	return EventExpectation{base: x.base.And(other.base)}
}

func (f EventExpectationFactory) New(
	description string,
	fn func(t assert.TestingT, ev mockld.Event) bool,
) EventExpectation {
	base := helpers.NewExpectation(
		description,
		func(value interface{}) string {
			return value.(mockld.Event).JSONString()
		},
		func(t assert.TestingT, value interface{}) bool {
			return fn == nil || fn(t, value.(mockld.Event))
		},
	)
	return EventExpectation{base}
}

func (f EventExpectationFactory) HasKind(kind string) EventExpectation {
	return f.New(fmt.Sprintf("event kind is %q", kind),
		func(t assert.TestingT, ev mockld.Event) bool {
			return assert.Equal(t, kind, ev.Kind())
		})
}

func (f EventExpectationFactory) Matches(props ldvalue.Value) EventExpectation {
	return f.New(fmt.Sprintf("event matches JSON: %s", helpers.CanonicalizedJSONString(props)),
		func(t assert.TestingT, ev mockld.Event) bool {
			return helpers.AssertJSONEqual(t, mockld.Event(props).CanonicalizedJSONString(), ev.CanonicalizedJSONString())
		})
}

func (f EventExpectationFactory) IsCustomEvent(
	eventKey string,
	eventUser mockld.EventUser,
	inlineUser bool,
	data ldvalue.Value,
	metricValue *float64,
) EventExpectation {
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
	return f.Matches(o.Build())
}

func (f EventExpectationFactory) IsFeatureEvent(
	flagKey string,
	eventUser mockld.EventUser,
	inlineUser bool,
	flagVersion ldvalue.OptionalInt,
	value ldvalue.Value,
	variation ldvalue.OptionalInt,
	reason ldreason.EvaluationReason,
	defaultValue ldvalue.Value,
) EventExpectation {
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
		o.Set("reason", helpers.AsJSONValue(reason))
	} else {
		o.Set("reason", ldvalue.Null())
	}
	return f.Matches(o.Build())
}

func (f EventExpectationFactory) IsIdentifyEvent(eventUser mockld.EventUser) EventExpectation {
	return f.Matches(ldvalue.ObjectBuild().
		Set("kind", ldvalue.String("identify")).
		Set("key", ldvalue.String(eventUser.GetKey())).
		Set("user", eventUser.AsValue()).
		Build())
}

func (f EventExpectationFactory) IsIndexEvent(eventUser mockld.EventUser) EventExpectation {
	return f.Matches(ldvalue.ObjectBuild().
		Set("kind", ldvalue.String("index")).
		Set("user", eventUser.AsValue()).
		Build())
}
