package mockld

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/go-jsonstream/v2/jreader"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldmodel"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
)

type SDKKind string

const (
	ServerSideSDK SDKKind = "server"
	MobileSDK     SDKKind = "mobile"
	JSClientSDK   SDKKind = "jsclient"
)

type DataItemKind string

type SDKData interface {
	SDKKind() SDKKind
	Serialize() []byte
}

type blockingUnavailableSDKData struct {
	kind SDKKind
}

// BlockingUnavailableSDKData returns an object that will cause the mock streaming service *not* to provide
// any data. It will accept connections, but then hang. This allows us to simulate a state where the SDK
// client times out without initializing, because it has not received any "put" event.
func BlockingUnavailableSDKData(sdkKind SDKKind) SDKData {
	return blockingUnavailableSDKData{kind: sdkKind}
}

func (b blockingUnavailableSDKData) SDKKind() SDKKind  { return b.kind }
func (b blockingUnavailableSDKData) Serialize() []byte { return nil }

// ServerSDKData contains simulated LaunchDarkly environment data for a server-side SDK.
//
// This includes the full JSON configuration of every flag and segment, in the same format that is used in
// streaming and polling responses.
type ServerSDKData map[DataItemKind]map[string]json.RawMessage

func EmptyServerSDKData() ServerSDKData {
	return NewServerSDKDataBuilder().Build() // ensures that "flags" and "segments" properties are present, but empty
}

func (d ServerSDKData) SDKKind() SDKKind {
	return ServerSideSDK
}

func (d ServerSDKData) Serialize() []byte {
	data, _ := json.Marshal(d)
	return data
}

func (d ServerSDKData) JSONString() string {
	s, _ := json.Marshal(d)
	return string(s)
}

func (d *ServerSDKData) UnmarshalJSON(data []byte) error {
	var originalMap map[DataItemKind]map[string]json.RawMessage
	if err := json.Unmarshal(data, &originalMap); err != nil {
		return err
	}
	// Validate that the SDK data has no schema errors. The ldmodel package provides JSON unmarshalling for
	// these data types. We could use json.Unmarshal, but using the "FromJSONReader" functions gives us better
	// error messages.
	// In the future, we may want to allow deliberately providing malformed SDK data for a test. But normally
	// we want to catch any such errors before we try running the test.
	builder := NewServerSDKDataBuilder()
	for key, data := range originalMap["flags"] {
		newData, err := normalizeFlag(key, data)
		if err != nil {
			return err
		}
		builder.RawFlag(key, newData)
	}
	for key, data := range originalMap["segments"] {
		newData, err := normalizeSegment(key, data)
		if err != nil {
			return err
		}
		builder.RawSegment(key, newData)
	}
	*d = builder.Build()
	return nil
}

func normalizeFlag(key string, data json.RawMessage) (json.RawMessage, error) {
	var reader = jreader.NewReader(data)
	flag := ldmodel.UnmarshalFeatureFlagFromJSONReader(&reader)
	if reader.Error() != nil {
		return nil, fmt.Errorf("malformed JSON for flag %q: %s -- JSON data follows: %s", key,
			reader.Error(), string(data))
	}
	flag.Key = key
	if flag.Version == 0 {
		flag.Version = 1
	}
	json, _ := json.Marshal(flag)
	return json, nil
}

func normalizeSegment(key string, data json.RawMessage) (json.RawMessage, error) {
	var reader = jreader.NewReader(data)
	segment := ldmodel.UnmarshalSegmentFromJSONReader(&reader)
	if reader.Error() != nil {
		return nil, fmt.Errorf("malformed JSON for segment %q: %s -- JSON data follows: %s", key,
			reader.Error(), string(data))
	}
	segment.Key = key
	if segment.Version == 0 {
		segment.Version = 1
	}
	json, _ := json.Marshal(segment)
	return json, nil
}

type ServerSDKDataBuilder struct {
	flags    map[string]json.RawMessage
	segments map[string]json.RawMessage
}

func NewServerSDKDataBuilder() *ServerSDKDataBuilder {
	return &ServerSDKDataBuilder{
		flags:    make(map[string]json.RawMessage),
		segments: make(map[string]json.RawMessage),
	}
}

func (b *ServerSDKDataBuilder) Build() ServerSDKData {
	flags := make(map[string]json.RawMessage)
	segments := make(map[string]json.RawMessage)
	for k, v := range b.flags {
		flags[k] = v
	}
	for k, v := range b.segments {
		segments[k] = v
	}
	return map[DataItemKind]map[string]json.RawMessage{"flags": flags, "segments": segments}
}

func (b *ServerSDKDataBuilder) RawFlag(key string, data json.RawMessage) *ServerSDKDataBuilder {
	b.flags[key] = data
	return b
}

func (b *ServerSDKDataBuilder) Flag(flags ...ldmodel.FeatureFlag) *ServerSDKDataBuilder {
	for _, flag := range flags {
		b = b.RawFlag(flag.Key, jsonhelpers.ToJSON(flag))
	}
	return b
}

func (b *ServerSDKDataBuilder) RawSegment(key string, data json.RawMessage) *ServerSDKDataBuilder {
	b.segments[key] = data
	return b
}

func (b *ServerSDKDataBuilder) Segment(segments ...ldmodel.Segment) *ServerSDKDataBuilder {
	for _, segment := range segments {
		b = b.RawSegment(segment.Key, jsonhelpers.ToJSON(segment))
	}
	return b
}
