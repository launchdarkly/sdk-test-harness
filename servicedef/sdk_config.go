package servicedef

import (
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
)

type SDKConfigParams struct {
	Credential          string                                      `json:"credential"`
	StartWaitTimeMS     o.Maybe[ldtime.UnixMillisecondTime]         `json:"startWaitTimeMs,omitempty"`
	InitCanFail         bool                                        `json:"initCanFail,omitempty"`
	ServiceEndpoints    o.Maybe[SDKConfigServiceEndpointsParams]    `json:"serviceEndpoints,omitempty"`
	Streaming           o.Maybe[SDKConfigStreamingParams]           `json:"streaming,omitempty"`
	Polling             o.Maybe[SDKConfigPollingParams]             `json:"polling,omitempty"`
	ExternalUpdatesOnly bool                                        `json:"externalUpdatesOnly,omitempty"`
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
	GlobalPrivateAttributes []lduser.UserAttribute              `json:"globalPrivateAttributes,omitempty"`
	FlushIntervalMS         o.Maybe[ldtime.UnixMillisecondTime] `json:"flushIntervalMs,omitempty"`
	InlineUsers             bool                                `json:"inlineUsers,omitempty"`
}

type SDKConfigPersistentDataStoreParams struct {
	Integration SDKConfigStoreIntegrationParams     `json:"integration"`
	CacheTime   o.Maybe[ldtime.UnixMillisecondTime] `json:"cacheTimeMs"`
}

type SDKConfigBigSegmentsParams struct {
	CallbackURI          string                              `json:"callbackUri"` // deprecated, use "fixture" instead
	Integration          SDKConfigStoreIntegrationParams     `json:"integration"`
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
	InitialUser        lduser.User   `json:"initialUser"`
	AutoAliasingOptOut o.Maybe[bool] `json:"autoAliasingOptOut,omitempty"`
	EvaluationReasons  o.Maybe[bool] `json:"evaluationReasons,omitempty"`
	UseReport          o.Maybe[bool] `json:"useReport,omitempty"`
}

type SDKConfigStoreIntegrationParams struct {
	Type     string                        `json:"type"`
	Fixture  *SDKConfigFixtureParams       `json:"fixture"`
	Redis    *SDKConfigRedisStoreParams    `json:"redis"`
	DynamoDB *SDKConfigDynamoDBStoreParams `json:"dynamoDb"`
	Consul   *SDKConfigConsulStoreParams   `json:"consul"`
}

type SDKConfigFixtureParams struct {
	URI string `json:"uri"`
}

type SDKConfigRedisStoreParams struct {
	URL    string `json:"url"`
	Prefix string `json:"prefix"`
}

type SDKConfigDynamoDBStoreParams struct {
	Table  string `json:"table"`
	Prefix string `json:"prefix"`
}

type SDKConfigConsulStoreParams struct {
	Address string `json:"address"`
	Prefix  string `json:"prefix"`
}
