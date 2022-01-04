package servicedef

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

const (
	CommandEvaluateFlag     = "evaluate"
	CommandEvaluateAllFlags = "evaluateAll"
	CommandIdentifyEvent    = "identifyEvent"
	CommandCustomEvent      = "customEvent"
	CommandFlushEvents      = "flushEvents"
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
	Command       string                  `json:"command"`
	Evaluate      *EvaluateFlagParams     `json:"evaluate,omitempty"`
	EvaluateAll   *EvaluateAllFlagsParams `json:"evaluateAll,omitempty"`
	CustomEvent   *CustomEventParams      `json:"customEvent,omitempty"`
	IdentifyEvent *IdentifyEventParams    `json:"identifyEvent,omitempty"`
}

type EvaluateFlagParams struct {
	FlagKey      string        `json:"flagKey"`
	User         *lduser.User  `json:"user,omitempty"`
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
	User *lduser.User `json:"user,omitempty"`
}

type EvaluateAllFlagsResponse struct {
	Values map[string]ldvalue.Value `json:"values"`
}

type CustomEventParams struct {
	EventKey     string        `json:"eventKey"`
	User         *lduser.User  `json:"user,omitempty"`
	Data         ldvalue.Value `json:"data,omitempty"`
	OmitNullData bool          `json:"omitNullData"`
	MetricValue  *float64      `json:"metricValue,omitempty"`
}

type IdentifyEventParams struct {
	User lduser.User `json:"user"`
}
