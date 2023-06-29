package callbackfixtures

import "github.com/launchdarkly/go-sdk-common/v3/ldtime"

const (
	BigSegmentStorePathGetMetadata   = "/getMetadata"
	BigSegmentStorePathGetMembership = "/getMembership"
)

type BigSegmentStoreGetMetadataResponse struct {
	LastUpToDate ldtime.UnixMillisecondTime `json:"lastUpToDate"`
}

type BigSegmentStoreGetMembershipParams struct {
	ContextHash string `json:"contextHash"`
}

type BigSegmentStoreGetMembershipResponse struct {
	Values map[string]bool `json:"values,omitempty"`
}
