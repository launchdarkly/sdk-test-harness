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

	CapabilityBigSegments        = "big-segments"
	CapabilityContextType        = "context-type"
	CapabilityContextComparison  = "context-comparison"
	CapabilitySecureModeHash     = "secure-mode-hash"
	CapabilityServerSidePolling  = "server-side-polling"
	CapabilityServiceEndpoints   = "service-endpoints"
	CapabilityTags               = "tags"
	CapabilityUserType           = "user-type"
	CapabilityFiltering          = "filtering"
	CapabilityAutoEnvAttributes  = "auto-env-attributes"
	CapabilityMigrations         = "migrations"
	CapabilityEventSampling      = "event-sampling"
	CapabilityEventGzip          = "event-gzip"
	CapabilityETagCaching        = "etag-caching"
	CapabilityInlineContext      = "inline-context"
	CapabilityAnonymousRedaction = "anonymous-redaction"
	CapabilityPollingGzip        = "polling-gzip"
	CapabilityEvaluationHooks    = "evaluation-hooks"

	// CapabilityTLSVerifyPeer means the SDK is capable of establishing a TLS session and verifying
	// its peer. This is generally a standard capability of all SDKs.
	// However, the additional tests this enables may cause the suite to run slower than normal and may cause
	// unexpected behavior. Therefore, it should be manually tested first.
	CapabilityTLSVerifyPeer = "tls:verify-peer"

	// CapabilityTLSSkipVerifyPeer means the SDK is capable of establishing a TLS session but can be configured to
	// skip the peer verification step. This allows the SDK to establish a connection with the test harness using
	// a self-signed certificate without a CA. Not all SDKs have this capability.
	CapabilityTLSSkipVerifyPeer = "tls:skip-verify-peer"
)

type StatusRep struct {
	serviceinfo.TestServiceInfo
	ClientVersion string `json:"clientVersion"`
}

type CreateInstanceParams struct {
	Configuration SDKConfigParams `json:"configuration"`
	Tag           string          `json:"tag"`
}
