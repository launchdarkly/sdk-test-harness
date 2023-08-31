package mockld

import (
	"encoding/json"
	"fmt"

	"golang.org/x/exp/maps"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
)

type SDKKind string

const (
	ServerSideSDK SDKKind = "server"
	MobileSDK     SDKKind = "mobile"
	JSClientSDK   SDKKind = "jsclient"
	PHPSDK        SDKKind = "php"
	RokuSDK       SDKKind = "roku"
)

func (k SDKKind) IsServerSide() bool {
	return k == ServerSideSDK || k == PHPSDK
}

func (k SDKKind) IsClientSide() bool {
	return !k.IsServerSide()
}

type DataItemKind string

type SDKData interface {
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
//
// We use this for both regular server-side SDKs and the PHP SDK.
type ServerSDKData map[DataItemKind]map[string]json.RawMessage

// ClientSDKData contains simulated LaunchDarkly environment data for a client-side SDK.
//
// This does not include flag or segment configurations, but only flag evaluation results for a specific user.
type ClientSDKData map[string]ClientSDKFlag

// ClientSDKFlag contains the flag evaluation results for a single flag in ClientSDKData.
type ClientSDKFlag struct {
	Value                ldvalue.Value                       `json:"value"`
	Variation            o.Maybe[int]                        `json:"variation"`
	Reason               o.Maybe[ldreason.EvaluationReason]  `json:"reason"`
	Version              int                                 `json:"version"`
	FlagVersion          o.Maybe[int]                        `json:"flagVersion"`
	TrackEvents          bool                                `json:"trackEvents"`
	TrackReason          bool                                `json:"trackReason"`
	DebugEventsUntilDate o.Maybe[ldtime.UnixMillisecondTime] `json:"debugEventsUntilDate"`
}

// ClientSDKFlagWithKey is used only in stream updates, where the key is within the same object.
type ClientSDKFlagWithKey struct {
	ClientSDKFlag
	Key string `json:"key"`
}

func EmptyServerSDKData() ServerSDKData {
	return NewServerSDKDataBuilder().Build() // ensures that "flags" and "segments" properties are present, but empty
}

func EmptyClientSDKData() ClientSDKData {
	return NewClientSDKDataBuilder().Build()
}

func EmptyData(sdkKind SDKKind) SDKData {
	return h.IfElse[SDKData](sdkKind == ServerSideSDK, EmptyServerSDKData(), EmptyClientSDKData())
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
	flags           map[string]json.RawMessage
	segments        map[string]json.RawMessage
	configOverrides map[string]json.RawMessage
	metrics         map[string]json.RawMessage
}

func NewServerSDKDataBuilder() *ServerSDKDataBuilder {
	return &ServerSDKDataBuilder{
		flags:           make(map[string]json.RawMessage),
		segments:        make(map[string]json.RawMessage),
		configOverrides: make(map[string]json.RawMessage),
		metrics:         make(map[string]json.RawMessage),
	}
}

func (b *ServerSDKDataBuilder) Build() ServerSDKData {
	flags := maps.Clone(b.flags)
	segments := maps.Clone(b.segments)
	configOverrides := maps.Clone(b.configOverrides)
	metrics := maps.Clone(b.metrics)

	return map[DataItemKind]map[string]json.RawMessage{
		"flags":                  flags,
		"segments":               segments,
		"configurationOverrides": configOverrides,
		"metrics":                metrics,
	}
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

func (b *ServerSDKDataBuilder) RawConfigOverride(key string, data json.RawMessage) *ServerSDKDataBuilder {
	b.configOverrides[key] = data
	return b
}

func (b *ServerSDKDataBuilder) ConfigOverride(overrides ...ldmodel.ConfigOverride) *ServerSDKDataBuilder {
	for _, override := range overrides {
		b = b.RawConfigOverride(override.Key, jsonhelpers.ToJSON(override))
	}
	return b
}

func (b *ServerSDKDataBuilder) RawMetric(key string, data json.RawMessage) *ServerSDKDataBuilder {
	b.metrics[key] = data
	return b
}

func (b *ServerSDKDataBuilder) Metric(metrics ...ldmodel.Metric) *ServerSDKDataBuilder {
	for _, metric := range metrics {
		b = b.RawMetric(metric.Key, jsonhelpers.ToJSON(metric))
	}
	return b
}

func (d ClientSDKData) Serialize() []byte {
	return jsonhelpers.ToJSON(d)
}

func (d ClientSDKData) JSONString() string {
	return jsonhelpers.ToJSONString(d)
}

func (d ClientSDKData) WithoutReasons() ClientSDKData {
	ret := make(ClientSDKData)
	for k, v := range d {
		v.Reason = o.None[ldreason.EvaluationReason]()
		ret[k] = v
	}
	return ret
}

type ClientSDKDataBuilder struct {
	flags map[string]ClientSDKFlag
}

func NewClientSDKDataBuilder() *ClientSDKDataBuilder {
	return &ClientSDKDataBuilder{
		flags: make(map[string]ClientSDKFlag),
	}
}

func (b *ClientSDKDataBuilder) Build() ClientSDKData {
	ret := make(ClientSDKData)
	for k, v := range b.flags {
		ret[k] = v
	}
	return ret
}

func (b *ClientSDKDataBuilder) Flag(key string, props ClientSDKFlag) *ClientSDKDataBuilder {
	b.flags[key] = props
	return b
}

func (b *ClientSDKDataBuilder) FullFlag(props ClientSDKFlagWithKey) *ClientSDKDataBuilder {
	b.flags[props.Key] = props.ClientSDKFlag
	return b
}

func (b *ClientSDKDataBuilder) FlagWithValue(
	key string,
	version int,
	value ldvalue.Value,
	variationIndex int,
) *ClientSDKDataBuilder {
	return b.Flag(key, ClientSDKFlag{Version: version, Value: value, Variation: o.Some(variationIndex)})
}
