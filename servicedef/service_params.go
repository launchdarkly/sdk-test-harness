package servicedef

import "github.com/launchdarkly/sdk-test-harness/framework/harness"

const (
	CapabilityClientSide    = "client-side"
	CapabilityServerSide    = "server-side"
	CapabilityStronglyTyped = "strongly-typed"
)

type StatusRep struct {
	harness.TestServiceInfo
	ClientVersion string `json:"clientVersion"`
}

type CreateInstanceParams struct {
	Configuration SDKConfigParams `json:"configuration"`
	Tag           string          `json:"tag"`
}
