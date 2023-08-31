package sdktests

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
)

// These are used with the matchers API to make assertions about JSON event data. The value
// representing an individual analytics event uses the type mockld.Event, which we treat here
// as an opaque wrapper for a JSON object. The matchers can use the transformation m.JSONMap()
// to treat that object as a map, and then make assertions about the key and values in the map;
// or, for simpler assertions about a single property, they use the shortcut m.JSONProperty().
//
// In order to make tests as granular and useful as possible, we don't want to always be
// making equality assertions about entire events at once; if we did, then an SDK bug that
// affects just one detail of an event would cause failures in many tests that were supposed
// to be about other things, making it harder to isolate the problem. Therefore, we have very
// basic matchers like IsIdentifyEvent which just verify the kind of the event, and also ones
// like IsIdentifyEventForContext that look at an additional property to make sure it is for
// the right context, without verifying all of the properties.

func EventHasKind(kind string) m.Matcher {
	return m.JSONProperty("kind").Should(m.Equal(kind))
}

func HasContextKeys(context ldcontext.Context) m.Matcher {
	kvs := []m.KeyValueMatcher{}
	for _, mc := range context.GetAllIndividualContexts(nil) {
		kvs = append(kvs, m.KV(string(mc.Kind()), m.Equal(mc.Key())))
	}
	return m.JSONProperty("contextKeys").Should(m.MapOf(kvs...))
}

func HasAnyCreationDate() m.Matcher {
	return m.JSONProperty("creationDate").Should(ValueIsPositiveNonZeroInteger())
}

func HasContextObjectWithMatchingKeys(context ldcontext.Context) m.Matcher {
	if context.Multiple() {
		kvs := []m.KeyValueMatcher{
			m.KV("kind", m.Equal("multi")),
		}
		for _, mc := range context.GetAllIndividualContexts(nil) {
			kvs = append(kvs, m.KV(string(mc.Kind()),
				m.JSONProperty("key").Should(m.Equal(mc.Key()))))
		}
		return m.JSONProperty("context").Should(m.MapOf(kvs...))
	}
	return m.JSONProperty("context").Should(m.JSONProperty("key").Should(m.Equal(context.Key())))
}

func HasContextObjectWithKey(key string) m.Matcher {
	return m.JSONProperty("context").Should(m.JSONProperty("key").Should(m.Equal(key)))
}

func HasNoContextObject() m.Matcher {
	return JSONPropertyNullOrAbsent("context")
}

func IsIndexEvent() m.Matcher       { return EventHasKind("index") }
func IsIdentifyEvent() m.Matcher    { return EventHasKind("identify") }
func IsFeatureEvent() m.Matcher     { return EventHasKind("feature") }
func IsDebugEvent() m.Matcher       { return EventHasKind("debug") }
func IsCustomEvent() m.Matcher      { return EventHasKind("custom") }
func IsSummaryEvent() m.Matcher     { return EventHasKind("summary") }
func IsMigrationOpEvent() m.Matcher { return EventHasKind("migration_op") }

func IsIndexEventForContext(context ldcontext.Context) m.Matcher {
	return m.AllOf(IsIndexEvent(), HasContextObjectWithMatchingKeys(context))
}

func IsIdentifyEventForContext(context ldcontext.Context) m.Matcher {
	return m.AllOf(IsIdentifyEvent(), HasContextObjectWithMatchingKeys(context))
}

func IsCustomEventForEventKey(key string) m.Matcher {
	return m.AllOf(IsCustomEvent(), m.JSONProperty("key").Should(m.Equal(key)))
}

func IsValidFeatureEventWithConditions(isPHP bool, context ldcontext.Context, matchers ...m.Matcher) m.Matcher {
	propertyKeys := []string{"kind", "creationDate", "key", "version",
		"value", "variation", "reason", "default", "prereqOf"}
	if isPHP {
		propertyKeys = append(propertyKeys, "trackEvents", "debugEventsUntilDate", "context")
	} else {
		propertyKeys = append(propertyKeys, "contextKeys")
	}
	return m.AllOf(
		append(
			[]m.Matcher{
				IsFeatureEvent(),
				HasAnyCreationDate(),
				JSONPropertyKeysCanOnlyBe(propertyKeys...),
				h.IfElse(isPHP, HasContextObjectWithMatchingKeys(context), HasContextKeys(context)),
			},
			matchers...)...)
}

func IsValidSummaryEventWithFlags(keyValueMatchers ...m.KeyValueMatcher) m.Matcher {
	return m.AllOf(
		IsSummaryEvent(),
		JSONPropertyKeysCanOnlyBe("kind", "startDate", "endDate", "features"),
		m.JSONProperty("features").Should(m.MapOf(keyValueMatchers...)),
	)
}

func IsValidMigrationOpEventWithConditions(context ldcontext.Context, matchers ...m.Matcher) m.Matcher {
	propertyKeys := []string{
		"kind",
		"operation",
		"creationDate",
		"samplingRatio",
		"contextKeys",
		"evaluation",
		"measurements",
	}

	return m.AllOf(
		append(
			[]m.Matcher{
				IsMigrationOpEvent(),
				HasAnyCreationDate(),
				JSONPropertyKeysCanOnlyBe(propertyKeys...),
				HasContextKeys(context),
			},
			matchers...)...)
}
