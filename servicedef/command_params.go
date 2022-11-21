package servicedef

import (
	"encoding/json"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

const (
	CommandEvaluateFlag             = "evaluate"
	CommandEvaluateAllFlags         = "evaluateAll"
	CommandIdentifyEvent            = "identifyEvent"
	CommandCustomEvent              = "customEvent"
	CommandAliasEvent               = "aliasEvent"
	CommandFlushEvents              = "flushEvents"
	CommandGetBigSegmentStoreStatus = "getBigSegmentStoreStatus"
	CommandContextBuild             = "contextBuild"
	CommandContextConvert           = "contextConvert"
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
	ContextBuild   o.Maybe[ContextBuildParams]     `json:"contextBuild,omitempty"`
	ContextConvert o.Maybe[ContextConvertParams]   `json:"contextConvert,omitempty"`
	SecureModeHash o.Maybe[SecureModeHashParams]   `json:"secureModeHash,omitempty"`
}

type EvaluateFlagParams struct {
	FlagKey      string                     `json:"flagKey"`
	Context      o.Maybe[ldcontext.Context] `json:"context,omitempty"`
	User         json.RawMessage            `json:"user,omitempty"`
	ValueType    ValueType                  `json:"valueType"`
	DefaultValue ldvalue.Value              `json:"defaultValue"`
	Detail       bool                       `json:"detail"`
}

type EvaluateFlagResponse struct {
	Value          ldvalue.Value                      `json:"value"`
	VariationIndex o.Maybe[int]                       `json:"variationIndex,omitempty"`
	Reason         o.Maybe[ldreason.EvaluationReason] `json:"reason,omitempty"`
}

type EvaluateAllFlagsParams struct {
	Context                    o.Maybe[ldcontext.Context] `json:"context,omitempty"`
	User                       json.RawMessage            `json:"user,omitempty"`
	WithReasons                bool                       `json:"withReasons"`
	ClientSideOnly             bool                       `json:"clientSideOnly"`
	DetailsOnlyForTrackedFlags bool                       `json:"detailsOnlyForTrackedFlags"`
}

type EvaluateAllFlagsResponse struct {
	State map[string]ldvalue.Value `json:"state"`
}

type CustomEventParams struct {
	EventKey     string                     `json:"eventKey"`
	Context      o.Maybe[ldcontext.Context] `json:"context,omitempty"`
	User         json.RawMessage            `json:"user,omitempty"`
	Data         ldvalue.Value              `json:"data,omitempty"`
	OmitNullData bool                       `json:"omitNullData"`
	MetricValue  o.Maybe[float64]           `json:"metricValue,omitempty"`
}

type IdentifyEventParams struct {
	Context o.Maybe[ldcontext.Context] `json:"context"`
	User    json.RawMessage            `json:"user,omitempty"`
}

type BigSegmentStoreStatusResponse struct {
	Available bool `json:"available"`
	Stale     bool `json:"stale"`
}

type ContextBuildParams struct {
	Single *ContextBuildSingleParams  `json:"single,omitempty"`
	Multi  []ContextBuildSingleParams `json:"multi,omitempty"`
}

type ContextBuildSingleParams struct {
	Kind      *string                  `json:"kind,omitempty"`
	Key       string                   `json:"key"`
	Name      *string                  `json:"name,omitempty"`
	Anonymous *bool                    `json:"anonymous,omitempty"`
	Private   []string                 `json:"private,omitempty"`
	Custom    map[string]ldvalue.Value `json:"custom,omitempty"`
}

type ContextBuildResponse struct {
	Output string `json:"output"`
	Error  string `json:"error"`
}

type ContextConvertParams struct {
	Input string `json:"input"`
}

type SecureModeHashParams struct {
	Context ldcontext.Context `json:"context"`
	User    json.RawMessage   `json:"user,omitempty"`
}

type SecureModeHashResponse struct {
	Result string `json:"result"`
}
