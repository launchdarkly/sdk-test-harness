package servicedef

import (
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
)

type SDKConfigParams struct {
	Credential          string                                      `json:"credential"`
	StartWaitTimeMS     o.Maybe[ldtime.UnixMillisecondTime]         `json:"startWaitTimeMs,omitempty"`
	InitCanFail         bool                                        `json:"initCanFail,omitempty"`
	ServiceEndpoints    o.Maybe[SDKConfigServiceEndpointsParams]    `json:"serviceEndpoints,omitempty"`
	Streaming           o.Maybe[SDKConfigStreamingParams]           `json:"streaming,omitempty"`
	Polling             o.Maybe[SDKConfigPollingParams]             `json:"polling,omitempty"`
	Events              o.Maybe[SDKConfigEventParams]               `json:"events,omitempty"`
	PersistentDataStore o.Maybe[SDKConfigPersistentDataStoreParams] `json:"persistentDataStore,omitempty"`
	BigSegments         o.Maybe[SDKConfigBigSegmentsParams]         `json:"bigSegments,omitempty"`
	Tags                o.Maybe[SDKConfigTagsParams]                `json:"tags,omitempty"`
	ClientSide          o.Maybe[SDKConfigClientSideParams]          `json:"clientSide,omitempty"`
}

type SDKConfigServiceEndpointsParams struct {
	Streaming string `json:"streaming,omitempty"`
	Polling   string `json:"polling,omitempty"`
	Events    string `json:"events,omitempty"`
}

type SDKConfigStreamingParams struct {
	BaseURI             string                              `json:"baseUri,omitempty"`
	InitialRetryDelayMS o.Maybe[ldtime.UnixMillisecondTime] `json:"initialRetryDelayMs,omitempty"`
}

type SDKConfigPollingParams struct {
	BaseURI        string                              `json:"baseUri,omitempty"`
	PollIntervalMS o.Maybe[ldtime.UnixMillisecondTime] `json:"pollIntervalMs,omitempty"`
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
	InitialContext    ldcontext.Context `json:"initialContext"`
	EvaluationReasons o.Maybe[bool]     `json:"evaluationReasons,omitempty"`
	UseReport         o.Maybe[bool]     `json:"useReport,omitempty"`
}
