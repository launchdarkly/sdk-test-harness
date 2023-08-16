package sdktests

import (
	"strings"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// JSONMatchesContext builds a Matcher to verify that the input JSON is a valid representation of the
// specified context. This is using the regular context schema (i.e. what would be sent to evaluation
// endpoints), not the event schema.
//
// The matcher should be tolerant of all allowable variants: for instance, it is legal to include
// `"anonymous": false` in the representation rather than omitting anonymous.
func JSONMatchesContext(context ldcontext.Context) m.Matcher {
	return jsonMatchesContext(context, false, nil)
}

// JSONMatchesEventContext builds a Matcher to verify that the input JSON is a valid representation of
// the specified context within event data. The context should represent the attributes *after* any
// private attribute redaction; redactedShouldBe specifies what we expect to see in redactedAttributes.
//
// The matcher should be tolerant of all allowable variants: for instance, it is legal to include
// `"anonymous": false` in the representation rather than omitting anonymous, and attribute names that
// appear in redactedAttributes could either use the literal syntax or the slash syntax.
func JSONMatchesEventContext(context ldcontext.Context, redactedShouldBe []string) m.Matcher {
	return jsonMatchesContext(context, true, redactedShouldBe)
}

func jsonMatchesContext(topLevelContext ldcontext.Context, isEventContext bool, redactedShouldBe []string) m.Matcher {
	matchSingleKind := func(c ldcontext.Context, kindIsKnown bool) m.Matcher {
		var keys []string
		var ms []m.Matcher
		if !kindIsKnown {
			keys = append(keys, "kind")
			ms = append(ms, m.JSONProperty("kind").Should(m.Equal(string(c.Kind()))))
		}
		keys = append(keys, "key", "anonymous", "_meta")
		ms = append(ms, m.JSONProperty("key").Should(m.Equal(c.Key())))
		if c.Anonymous() {
			ms = append(ms, m.JSONProperty("anonymous").Should(m.Equal(true)))
		} else {
			ms = append(ms, JSONPropertyNullOrAbsentOrEqualTo("anonymous", false))
		}
		for _, attr := range c.GetOptionalAttributeNames(nil) {
			if value := c.GetValue(attr); value.IsDefined() {
				keys = append(keys, attr)
				ms = append(ms, m.JSONProperty(attr).Should(m.JSONEqual(value)))
			}
		}

		var meta []m.Matcher
		requireMeta := false
		if isEventContext {
			if len(redactedShouldBe) != 0 {
				meta = append(meta, m.JSONProperty("redactedAttributes").Should(RedactedAttributesAre(redactedShouldBe...)))
				requireMeta = true
			} else {
				meta = append(meta, JSONPropertyNullOrAbsentOrEqualTo("redactedAttributes", ldvalue.ArrayOf()))
			}
		} else {
			if c.PrivateAttributeCount() != 0 {
				var pa []m.Matcher
				for i := 0; i < c.PrivateAttributeCount(); i++ {
					if attr, ok := c.PrivateAttributeByIndex(i); ok {
						pa = append(pa, m.Equal(attr.String()))
					}
				}
				meta = append(meta, m.JSONProperty("privateAttributes").Should(m.ItemsInAnyOrder(pa...)))
				requireMeta = true
			} else {
				meta = append(meta, JSONPropertyNullOrAbsentOrEqualTo("privateAttributes", ldvalue.ArrayOf()))
			}
		}

		if requireMeta {
			ms = append(ms, m.JSONProperty("_meta").Should(m.AllOf(meta...)))
		} else {
			ms = append(ms, m.JSONOptProperty("_meta").Should(m.AnyOf(m.BeNil(), m.AllOf(meta...))))
		}

		ms = append(ms, JSONPropertyKeysCanOnlyBe(keys...))
		return m.AllOf(ms...)
	}

	if topLevelContext.Multiple() {
		var ms []m.Matcher
		keys := make([]string, 0)
		keys = append(keys, "kind")
		ms = append(ms, m.JSONProperty("kind").Should(m.Equal("multi")))
		for _, mc := range topLevelContext.GetAllIndividualContexts(nil) {
			ms = append(ms, m.JSONProperty(string(mc.Kind())).Should(matchSingleKind(mc, true)))
			keys = append(keys, string(mc.Kind()))
		}
		ms = append(ms, JSONPropertyKeysCanOnlyBe(keys...))
		return m.AllOf(ms...)
	}
	return matchSingleKind(topLevelContext, false)
}

// RedactedAttributesAre is a matcher for the value of an event context's redactedAttributes property,
// verifying that it has the specified attribute names/references and no others. This is not just a
// plain slice match, because 1. they can be in any order and 2. for simple attribute names, the SDK
// is allowed to send either "name" or "/name" (with any slashes or tildes escaped in the latter case).
func RedactedAttributesAre(attrStrings ...string) m.Matcher {
	matchers := make([]m.Matcher, 0, len(attrStrings))
	for _, s := range attrStrings {
		if strings.HasPrefix(s, "/") {
			matchers = append(matchers, m.Equal(s))
		} else {
			escapedName := strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
			matchers = append(matchers, m.AnyOf(m.Equal(s), m.Equal("/"+escapedName)))
		}
	}
	return m.ItemsInAnyOrder(matchers...)
}
