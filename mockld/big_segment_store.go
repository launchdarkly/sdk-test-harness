package mockld

import (
	"encoding/json"
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"
	cf "github.com/launchdarkly/sdk-test-harness/v3/servicedef/callbackfixtures"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
)

// MockBigSegmentStoreService is the low-level component providing the mock endpoints for the
// Big Segments test fixture. The higher-level component sdktests.BigSegmentStore decorates this
// to make it more convenient to use in test logic.
type MockBigSegmentStoreService struct {
	service                *callbackService
	getMetadataFn          func() (ldtime.UnixMillisecondTime, error)
	getContextMembershipFn func(string) (map[string]bool, error)
}

func NewMockBigSegmentStoreService(
	getMetadata func() (ldtime.UnixMillisecondTime, error),
	getContextMembership func(string) (map[string]bool, error),
	logger framework.Logger,
) *MockBigSegmentStoreService {
	service := newCallbackService(logger, "BigSegmentStore")
	m := &MockBigSegmentStoreService{service: service, getMetadataFn: getMetadata,
		getContextMembershipFn: getContextMembership}
	service.addPath(cf.BigSegmentStorePathGetMetadata, m.doGetMetadata)
	service.addPath(cf.BigSegmentStorePathGetMembership, m.doGetMembership)
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

func (m *MockBigSegmentStoreService) doGetMembership(readParams *json.Decoder) (interface{}, error) {
	var params cf.BigSegmentStoreGetMembershipParams
	if err := readParams.Decode(&params); err != nil {
		return nil, err
	}

	var membership map[string]bool
	var err error
	if m.getContextMembershipFn != nil {
		membership, err = m.getContextMembershipFn(params.ContextHash)
	}

	if err != nil {
		return nil, err
	}

	return cf.BigSegmentStoreGetMembershipResponse{Values: membership}, nil
}
