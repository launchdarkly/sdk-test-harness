package servicedef

import (
	"encoding/json"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
)

type SDKConfigParams struct {
	Credential          string                                      `json:"credential"`
	StartWaitTimeMS     o.Maybe[ldtime.UnixMillisecondTime]         `json:"startWaitTimeMs,omitempty"`
	InitCanFail         bool                                        `json:"initCanFail,omitempty"`
	TLS                 o.Maybe[SDKConfigTLSParams]                 `json:"tls,omitempty"`
	Proxy               o.Maybe[SDKConfigProxyParams]               `json:"proxy,omitempty"`
	Events              o.Maybe[SDKConfigEventParams]               `json:"events,omitempty"`
	BigSegments         o.Maybe[SDKConfigBigSegmentsParams]         `json:"bigSegments,omitempty"`
	Tags                o.Maybe[SDKConfigTagsParams]                `json:"tags,omitempty"`
	ClientSide          o.Maybe[SDKConfigClientSideParams]          `json:"clientSide,omitempty"`
	Hooks               o.Maybe[SDKConfigHooksParams]               `json:"hooks,omitempty"`
	Wrapper             o.Maybe[SDKConfigWrapper]                   `json:"wrapper,omitempty"`
	PersistentDataStore o.Maybe[SDKConfigPersistentDataStoreParams] `json:"persistentDataStore,omitempty"`
	DataSystem          o.Maybe[DataSystem]                         `json:"dataSystem,omitempty"`
	ServiceEndpoints    o.Maybe[SDKConfigServiceEndpointsParams]    `json:"serviceEndpoints,omitempty"`
}

type DataStoreMode int

const (
	// DataStoreModeRead indicates that the data store is read-only. Data will never be written back to the store by
	// the SDK.
	DataStoreModeRead = 0
	// DataStoreModeReadWrite indicates that the data store is read-write. Data from initializers/synchronizers may be
	// written to the store as necessary.
	DataStoreModeReadWrite = 1
)

// DataSystem describes the SDK's data acquisition configuration.
//
// FDv1Fallback configures the SDK's FDv1 Fallback Synchronizer — a polling-only
// data source engaged only in response to a server-directed FDv1 Fallback
// Directive (section 1.6 of the Data System spec). It is architecturally distinct
// from the Primary/Fallback FDv2 synchronizers listed in Synchronizers: those
// handle the heuristic failover described in section 1.2, whereas FDv1Fallback is
// used only on directed fallback and, once engaged, becomes the SDK's sole data
// source for the remainder of its lifetime. Streaming is not a valid transport
// for the FDv1 fallback.
type DataSystem struct {
	UseDefaultDataSystem o.Maybe[bool]                   `json:"useDefaultDataSystem,omitempty"`
	Store                o.Maybe[DataStore]              `json:"store,omitempty"`
	StoreMode            DataStoreMode                   `json:"storeMode"`
	Initializers         []DataInitializer               `json:"initializers"`
	Synchronizers        []DataSynchronizer              `json:"synchronizers"`
	FDv1Fallback         o.Maybe[SDKConfigPollingParams] `json:"fdv1Fallback,omitempty"`
	ConnectionModeConfig o.Maybe[ConnectionModeConfig]   `json:"connectionModeConfig,omitempty"`
}

type ConnectionModeConfig struct {
	InitialConnectionMode o.Maybe[string]                    `json:"initialConnectionMode,omitempty"`
	CustomConnectionModes o.Maybe[map[string]ModeDefinition] `json:"customConnectionModes,omitempty"`
}

type ModeDefinition struct {
	Initializers  []DataInitializer  `json:"initializers"`
	Synchronizers []DataSynchronizer `json:"synchronizers"`
}

type DataStore struct {
	PersistentDataStore o.Maybe[SDKConfigPersistentDataStoreParams] `json:"persistentDataStore,omitempty"`
}

type DataInitializer struct {
	Polling o.Maybe[SDKConfigPollingParams] `json:"polling,omitempty"`
}

type DataSynchronizer struct {
	Streaming o.Maybe[SDKConfigStreamingParams] `json:"streaming,omitempty"`
	Polling   o.Maybe[SDKConfigPollingParams]   `json:"polling,omitempty"`
}

type SDKConfigTLSParams struct {
	SkipVerifyPeer bool   `json:"skipVerifyPeer,omitempty"`
	CustomCAFile   string `json:"customCAFile,omitempty"`
}

type SDKConfigProxyParams struct {
	HTTPProxy o.Maybe[string] `json:"httpProxy,omitempty"`
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
	OmitAnonymousContexts   bool                                `json:"omitAnonymousContexts,omitempty"`
	EnableGzip              o.Maybe[bool]                       `json:"enableGzip,omitempty"`
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
	InitialContext               o.Maybe[ldcontext.Context]        `json:"initialContext,omitempty"`
	InitialUser                  json.RawMessage                   `json:"initialUser,omitempty"`
	EvaluationReasons            o.Maybe[bool]                     `json:"evaluationReasons,omitempty"`
	UseReport                    o.Maybe[bool]                     `json:"useReport,omitempty"`
	IncludeEnvironmentAttributes o.Maybe[bool]                     `json:"includeEnvironmentAttributes,omitempty"`
	Hash                         o.Maybe[string]                   `json:"hash,omitempty"`
	Bootstrap                    o.Maybe[map[string]ldvalue.Value] `json:"bootstrap,omitempty"`
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

type SDKConfigWrapper struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type SDKConfigPersistentDataStoreParams struct {
	Store SDKConfigPersistentStore `json:"store"`
	Cache SDKConfigPersistentCache `json:"cache"`
}

type SDKConfigPersistentType string

const (
	Redis    = SDKConfigPersistentType("redis")
	DynamoDB = SDKConfigPersistentType("dynamodb")
	Consul   = SDKConfigPersistentType("consul")
)

type SDKConfigPersistentStore struct {
	Type   SDKConfigPersistentType `json:"type"`
	Prefix o.Maybe[string]         `json:"prefix,omitempty"`
	DSN    string                  `json:"dsn"`
}

type SDKConfigPersistentMode string

const (
	CacheModeOff      = SDKConfigPersistentMode("off")
	CacheModeTTL      = SDKConfigPersistentMode("ttl")
	CacheModeInfinite = SDKConfigPersistentMode("infinite")
)

type SDKConfigPersistentCache struct {
	Mode SDKConfigPersistentMode `json:"mode"`

	// This value is only valid when the Mode is set to TTL. It must be a positive integer.
	TTL o.Maybe[int] `json:"ttl,omitempty"`
}
