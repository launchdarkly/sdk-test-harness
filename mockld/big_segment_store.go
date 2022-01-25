package mockld

import (
	"encoding/json"
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/framework"
	cf "github.com/launchdarkly/sdk-test-harness/servicedef/callbackfixtures"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
)

// MockBigSegmentStoreService is the low-level component providing the mock endpoints for the
// Big Segments test fixture. The higher-level component sdktests.BigSegmentStore decorates this
// to make it more convenient to use in test logic.
type MockBigSegmentStoreService struct {
	service             *callbackService
	getMetadataFn       func() (ldtime.UnixMillisecondTime, error)
	getUserMembershipFn func(string) (map[string]bool, error)
}

func NewMockBigSegmentStoreService(
	getMetadata func() (ldtime.UnixMillisecondTime, error),
	getUserMembership func(string) (map[string]bool, error),
	logger framework.Logger,
) *MockBigSegmentStoreService {
	service := newCallbackService(logger, "BigSegmentStore")
	m := &MockBigSegmentStoreService{service: service, getMetadataFn: getMetadata, getUserMembershipFn: getUserMembership}
	service.addPath(cf.BigSegmentStorePathGetMetadata, m.doGetMetadata)
	service.addPath(cf.BigSegmentStorePathGetMembership, m.doGetUserMembership)
	return m
}

func (m *MockBigSegmentStoreService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.service.router.ServeHTTP(w, r)
}

func (m *MockBigSegmentStoreService) doGetMetadata(*json.Decoder) (interface{}, error) {
	var lastUpToDate ldtime.UnixMillisecondTime
	var err error
	if m.getMetadataFn != nil {
		lastUpToDate, err = m.getMetadataFn()
	}

	if err != nil {
		return nil, err
	}

	return cf.BigSegmentStoreGetMetadataResponse{LastUpToDate: lastUpToDate}, nil
}

func (m *MockBigSegmentStoreService) doGetUserMembership(readParams *json.Decoder) (interface{}, error) {
	var params cf.BigSegmentStoreGetMembershipParams
	if err := readParams.Decode(&params); err != nil {
		return nil, err
	}

	var membership map[string]bool
	var err error
	if m.getUserMembershipFn != nil {
		membership, err = m.getUserMembershipFn(params.UserHash)
	}

	if err != nil {
		return nil, err
	}

	return cf.BigSegmentStoreGetMembershipResponse{Values: membership}, nil
}
