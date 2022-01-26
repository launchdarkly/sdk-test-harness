package mockld

import (
	"encoding/json"
	"sort"

	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

// Event is a JSON representation of an event. For convenience, this is stored as ldvalue.Value.
type Event ldvalue.Value

// Events is an array of events. This specialized type provides helper methods.
type Events []Event

// EventUser is a JSON representation of a user in an event. LaunchDarkly's JSON schema for users
// in events is slightly different than users as SDK inputs.
type EventUser ldvalue.Value

func EventFromMap(m map[string]interface{}) Event {
	return Event(ldvalue.CopyArbitraryValue(m))
}

func (e Event) Kind() string {
	return ldvalue.Value(e).GetByKey("kind").StringValue()
}

// CanonicalizedJSONString transforms the JSON formatting of the event to make comparisons
// reliable, as follows: 1. Drop the creationDate property, since its value is unpredictable.
// 2. Call EventUser.CanonicalizedJSONString on user properties, if any. 3. Ensure that
// all expected properties, if nullable, do appear in the object when null instead of being
// omitted.
func (e Event) CanonicalizedJSONString() string {
	b := ldvalue.ObjectBuild()
	for key, value := range ldvalue.Value(e).AsValueMap().AsMap() {
		if key == "creationDate" {
			continue // We won't try to make assertions about the timestamp, just omit it
		}
		if key == "user" {
			b.Set(key, ldvalue.Raw([]byte(EventUser(value).CanonicalizedJSONString())))
		} else {
			b.Set(key, value)
		}
	}
	var nullableProps []string
	switch e.Kind() {
	case "feature", "debug":
		nullableProps = []string{"userKey", "user", "version", "variation", "reason", "default", "prereqOf"}
	case "custom":
		nullableProps = []string{"userKey", "user", "data", "metricValue"}
	}
	for _, k := range nullableProps {
		if !ldvalue.Value(e).GetByKey(k).IsDefined() {
			b.Set(k, ldvalue.Null())
		}
	}
	return b.Build().JSONString()
}

func (e Event) AsValue() ldvalue.Value { return ldvalue.Value(e) }
func (e Event) JSONString() string {
	return string(jsonhelpers.CanonicalizeJSON([]byte(e.AsValue().JSONString())))
}
func (e Event) String() string { return e.JSONString() }

func (e Event) MarshalJSON() ([]byte, error) {
	return json.Marshal(ldvalue.Value(e))
}

func (e *Event) UnmarshalJSON(data []byte) error {
	var v ldvalue.Value
	err := json.Unmarshal(data, &v)
	*e = Event(v)
	return err
}

func (es Events) JSONString() string {
	data, _ := json.Marshal(es)
	return string(data)
}

func SimpleEventUser(user lduser.User) EventUser {
	// This function can be used when we're not testing anything related to private attributes
	return ExpectedEventUserFromUser(user, servicedef.SDKConfigEventParams{})
}

func ExpectedEventUserFromUser(user lduser.User, eventsConfig servicedef.SDKConfigEventParams) EventUser {
	// This simulates the expected behavior of SDK event processors with regard to redacting
	// private attributes. For more details about how this works, please see the SDK
	// documentation, and/or the implementations of the equivalent logic in the SDKs
	// (such as https://github.com/launchdarkly/go-sdk-events).

	// First, get the regular JSON representation of the user, since it's simplest to treat
	// this as a transformation of one JSON object to another.
	allJSON := ldvalue.Parse(jsonhelpers.ToJSON(user))
	o := ldvalue.ObjectBuild()
	var custom ldvalue.ObjectBuilder
	var private []ldvalue.Value
	allAttributes := append(allJSON.Keys(), allJSON.GetByKey("custom").Keys()...)

	// allAttributes is now a list of all of the user's top-level properties plus all of
	// its custom attribute names. It's simplest to loop through all of those at once since
	// the logic for determining whether an attribute should be private is always the same.
	for _, attr := range allAttributes {
		if attr == "custom" || attr == "privateAttributeNames" {
			// "custom" and "privateAttributeNames" aren't considered user attributes, they
			// are just details of the JSON schema
			continue
		}
		// An attribute is private if 1. it was marked private for that particular user (as
		// reported by user.IsPrivateAttribute), 2. the SDK configuration (represented here
		// as eventsConfig) says that that particular one should always be private, or 3.
		// the SDK configuration says *all* of them should be private. Note that "key" can
		// never be private.
		isPrivate := attr != "key" && (eventsConfig.AllAttributesPrivate ||
			user.IsPrivateAttribute(lduser.UserAttribute(attr)))
		for _, pa := range eventsConfig.GlobalPrivateAttributes {
			isPrivate = isPrivate || string(pa) == attr
		}
		if isPrivate {
			private = append(private, ldvalue.String(attr))
		} else {
			if _, isTopLevel := allJSON.TryGetByKey(attr); isTopLevel {
				o.Set(attr, user.GetAttribute(lduser.UserAttribute(attr)))
			} else {
				if custom == nil {
					custom = ldvalue.ObjectBuild()
				}
				custom.Set(attr, user.GetAttribute(lduser.UserAttribute(attr)))
			}
		}
	}
	if custom != nil {
		o.Set("custom", custom.Build())
	}
	if len(private) != 0 {
		sort.Slice(private, func(i, j int) bool { return private[i].StringValue() < private[j].StringValue() })
		o.Set("privateAttrs", ldvalue.ArrayOf(private...))
		// event schema uses "privateAttrs" rather than "privateAttributeNames"
	}
	return EventUser(o.Build())
}

// CanonicalizedJSONString transforms the JSON formatting of the EventUser to make comparisons
// reliable, as follows: 1. Ensure that the "privateAttrs" array, if present, is sorted. 2. Omit
// the "custom" object if it is empty.
func (u EventUser) CanonicalizedJSONString() string {
	// Transform the user JSON to make comparisons reliable, by ensuring that the privateAttrs
	// array (if present) is sorted.
	o := ldvalue.ObjectBuild()
	for key, value := range ldvalue.Value(u).AsValueMap().AsMap() {
		switch key {
		case "privateAttrs":
			var values = value.AsValueArray().AsSlice()
			sort.Slice(values, func(i, j int) bool { return values[i].StringValue() < values[j].StringValue() })
			o.Set(key, ldvalue.ArrayOf(values...))
		case "custom":
			if value.Count() != 0 {
				o.Set(key, value)
			}
		default:
			o.Set(key, value)
		}
	}
	return o.Build().JSONString()
}

func (u EventUser) AsValue() ldvalue.Value { return ldvalue.Value(u) }

func (u EventUser) GetKey() string { return u.AsValue().GetByKey("key").StringValue() }
