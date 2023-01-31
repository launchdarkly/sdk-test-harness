package servicedef

import "github.com/launchdarkly/sdk-test-harness/framework/harness"

const (
	CapabilityClientSide    = "client-side"
	CapabilityServerSide    = "server-side"
	CapabilityStronglyTyped = "strongly-typed"
	CapabilityMobile        = "mobile"
	CapabilityPHP           = "php"
	CapabilitySingleton     = "singleton"

	CapabilityAllFlagsWithReasons                = "all-flags-with-reasons"
	CapabilityAllFlagsClientSideOnly             = "all-flags-client-side-only"
	CapabilityAllFlagsDetailsOnlyForTrackedFlags = "all-flags-details-only-for-tracked-flags"

	CapabilityBigSegments       = "big-segments"
	CapabilitySecureModeHash    = "secure-mode-hash"
	CapabilityServerSidePolling = "server-side-polling"
	CapabilityServiceEndpoints  = "service-endpoints"
	CapabilityTags              = "tags"
	CapabilityFiltering         = "filtering"
)

type StatusRep struct {
	harness.TestServiceInfo
	ClientVersion string `json:"clientVersion"`
}

type CreateInstanceParams struct {
	Configuration SDKConfigParams `json:"configuration"`
	Tag           string          `json:"tag"`
}
