package servicedef

import (
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

const (
	CommandEvaluateFlag             = "evaluate"
	CommandEvaluateAllFlags         = "evaluateAll"
	CommandIdentifyEvent            = "identifyEvent"
	CommandCustomEvent              = "customEvent"
	CommandAliasEvent               = "aliasEvent"
	CommandFlushEvents              = "flushEvents"
	CommandGetBigSegmentStoreStatus = "getBigSegmentStoreStatus"
	CommandSecureModeHash           = "secureModeHash"
)

type ValueType string

const (
	ValueTypeBool   = "bool"
	ValueTypeInt    = "int"
	ValueTypeDouble = "double"
	ValueTypeString = "string"
	ValueTypeAny    = "any"
)

type CommandParams struct {
	Command        string                          `json:"command"`
	Evaluate       o.Maybe[EvaluateFlagParams]     `json:"evaluate,omitempty"`
	EvaluateAll    o.Maybe[EvaluateAllFlagsParams] `json:"evaluateAll,omitempty"`
	CustomEvent    o.Maybe[CustomEventParams]      `json:"customEvent,omitempty"`
	IdentifyEvent  o.Maybe[IdentifyEventParams]    `json:"identifyEvent,omitempty"`
	AliasEvent     o.Maybe[AliasEventParams]       `json:"aliasEvent,omitempty"`
	SecureModeHash o.Maybe[SecureModeHashParams]   `json:"secureModeHash,omitempty"`
}

type EvaluateFlagParams struct {
	FlagKey      string               `json:"flagKey"`
	User         o.Maybe[lduser.User] `json:"user"`
	ValueType    ValueType            `json:"valueType"`
	DefaultValue ldvalue.Value        `json:"defaultValue"`
	Detail       bool                 `json:"detail"`
}

type EvaluateFlagResponse struct {
	Value          ldvalue.Value                      `json:"value"`
	VariationIndex o.Maybe[int]                       `json:"variationIndex,omitempty"`
	Reason         o.Maybe[ldreason.EvaluationReason] `json:"reason,omitempty"`
}

type EvaluateAllFlagsParams struct {
	User                       o.Maybe[lduser.User] `json:"user,omitempty"`
	WithReasons                bool                 `json:"withReasons"`
	ClientSideOnly             bool                 `json:"clientSideOnly"`
	DetailsOnlyForTrackedFlags bool                 `json:"detailsOnlyForTrackedFlags"`
}

type EvaluateAllFlagsResponse struct {
	State map[string]ldvalue.Value `json:"state"`
}

type CustomEventParams struct {
	EventKey     string               `json:"eventKey"`
	User         o.Maybe[lduser.User] `json:"user"`
	Data         ldvalue.Value        `json:"data,omitempty"`
	OmitNullData bool                 `json:"omitNullData"`
	MetricValue  o.Maybe[float64]     `json:"metricValue,omitempty"`
}

type IdentifyEventParams struct {
	User lduser.User `json:"user"`
}

type AliasEventParams struct {
	User         lduser.User `json:"user"`
	PreviousUser lduser.User `json:"previousUser"`
}

type BigSegmentStoreStatusResponse struct {
	Available bool `json:"available"`
	Stale     bool `json:"stale"`
}

type SecureModeHashParams struct {
	User lduser.User `json:"user"`
}

type SecureModeHashResponse struct {
	Result string `json:"result"`
}
