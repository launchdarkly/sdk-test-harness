package servicedef

import (
	"encoding/json"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
)

type SDKConfigParams struct {
	Credential          string                                      `json:"credential"`
	StartWaitTimeMS     o.Maybe[ldtime.UnixMillisecondTime]         `json:"startWaitTimeMs,omitempty"`
	InitCanFail         bool                                        `json:"initCanFail,omitempty"`
	ServiceEndpoints    o.Maybe[SDKConfigServiceEndpointsParams]    `json:"serviceEndpoints,omitempty"`
	TLS                 o.Maybe[SDKConfigTLSParams]                 `json:"tls,omitempty"`
	Streaming           o.Maybe[SDKConfigStreamingParams]           `json:"streaming,omitempty"`
	Polling             o.Maybe[SDKConfigPollingParams]             `json:"polling,omitempty"`
	Events              o.Maybe[SDKConfigEventParams]               `json:"events,omitempty"`
	PersistentDataStore o.Maybe[SDKConfigPersistentDataStoreParams] `json:"persistentDataStore,omitempty"`
	BigSegments         o.Maybe[SDKConfigBigSegmentsParams]         `json:"bigSegments,omitempty"`
	Tags                o.Maybe[SDKConfigTagsParams]                `json:"tags,omitempty"`
	ClientSide          o.Maybe[SDKConfigClientSideParams]          `json:"clientSide,omitempty"`
	Hooks               o.Maybe[SDKConfigHooksParams]               `json:"hooks,omitempty"`
}

type SDKConfigTLSParams struct {
	VerifyPeer               bool   `json:"verifyPeer,omitempty"`
	CertificateAuthorityPath string `json:"certificateAuthorityPath,omitempty"`
}
type SDKConfigServiceEndpointsParams struct {
	Streaming string `json:"streaming,omitempty"`
	Polling   string `json:"polling,omitempty"`
	Events    string `json:"events,omitempty"`
}

type SDKConfigStreamingParams struct {
	BaseURI             string                              `json:"baseUri,omitempty"`
	InitialRetryDelayMS o.Maybe[ldtime.UnixMillisecondTime] `json:"initialRetryDelayMs,omitempty"`
	Filter              o.Maybe[string]                     `json:"filter,omitempty"`
}

type SDKConfigPollingParams struct {
	BaseURI        string                              `json:"baseUri,omitempty"`
	PollIntervalMS o.Maybe[ldtime.UnixMillisecondTime] `json:"pollIntervalMs,omitempty"`
	Filter         o.Maybe[string]                     `json:"filter,omitempty"`
}

type SDKConfigEventParams struct {
	BaseURI                 string                              `json:"baseUri,omitempty"`
	Capacity                o.Maybe[int]                        `json:"capacity,omitempty"`
	EnableDiagnostics       bool                                `json:"enableDiagnostics"`
	AllAttributesPrivate    bool                                `json:"allAttributesPrivate,omitempty"`
	GlobalPrivateAttributes []string                            `json:"globalPrivateAttributes,omitempty"`
	FlushIntervalMS         o.Maybe[ldtime.UnixMillisecondTime] `json:"flushIntervalMs,omitempty"`
}

type SDKConfigPersistentDataStoreParams struct {
	CallbackURI string `json:"callbackURI"`
}

type SDKConfigBigSegmentsParams struct {
	CallbackURI          string                              `json:"callbackUri"`
	UserCacheSize        o.Maybe[int]                        `json:"userCacheSize,omitempty"`
	UserCacheTimeMS      o.Maybe[ldtime.UnixMillisecondTime] `json:"userCacheTimeMs,omitempty"`
	StatusPollIntervalMS o.Maybe[ldtime.UnixMillisecondTime] `json:"statusPollIntervalMs,omitempty"`
	StaleAfterMS         o.Maybe[ldtime.UnixMillisecondTime] `json:"staleAfterMs,omitempty"`
}

type SDKConfigTagsParams struct {
	ApplicationID      o.Maybe[string] `json:"applicationId,omitempty"`
	ApplicationVersion o.Maybe[string] `json:"applicationVersion,omitempty"`
}

type SDKConfigClientSideParams struct {
	InitialContext               o.Maybe[ldcontext.Context] `json:"initialContext,omitempty"`
	InitialUser                  json.RawMessage            `json:"initialUser,omitempty"`
	EvaluationReasons            o.Maybe[bool]              `json:"evaluationReasons,omitempty"`
	UseReport                    o.Maybe[bool]              `json:"useReport,omitempty"`
	IncludeEnvironmentAttributes o.Maybe[bool]              `json:"includeEnvironmentAttributes,omitempty"`
}

type SDKConfigEvaluationHookData map[string]ldvalue.Value

type SDKConfigHookInstance struct {
	Name        string                                    `json:"name"`
	CallbackURI string                                    `json:"callbackUri"`
	Data        map[HookStage]SDKConfigEvaluationHookData `json:"data,omitempty"`
	Errors      map[HookStage]o.Maybe[string]             `json:"errors,omitempty"`
}

type SDKConfigHooksParams struct {
	Hooks []SDKConfigHookInstance `json:"hooks"`
}
