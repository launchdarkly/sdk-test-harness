package helpers

import (
	"encoding/json"
	"sort"
	"strings"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

// AsJSON is just a shortcut for calling json.Marshal and taking only the first result.
func AsJSON(value interface{}) []byte {
	ret, _ := json.Marshal(value)
	return ret
}

// AsJSONString calls json.Marshal and returns the result as a string.
func AsJSONString(value interface{}) string { return string(AsJSON(value)) }

// AsJSONValue calls json.Marshal and returns the result as an ldvalue.Value. The Value type
// is often convenient in test code to represent arbitrary JSON data.
func AsJSONValue(value interface{}) ldvalue.Value { return ldvalue.Parse(AsJSON(value)) }

// CanonicalizedJSONString reformats a JSON value so that object properties are alphabetized,
// making it easier for a human reader to find a property.
func CanonicalizedJSONString(value ldvalue.Value) string {
	switch value.Type() {
	case ldvalue.ArrayType:
		items := make([]string, 0, value.Count())
		for i := 0; i < value.Count(); i++ {
			items = append(items, CanonicalizedJSONString(value.GetByIndex((i))))
		}
		return "[" + strings.Join(items, ",") + "]"
	case ldvalue.ObjectType:
		keys := value.Keys()
		sort.Strings(keys)
		items := make([]string, 0, len(keys))
		for _, k := range keys {
			items = append(items, `"`+k+`":`+CanonicalizedJSONString(value.GetByKey(k)))
		}
		return "{" + strings.Join(items, ",") + "}"
	default:
		return value.JSONString()
	}
}
