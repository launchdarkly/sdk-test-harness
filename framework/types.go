package framework

import (
	"encoding/json"

	"github.com/launchdarkly/eventsource"
)

type PollingPayload struct {
	Events []PayloadEvent `json:"events"`
}

type PayloadEvent struct {
	Name      string      `json:"name"`
	EventData interface{} `json:"data"`
}

type ChangeSet struct {
	intent *ServerIntent       //nolint:unused
	events []eventsource.Event //nolint:unused
}

type ServerIntent struct {
	Payloads []Payload `json:"payloads"`
}

type Payload struct {
	// The id here doesn't seem to match the state that is included in the
	// payload transferred object.

	// It would be nice if we had the same value available in both so we could
	// use that as the key consistently throughout the process.
	ID     string `json:"id"`
	Target int    `json:"target"`
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// This is the general shape of a put-object event. The delete-object is the same, with the object field being nil.
type BaseObject struct {
	Version int             `json:"version"`
	Kind    string          `json:"kind"`
	Key     string          `json:"key"`
	Object  json.RawMessage `json:"object,omitempty"`
}

type PayloadTransferred struct {
	State   string `json:"state"`
	Version int    `json:"version"`
}

// TODO: Todd doesn't have this in his spec. What are we going to do here?
//
//nolint:godox
type ErrorEvent struct {
	PayloadID string `json:"payloadId"`
	Reason    string `json:"reason"`
}

// type heartBeat struct{}

type Goodbye struct {
	Reason      string `json:"reason"`
	Silent      bool   `json:"silent"`
	Catastrophe bool   `json:"catastrophe"`
	//nolint:godox
	// TODO: Might later include some advice or backoff information
}
