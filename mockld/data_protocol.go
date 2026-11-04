package mockld

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"

	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
)

// DataProtocol is the protocol that the mock streaming and polling services use to deliver flag data.
type DataProtocol string

const (
	// FDv1 is the protocol of the original streaming and polling services. A stream delivers all data in a
	// "put" event, and then delivers changes in "patch" and "delete" events. A poll response contains all
	// data.
	FDv1 DataProtocol = "fdv1"

	// FDv2 is the protocol of the data system. A stream or a poll response delivers a payload as
	// "server-intent", "put-object", "delete-object", and "payload-transferred" events.
	FDv2 DataProtocol = "fdv2"
)

// clientFlagKind is the FDv2 kind of a client-side flag evaluation result.
const clientFlagKind = "flag-eval"

// fdv1Namespace returns the FDv1 namespace, such as "flags", for the FDv2 kind of a server-side data item,
// such as "flag".
func fdv1Namespace(kind string) string {
	switch kind {
	case "flag":
		return "flags"
	case "segment":
		return "segments"
	default:
		panic(fmt.Sprintf("test logic error: FDv1 has no namespace for data kind %q", kind))
	}
}

// requireFDv1FullTransfer panics unless the data is a full data set. FDv1 can send only a full data set
// or a change to a single item, so it cannot represent the other FDv2 intents.
func requireFDv1FullTransfer(data FDv2SDKData) {
	if data.intentCode != "xfer-full" {
		panic(fmt.Sprintf("test logic error: FDv1 cannot represent the FDv2 intent %q", data.intentCode))
	}
}

// fdv1ServerPayload returns server-side data in the FDv1 format: a "flags" map and a "segments" map,
// both keyed by item key.
func fdv1ServerPayload(data SDKData) map[string]map[string]json.RawMessage {
	payload := map[string]map[string]json.RawMessage{
		"flags":    {},
		"segments": {},
	}
	switch d := data.(type) {
	case ServerSDKData:
		for kind, items := range d {
			for key, item := range items {
				payload[fdv1Namespace(string(kind))][key] = item
			}
		}
	case FDv2SDKData:
		requireFDv1FullTransfer(d)
		for _, item := range d.events {
			payload[fdv1Namespace(item.Kind)][item.Key] = item.Object
		}
	default:
		panic(fmt.Sprintf("test logic error: FDv1 server-side data cannot be made from %T", data))
	}
	return payload
}

// fdv1ClientPayload returns client-side data in the FDv1 format: a map of flag keys to flag evaluation
// results.
func fdv1ClientPayload(data SDKData) map[string]json.RawMessage {
	payload := map[string]json.RawMessage{}
	switch d := data.(type) {
	case ClientSDKData:
		for key, flag := range d {
			payload[key] = jsonhelpers.ToJSON(flag)
		}
	case FDv2SDKData:
		requireFDv1FullTransfer(d)
		for _, item := range d.events {
			if item.Kind != clientFlagKind {
				panic(fmt.Sprintf("test logic error: FDv1 client-side data cannot contain data kind %q", item.Kind))
			}
			payload[item.Key] = item.Object
		}
	default:
		panic(fmt.Sprintf("test logic error: FDv1 client-side data cannot be made from %T", data))
	}
	return payload
}

// fdv1Payload returns the data as the body of an FDv1 poll response, which is also the data of an FDv1
// stream "put" event for a client-side SDK. Nil data gives an empty data set. The return value is nil if
// the data is deliberately unavailable.
func fdv1Payload(data SDKData, sdkKind SDKKind) []byte {
	if data == nil {
		data = NewFDv2SDKData("xfer-full", "payload-missing", "initial", nil)
	}
	if data.Serialize() == nil {
		return nil
	}
	if sdkKind.IsServerSide() {
		return jsonhelpers.ToJSON(fdv1ServerPayload(data))
	}
	return jsonhelpers.ToJSON(fdv1ClientPayload(data))
}

// toFDv2SDKData returns the data as a full FDv2 data set. Each server-side item has the version that is
// in its JSON data.
func toFDv2SDKData(data SDKData, state string) FDv2SDKData {
	switch d := data.(type) {
	case FDv2SDKData:
		return d
	case ServerSDKData:
		var items []framework.BaseObject
		for kind, kindItems := range d {
			for key, item := range kindItems {
				var props struct {
					Version int `json:"version"`
				}
				_ = json.Unmarshal(item, &props)
				items = append(items, framework.BaseObject{Kind: string(kind), Key: key, Version: props.Version, Object: item})
			}
		}
		return NewFDv2SDKData("xfer-full", "stale", state, items)
	case ClientSDKData:
		return FDv2SDKDataFromClientSDKData(d, "xfer-full", "stale", state)
	default:
		panic(fmt.Sprintf("test logic error: FDv2 data cannot be made from %T", data))
	}
}
