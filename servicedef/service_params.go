package servicedef

import "github.com/launchdarkly/sdk-test-harness/v2/framework/harness"

const (
	CapabilityClientSide    = "client-side"
	CapabilityServerSide    = "server-side"
	CapabilityStronglyTyped = "strongly-typed"
	CapabilityMobile        = "mobile"
	CapabilityPHP           = "php"
	CapabilityRoku          = "roku"
	CapabilitySingleton     = "singleton"

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
	CapabilityETagCaching        = "etag-caching"
	CapabilityInlineContext      = "inline-context"
	CapabilityAnonymousRedaction = "anonymous-redaction"
)

type StatusRep struct {
	harness.TestServiceInfo
	ClientVersion string `json:"clientVersion"`
}

type CreateInstanceParams struct {
	Configuration SDKConfigParams `json:"configuration"`
	Tag           string          `json:"tag"`
}
