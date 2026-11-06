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

// BigSegmentStoreGetMembershipResponse is the response to a membership query. A nil Values map is
// sent as an explicit null; an empty map is sent as an empty object.
type BigSegmentStoreGetMembershipResponse struct {
	Values map[string]bool `json:"values"`
}
