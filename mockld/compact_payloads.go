package mockld

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-jsonstream/v3/jwriter"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

// The query parameter that an FDv2 SDK adds to its requests to ask for compact payloads.
const (
	CompactPayloadsQueryParam = "enc"
	CompactPayloadsQueryValue = "compact"
)

// asksForCompactPayloads tells if an FDv2 request asks for compact payloads. LaunchDarkly sends
// compact payloads only to a request that asks for them, and so do the mock services.
func asksForCompactPayloads(r *http.Request) bool {
	return r.URL.Query().Get(CompactPayloadsQueryParam) == CompactPayloadsQueryValue
}

// compactPutObject returns obj with its object in the compact encoding.
func compactPutObject(obj framework.BaseObject) framework.BaseObject {
	if obj.Object == nil {
		return obj
	}
	obj.Object = CompactObjectJSON(obj.Kind, obj.Object)
	return obj
}

// compactPutObjectIf returns obj with its object in the compact encoding if compact is true.
// Otherwise it returns obj without changes.
func compactPutObjectIf(compact bool, obj framework.BaseObject) framework.BaseObject {
	if !compact {
		return obj
	}
	return compactPutObject(obj)
}

// CompactObjectJSON returns the compact encoding of the JSON for an FDv2 object of the given kind.
//
// For a flag or a segment, the compact encoding omits each property whose absence an SDK must read
// as the same value. This is the encoding that LaunchDarkly sends to SDKs that ask for compact
// payloads. For a client-side flag evaluation, the compact encoding omits null values, false
// booleans, and an empty prerequisites list. It always keeps value, variation, and flagVersion,
// even if they are null.
//
// The function returns data without changes if data is not the full encoding of a valid object.
// Thus the deliberately malformed data in some tests gets to the SDK as the test wrote it.
func CompactObjectJSON(kind string, data json.RawMessage) json.RawMessage {
	switch kind {
	case "flag":
		return compactFlagJSON(data)
	case "segment":
		return compactSegmentJSON(data)
	case "flag-eval":
		return compactFlagEvalJSON(data)
	default:
		return data
	}
}

func compactFlagJSON(data json.RawMessage) json.RawMessage {
	reader := jreader.NewReader(data)
	flag := ldmodel.UnmarshalFeatureFlagFromJSONReader(&reader)
	if reader.Error() != nil {
		return data
	}
	full, err := ldmodel.NewJSONDataModelSerialization().MarshalFeatureFlag(flag)
	if err != nil || !jsonEqual(full, data) {
		return data
	}
	writer := jwriter.NewWriter()
	ldmodel.MarshalFeatureFlagToJSONWriterCompact(flag, &writer)
	if writer.Error() != nil {
		return data
	}
	return writer.Bytes()
}

func compactSegmentJSON(data json.RawMessage) json.RawMessage {
	reader := jreader.NewReader(data)
	segment := ldmodel.UnmarshalSegmentFromJSONReader(&reader)
	if reader.Error() != nil {
		return data
	}
	full, err := ldmodel.NewJSONDataModelSerialization().MarshalSegment(segment)
	if err != nil || !jsonEqual(full, data) {
		return data
	}
	writer := jwriter.NewWriter()
	ldmodel.MarshalSegmentToJSONWriterCompact(segment, &writer)
	if writer.Error() != nil {
		return data
	}
	return writer.Bytes()
}

// flagEvalNullableProperties are the flag evaluation properties that an SDK reads as null when
// they are absent. The compact encoding always writes value, variation, and flagVersion, so they
// are not in this list.
var flagEvalNullableProperties = []string{ //nolint:gochecknoglobals // constant list
	"reason", "samplingRatio", "debugEventsUntilDate", "prerequisites",
}

// flagEvalBooleanProperties are the flag evaluation properties that an SDK reads as false when
// they are absent.
var flagEvalBooleanProperties = []string{ //nolint:gochecknoglobals // constant list
	"trackEvents", "trackReason",
}

func compactFlagEvalJSON(data json.RawMessage) json.RawMessage {
	var props map[string]json.RawMessage
	if err := json.Unmarshal(data, &props); err != nil || props == nil {
		return data
	}
	for _, name := range flagEvalNullableProperties {
		if value, ok := props[name]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			delete(props, name)
		}
	}
	if value, ok := props["prerequisites"]; ok && jsonEqual(value, []byte("[]")) {
		delete(props, "prerequisites")
	}
	for _, name := range flagEvalBooleanProperties {
		if value, ok := props[name]; ok && bytes.Equal(bytes.TrimSpace(value), []byte("false")) {
			delete(props, name)
		}
	}
	compact, err := json.Marshal(props)
	if err != nil {
		return data
	}
	return compact
}

// jsonEqual tells if a and b are the same JSON value.
func jsonEqual(a, b []byte) bool {
	var aValue, bValue any
	if json.Unmarshal(a, &aValue) != nil || json.Unmarshal(b, &bValue) != nil {
		return false
	}
	return reflect.DeepEqual(aValue, bValue)
}
