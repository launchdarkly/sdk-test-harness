package mockld

import (
	"encoding/json"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"
)

// Event is a JSON representation of an event. For convenience, this is stored as ldvalue.Value.
type Event ldvalue.Value

// Events is an array of events. This specialized type provides helper methods.
type Events []Event

func (e Event) Kind() string {
	return ldvalue.Value(e).GetByKey("kind").StringValue()
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
