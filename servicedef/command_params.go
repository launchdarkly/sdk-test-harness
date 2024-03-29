package servicedef

import (
	"encoding/json"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
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
	CommandContextComparison        = "contextComparison"
	CommandSecureModeHash           = "secureModeHash"
	CommandMigrationVariation       = "migrationVariation"
	CommandMigrationOperation       = "migrationOperation"
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
	Command            string                               `json:"command"`
	Evaluate           o.Maybe[EvaluateFlagParams]          `json:"evaluate,omitempty"`
	EvaluateAll        o.Maybe[EvaluateAllFlagsParams]      `json:"evaluateAll,omitempty"`
	CustomEvent        o.Maybe[CustomEventParams]           `json:"customEvent,omitempty"`
	IdentifyEvent      o.Maybe[IdentifyEventParams]         `json:"identifyEvent,omitempty"`
	ContextBuild       o.Maybe[ContextBuildParams]          `json:"contextBuild,omitempty"`
	ContextConvert     o.Maybe[ContextConvertParams]        `json:"contextConvert,omitempty"`
	ContextComparison  o.Maybe[ContextComparisonPairParams] `json:"contextComparison,omitempty"`
	SecureModeHash     o.Maybe[SecureModeHashParams]        `json:"secureModeHash,omitempty"`
	MigrationVariation o.Maybe[MigrationVariationParams]    `json:"migrationVariation,omitempty"`
	MigrationOperation o.Maybe[MigrationOperationParams]    `json:"migrationOperation,omitempty"`
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

type ContextComparisonPairParams struct {
	Context1 ContextComparisonParams `json:"context1"`
	Context2 ContextComparisonParams `json:"context2"`
}

type ContextComparisonParams struct {
	Single *ContextComparisonSingleParams  `json:"single,omitempty"`
	Multi  []ContextComparisonSingleParams `json:"multi,omitempty"`
}

type ContextComparisonSingleParams struct {
	Kind              string                `json:"kind"`
	Key               string                `json:"key"`
	Attributes        []AttributeDefinition `json:"attributes,omitempty"`
	PrivateAttributes []PrivateAttribute    `json:"privateAttributes:omitempty"`
}

type AttributeDefinition struct {
	Name  string        `json:"name"`
	Value ldvalue.Value `json:"value,omitempty"`
}

type PrivateAttribute struct {
	Value   string `json:"value"`
	Literal bool   `json:"literal"`
}

type ContextComparisonResponse struct {
	Equals bool `json:"equals"`
}

type SecureModeHashParams struct {
	Context o.Maybe[ldcontext.Context] `json:"context,omitempty"`
	User    json.RawMessage            `json:"user,omitempty"`
}

type SecureModeHashResponse struct {
	Result string `json:"result"`
}

type MigrationVariationParams struct {
	Key          string            `json:"key"`
	Context      ldcontext.Context `json:"context"`
	DefaultStage ldmigration.Stage `json:"defaultStage"`
}

type MigrationVariationResponse struct {
	Result string `json:"result"`
}

type MigrationOperationParams struct {
	Key                string                     `json:"key"`
	Context            ldcontext.Context          `json:"context"`
	DefaultStage       ldmigration.Stage          `json:"defaultStage"`
	ReadExecutionOrder ldmigration.ExecutionOrder `json:"readExecutionOrder"`
	Operation          ldmigration.Operation      `json:"operation"`
	OldEndpoint        string                     `json:"oldEndpoint"`
	NewEndpoint        string                     `json:"newEndpoint"`
	Payload            o.Maybe[string]            `json:"payload,omitempty"`
	TrackLatency       bool                       `json:"trackLatency"`
	TrackErrors        bool                       `json:"trackErrors"`
	TrackConsistency   bool                       `json:"trackConsistency"`
}

type MigrationOperationResponse struct {
	Result string `json:"result"`
}

type HookStage string

const (
	BeforeEvaluation HookStage = "beforeEvaluation"
	AfterEvaluation  HookStage = "afterEvaluation"
)

type EvaluationSeriesContext struct {
	FlagKey      string            `json:"flagKey"`
	Context      ldcontext.Context `json:"context"`
	DefaultValue ldvalue.Value     `json:"defaultValue"`
	Method       string            `json:"method"`
}

type HookExecutionPayload struct {
	EvaluationSeriesContext o.Maybe[EvaluationSeriesContext]  `json:"evaluationSeriesContext"`
	EvaluationSeriesData    o.Maybe[map[string]ldvalue.Value] `json:"evaluationSeriesData"`
	EvaluationDetail        o.Maybe[EvaluateFlagResponse]     `json:"evaluationDetail"`
	Stage                   o.Maybe[HookStage]                `json:"stage"`
}
