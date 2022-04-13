package mockld

import (
	"encoding/json"
	"net/http"

	"github.com/launchdarkly/sdk-test-harness/framework"
	cf "github.com/launchdarkly/sdk-test-harness/servicedef/callbackfixtures"
)

// PersistentDataStoreService is the low-level component providing the mock endpoints for the persistent
// data store test fixture. The higher-level component sdktests.PersistentDataStore decorates this to
// make it more convenient to use in test logic.
type PersistentDataStoreService struct {
	service         *callbackService
	initFn          func(cf.DataStoreInitParams) error
	getFn           func(cf.DataStoreGetParams) (cf.DataStoreGetResponse, error)
	getAllFn        func(cf.DataStoreGetAllParams) (cf.DataStoreGetAllResponse, error)
	upsertFn        func(cf.DataStoreUpsertParams) (cf.DataStoreUpsertResponse, error)
	isInitializedFn func() (bool, error)
}

func NewPersistentDataStoreService(
	initFn func(cf.DataStoreInitParams) error,
	getFn func(cf.DataStoreGetParams) (cf.DataStoreGetResponse, error),
	getAllFn func(cf.DataStoreGetAllParams) (cf.DataStoreGetAllResponse, error),
	upsertFn func(cf.DataStoreUpsertParams) (cf.DataStoreUpsertResponse, error),
	isInitializedFn func() (bool, error),
	logger framework.Logger,
) *PersistentDataStoreService {
	service := newCallbackService(logger, "PersistentDataStore")
	s := &PersistentDataStoreService{service: service,
		initFn: initFn, getFn: getFn, getAllFn: getAllFn, upsertFn: upsertFn, isInitializedFn: isInitializedFn}
	service.addPath(cf.PersistentDataStorePathInit, s.doInit)
	service.addPath(cf.PersistentDataStorePathGet, s.doGet)
	service.addPath(cf.PersistentDataStorePathGetAll, s.doGetAll)
	service.addPath(cf.PersistentDataStorePathUpsert, s.doUpsert)
	service.addPath(cf.PersistentDataStorePathIsInitialized, s.doIsInitialized)
	return s
}

func (s *PersistentDataStoreService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.service.router.ServeHTTP(w, r)
}

func (s *PersistentDataStoreService) doInit(readParams *json.Decoder) (interface{}, error) {
	var params cf.DataStoreInitParams
	if err := readParams.Decode(&params); err != nil {
		return nil, err
	}
	return nil, s.initFn(params)
}

func (s *PersistentDataStoreService) doGet(readParams *json.Decoder) (interface{}, error) {
	var params cf.DataStoreGetParams
	if err := readParams.Decode(&params); err != nil {
		return nil, err
	}
	return s.getFn(params)
}

func (s *PersistentDataStoreService) doGetAll(readParams *json.Decoder) (interface{}, error) {
	var params cf.DataStoreGetAllParams
	if err := readParams.Decode(&params); err != nil {
		return nil, err
	}
	return s.getAllFn(params)
}

func (s *PersistentDataStoreService) doUpsert(readParams *json.Decoder) (interface{}, error) {
	var params cf.DataStoreUpsertParams
	if err := readParams.Decode(&params); err != nil {
		return nil, err
	}
	return s.upsertFn(params)
}

func (s *PersistentDataStoreService) doIsInitialized(*json.Decoder) (interface{}, error) {
	result, err := s.isInitializedFn()
	if err != nil {
		return nil, err
	}
	return cf.DataStoreIsInitializedResponse{Result: result}, nil
}
