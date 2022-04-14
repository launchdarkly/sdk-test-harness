package servicedef

import "github.com/launchdarkly/sdk-test-harness/framework/harness"

const (
	CapabilityClientSide    = "client-side"
	CapabilityServerSide    = "server-side"
	CapabilityStronglyTyped = "strongly-typed"
	CapabilityMobile        = "mobile"
	CapabilitySingleton     = "singleton"

	CapabilityAllFlagsWithReasons                = "all-flags-with-reasons"
	CapabilityAllFlagsClientSideOnly             = "all-flags-client-side-only"
	CapabilityAllFlagsDetailsOnlyForTrackedFlags = "all-flags-details-only-for-tracked-flags"

	CapabilityPersistentDataStore = "persistent-data-store"
	CapabilityBigSegments         = "big-segments"
	CapabilityTags                = "tags"
)

type StatusRep struct {
	harness.TestServiceInfo
	ClientVersion string `json:"clientVersion"`
}

type CreateInstanceParams struct {
	Configuration SDKConfigParams `json:"configuration"`
	Tag           string          `json:"tag"`
}
