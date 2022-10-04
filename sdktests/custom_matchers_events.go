package sdktests

import (
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
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
// like IsIdentifyEventForUserKey that look at an additional property to make sure it is for
// the right user, without verifying all of the properties.

func EventHasKind(kind string) m.Matcher {
	return m.JSONProperty("kind").Should(m.Equal(kind))
}

func HasContextKind(user lduser.User) m.Matcher {
	if user.GetAnonymous() {
		return m.JSONProperty("contextKind").Should(m.Equal("anonymousUser"))
	}
	return JSONPropertyNullOrAbsent("contextKind")
}

func HasAnyCreationDate() m.Matcher {
	return m.JSONProperty("creationDate").Should(ValueIsPositiveNonZeroInteger())
}

func HasUserObjectWithKey(key string) m.Matcher {
	return m.JSONProperty("user").Should(m.JSONProperty("key").Should(m.Equal(key)))
}

func HasNoUserObject() m.Matcher {
	return JSONPropertyNullOrAbsent("user")
}

func HasUserKeyProperty(key string) m.Matcher {
	return m.JSONProperty("userKey").Should(m.Equal(key))
}

func HasNoUserKeyProperty() m.Matcher {
	return JSONPropertyNullOrAbsent("userKey")
}

func IsIndexEvent() m.Matcher    { return EventHasKind("index") }
func IsIdentifyEvent() m.Matcher { return EventHasKind("identify") }
func IsFeatureEvent() m.Matcher  { return EventHasKind("feature") }
func IsDebugEvent() m.Matcher    { return EventHasKind("debug") }
func IsCustomEvent() m.Matcher   { return EventHasKind("custom") }
func IsAliasEvent() m.Matcher    { return EventHasKind("alias") }
func IsSummaryEvent() m.Matcher  { return EventHasKind("summary") }

func IsIndexEventForUserKey(key string) m.Matcher {
	return m.AllOf(IsIndexEvent(), HasUserObjectWithKey(key))
}

func IsIdentifyEventForUserKey(key string) m.Matcher {
	return m.AllOf(IsIdentifyEvent(), HasUserObjectWithKey(key))
}

func IsCustomEventForEventKey(key string) m.Matcher {
	return m.AllOf(IsCustomEvent(), m.JSONProperty("key").Should(m.Equal(key)))
}

func IsValidFeatureEventWithConditions(isPHP, inlineUser bool, user lduser.User, matchers ...m.Matcher) m.Matcher {
	propertyKeys := []string{"kind", "creationDate", "key", "version", "user", "userKey", "contextKind",
		"value", "variation", "reason", "default", "prereqOf"}
	if isPHP {
		propertyKeys = append(propertyKeys, "trackEvents", "debugEventsUntilDate")
		inlineUser = true // PHP SDK always inlines users
	}
	return m.AllOf(
		append(
			[]m.Matcher{
				IsFeatureEvent(),
				HasAnyCreationDate(),
				JSONPropertyKeysCanOnlyBe(propertyKeys...),
				h.IfElse(inlineUser, HasNoUserKeyProperty(), HasUserKeyProperty(user.GetKey())),
				h.IfElse(inlineUser, HasUserObjectWithKey(user.GetKey()), HasNoUserObject()),
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
