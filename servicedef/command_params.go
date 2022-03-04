package servicedef

import (
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
	CommandContextBuild             = "contextBuild"
	CommandContextConvert           = "contextConvert"
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
	Command        string                  `json:"command"`
	Evaluate       *EvaluateFlagParams     `json:"evaluate,omitempty"`
	EvaluateAll    *EvaluateAllFlagsParams `json:"evaluateAll,omitempty"`
	CustomEvent    *CustomEventParams      `json:"customEvent,omitempty"`
	IdentifyEvent  *IdentifyEventParams    `json:"identifyEvent,omitempty"`
	ContextBuild   *ContextBuildParams     `json:"contextBuild,omitempty"`
	ContextConvert *ContextConvertParams   `json:"contextConvert,omitempty"`
}

type EvaluateFlagParams struct {
	FlagKey      string        `json:"flagKey"`
	User         lduser.User   `json:"user"`
	ValueType    ValueType     `json:"valueType"`
	DefaultValue ldvalue.Value `json:"defaultValue"`
	Detail       bool          `json:"detail"`
}

type EvaluateFlagResponse struct {
	Value          ldvalue.Value              `json:"value"`
	VariationIndex *int                       `json:"variationIndex,omitempty"`
	Reason         *ldreason.EvaluationReason `json:"reason,omitempty"`
}

type EvaluateAllFlagsParams struct {
	User                       *lduser.User `json:"user,omitempty"`
	WithReasons                bool         `json:"withReasons"`
	ClientSideOnly             bool         `json:"clientSideOnly"`
	DetailsOnlyForTrackedFlags bool         `json:"detailsOnlyForTrackedFlags"`
}

type EvaluateAllFlagsResponse struct {
	State map[string]ldvalue.Value `json:"state"`
}

type CustomEventParams struct {
	EventKey     string        `json:"eventKey"`
	User         lduser.User   `json:"user"`
	Data         ldvalue.Value `json:"data,omitempty"`
	OmitNullData bool          `json:"omitNullData"`
	MetricValue  *float64      `json:"metricValue,omitempty"`
}

type IdentifyEventParams struct {
	User lduser.User `json:"user"`
}

type BigSegmentStoreStatusResponse struct {
	Available bool `json:"available"`
	Stale     bool `json:"stale"`
}

type ContextBuildParams struct {
	Single *ContextBuildSingleParams `json:"single,omitempty"`
	Multi  *ContextBuildMultiParams  `json:"multi,omitempty"`
}

type ContextBuildSingleParams struct {
	Kind      *string                  `json:"kind,omitempty"`
	Key       string                   `json:"key"`
	Name      *string                  `json:"name,omitempty"`
	Transient *bool                    `json:"transient,omitempty"`
	Secondary *string                  `json:"secondary,omitempty"`
	Private   []string                 `json:"private,omitempty"`
	Custom    map[string]ldvalue.Value `json:"custom,omitempty"`
}

type ContextBuildMultiParams struct {
	Kinds []ContextBuildSingleParams `json:"kinds,omitempty"`
}

type ContextBuildResponse struct {
	Output string `json:"output"`
	Error  string `json:"error"`
}

type ContextConvertParams struct {
	Input string `json:"input"`
}
