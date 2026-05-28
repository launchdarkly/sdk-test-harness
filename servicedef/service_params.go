package servicedef

import (
	"github.com/launchdarkly/sdk-test-harness/v2/serviceinfo"
)

const (
	CapabilityClientSide         = "client-side"
	CapabilityServerSide         = "server-side"
	CapabilityStronglyTyped      = "strongly-typed"
	CapabilityMobile             = "mobile"
	CapabilityPHP                = "php"
	CapabilityRoku               = "roku"
	CapabilitySingleton          = "singleton"
	CapabilityClientIndependence = "client-independence"

	CapabilityAllFlagsWithReasons                = "all-flags-with-reasons"
	CapabilityAllFlagsClientSideOnly             = "all-flags-client-side-only"
	CapabilityAllFlagsDetailsOnlyForTrackedFlags = "all-flags-details-only-for-tracked-flags"

	CapabilityBigSegments                 = "big-segments"
	CapabilityContextType                 = "context-type"
	CapabilityContextComparison           = "context-comparison"
	CapabilitySecureModeHash              = "secure-mode-hash"
	CapabilityServerSidePolling           = "server-side-polling"
	CapabilityServiceEndpoints            = "service-endpoints"
	CapabilityTags                        = "tags"
	CapabilityUserType                    = "user-type"
	CapabilityFiltering                   = "filtering"
	CapabilityFilteringStrict             = "filtering-strict"
	CapabilityAutoEnvAttributes           = "auto-env-attributes"
	CapabilityMigrations                  = "migrations"
	CapabilityEventSampling               = "event-sampling"
	CapabilityEventGzip                   = "event-gzip"
	CapabilityOptionalEventGzip           = "optional-event-gzip"
	CapabilityETagCaching                 = "etag-caching"
	CapabilityInlineContext               = "inline-context"
	CapabilityInlineContextAll            = "inline-context-all"
	CapabilityInstanceID                  = "instance-id"
	CapabilityAnonymousRedaction          = "anonymous-redaction"
	CapabilityPollingGzip                 = "polling-gzip"
	CapabilityEvaluationHooks             = "evaluation-hooks"
	CapabilityFlagChangeListeners         = "flag-change-listeners"
	CapabilityFlagValueChangeListeners    = "flag-value-change-listeners"
	CapabilityTrackHooks                  = "track-hooks"
	CapabilityClientPrereqEvents          = "client-prereq-events"
	CapabilityPersistentDataStoreRedis    = "persistent-data-store-redis"
	CapabilityPersistentDataStoreConsul   = "persistent-data-store-consul"
	CapabilityPersistentDataStoreDynamoDB = "persistent-data-store-dynamodb"
	CapabilityClientPerContextSummaries   = "client-per-context-summaries"

	// CapabilityTLSVerifyPeer means the SDK is capable of establishing a TLS session and verifying
	// its peer. This is generally a standard capability of all SDKs.
	// However, the additional tests this enables may cause the suite to run slower than normal and may cause
	// unexpected behavior. Therefore, it should be manually tested first.
	CapabilityTLSVerifyPeer = "tls:verify-peer"

	// CapabilityTLSSkipVerifyPeer means the SDK is capable of establishing a TLS session but can be configured to
	// skip the peer verification step. This allows the SDK to establish a connection with the test harness using
	// a self-signed certificate without a CA. Not all SDKs have this capability.
	CapabilityTLSSkipVerifyPeer = "tls:skip-verify-peer"

	// CapabilityTLSCustomCA means the SDK is capable of establishing a TLS session and configuring peer verification
	// to use a custom CA certificate. The path to this CA cert is provided to the SDK. The SDK should then configure this
	// path as the only CA cert in its trust store (rather than adding it to an existing trust store.)
	CapabilityTLSCustomCA = "tls:custom-ca"

	// CapabilityClientEventSourceHTTPErrors indicates a client-side SDK's EventSource
	// implementation can detect HTTP error status codes (e.g. 401) and response headers.
	// Browser-native EventSource does not expose this information, so browser SDKs
	// typically lack this capability. Only checked for client-side SDKs; server-side
	// SDKs do not need to declare it.
	CapabilityClientEventSourceHTTPErrors = "client-event-source-http-errors"

	// CapabilityClientUseReport indicates that a client-side SDK can be configured to issue
	// streaming and polling flag requests using the REPORT HTTP method instead of GET.
	// SDKs that lack this capability will only have their GET request variants exercised,
	// and tests that hardcode REPORT will be skipped. Only checked for client-side SDKs;
	// server-side SDKs do not need to declare it.
	CapabilityClientUseReport = "client-use-report"

	CapabilityOmitAnonymousContexts = "omit-anonymous-contexts"

	// CapabilityWrapper indicates that the SDK supports setting wrapper name and version and including them in request
	// headers.
	CapabilityWrapper = "wrapper"

	// CapabilityHTTPProxy indicates that the SDK supports setting an HTTP proxy, through which the SDK will
	// make all requests.
	CapabilityHTTPProxy = "http-proxy"

	// CapabilityFDv1Fallback indicates that the SDK honors the server-directed FDv1 Fallback Directive
	// described in section 1.6 of the Data System spec. When an FDv2 endpoint returns an
	// `X-LD-FD-Fallback: true` response header, an SDK with this capability halts its configured FDv2
	// data sources and switches to its FDv1 Fallback Synchronizer for the remainder of its lifetime.
	//
	// This capability gates the "FDv1 fallback directive" subtests so that SDKs which do not yet
	// implement the behavior can opt out, and so the tests can be decommissioned by dropping the
	// capability from all SDK test services once it is ubiquitous.
	CapabilityFDv1Fallback = "fdv1-fallback"
)

type StatusRep struct {
	serviceinfo.TestServiceInfo
	ClientVersion string `json:"clientVersion"`
}

type CreateInstanceParams struct {
	Configuration SDKConfigParams `json:"configuration"`
	Tag           string          `json:"tag"`
}
