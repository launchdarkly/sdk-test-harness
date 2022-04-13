package sdktests

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	cf "github.com/launchdarkly/sdk-test-harness/servicedef/callbackfixtures"
)

// PersistentDataStore is a test fixture that provides callback endpoints for SDK clients to connect to,
// behaving like a persistent data store for a simulated database.
type PersistentDataStore struct {
	t               *ldtest.T
	service         *mockld.PersistentDataStoreService
	endpoint        *harness.MockEndpoint
	initFn          func(cf.DataStoreInitParams) error
	getFn           func(cf.DataStoreGetParams) (cf.DataStoreGetResponse, error)
	getAllFn        func(cf.DataStoreGetAllParams) (cf.DataStoreGetAllResponse, error)
	upsertFn        func(cf.DataStoreUpsertParams) (cf.DataStoreUpsertResponse, error)
	isInitializedFn func() (bool, error)
	lock            sync.Mutex
}

// NewPersistentDataStore creates a new PersistentDataStore with the specified initial status.
//
// The object's lifecycle is tied to the test scope that created it; it will be automatically closed
// when this test scope exits. It can be reused by subtests until then. Debug output related to the
// data source will be attached to this test scope.
//
// By default, calls to all methods except IsInitialized will cause a test failure unless the test
// has explicitly configured a behavior for that method. IsInitialized will return true by default
// since tests rarely need to change this.
func NewPersistentDataStore(t *ldtest.T) *PersistentDataStore {
	p := &PersistentDataStore{t: t}
	p.service = mockld.NewPersistentDataStoreService(
		p.doInit,
		p.doGet,
		p.doGetAll,
		nil, // p.doUpsert,
		p.doIsInitialized,
		t.DebugLogger(),
	)
	p.SetupIsInitialized(func() (bool, error) { return true, nil }) // reasonable default behavior for most tests
	p.endpoint = requireContext(t).harness.NewMockEndpoint(p.service, nil, t.DebugLogger())
	t.Defer(p.endpoint.Close)

	return p
}

// ApplyConfiguration updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the persistent data store test fixture.
func (p *PersistentDataStore) ApplyConfiguration(config *servicedef.SDKConfigParams) {
	if config.PersistentDataStore == nil {
		config.PersistentDataStore = &servicedef.SDKConfigPersistentDataStoreParams{}
	} else {
		ps := *config.PersistentDataStore
		config.PersistentDataStore = &ps // copy to avoid side effects
	}
	config.PersistentDataStore.CallbackURI = p.endpoint.BaseURL()
}

// SetupInit causes the specified function to be called whenever the SDK calls the Init method
// on the persistent data store.
func (p *PersistentDataStore) SetupInit(fn func(cf.DataStoreInitParams) error) {
	p.lock.Lock()
	p.initFn = fn
	p.lock.Unlock()
}

// SetupInitCapture causes the Init method to simply capture its parameters to a channel.
func (p *PersistentDataStore) SetupInitCapture() <-chan cf.DataStoreInitParams {
	ch := make(chan cf.DataStoreInitParams, 100)
	p.SetupInit(func(p cf.DataStoreInitParams) error {
		ch <- p
		return nil
	})
	return ch
}

// SetupGet causes the specified function to be called whenever the SDK calls the Get method
// on the persistent data store.
func (p *PersistentDataStore) SetupGet(fn func(cf.DataStoreGetParams) (cf.DataStoreGetResponse, error)) {
	p.lock.Lock()
	p.getFn = fn
	p.lock.Unlock()
}

// SetupGetCapture causes the Get method to capture its parameters, and also return values from a data set
// depending on the parameters. Queries for any items not in the data set will return a "not found" result
// (a nil item).
func (p *PersistentDataStore) SetupGetCapture(data mockld.ServerSDKData) <-chan cf.DataStoreGetParams {
	ch := make(chan cf.DataStoreGetParams, 100)
	p.SetupGet(func(p cf.DataStoreGetParams) (cf.DataStoreGetResponse, error) {
		ch <- p
		kind := p.Kind
		if kind == "features" {
			kind = "flags" // SDK persistent data store model is inconsistent with JSON data model on this
		}
		itemData := data[mockld.DataItemKind(kind)][p.Key]
		if itemData == nil {
			return cf.DataStoreGetResponse{}, nil
		}
		var versionOnly struct {
			Version int `json:"version"`
		}
		_ = json.Unmarshal(itemData, &versionOnly)
		return cf.DataStoreGetResponse{
			Item: &cf.DataStoreSerializedItem{
				Version: versionOnly.Version,
				Data:    string(itemData),
			},
		}, nil
	})
	return ch
}

// SetupGetAll causes the specified function to be called whenever the SDK calls the GetAll method
// on the persistent data store.
func (p *PersistentDataStore) SetupGetAll(fn func(cf.DataStoreGetAllParams) (cf.DataStoreGetAllResponse, error)) {
	p.lock.Lock()
	p.getAllFn = fn
	p.lock.Unlock()
}

// SetupGet causes the GetAll method to capture its parameters, and also return values from a data set
// depending on the parameters.
func (p *PersistentDataStore) SetupGetAllCapture(data mockld.ServerSDKData) <-chan cf.DataStoreGetAllParams {
	ch := make(chan cf.DataStoreGetAllParams, 100)
	p.SetupGetAll(func(p cf.DataStoreGetAllParams) (cf.DataStoreGetAllResponse, error) {
		ch <- p
		kind := p.Kind
		if kind == "features" {
			kind = "flags" // SDK persistent data store model is inconsistent with JSON data model on this
		}
		var ret cf.DataStoreGetAllResponse
		for key, itemData := range data[mockld.DataItemKind(kind)] {
			var versionOnly struct {
				Version int `json:"version"`
			}
			_ = json.Unmarshal(itemData, &versionOnly)
			ret.Items = append(ret.Items, cf.DataStoreKeyedItem{
				Key: key,
				Item: cf.DataStoreSerializedItem{
					Version: versionOnly.Version,
					Data:    string(itemData),
				},
			})
		}
		return ret, nil
	})
	return ch
}

// SetupIsInitialized causes the specified function to be called whenever the SDK calls the IsInitialized
// method on the persistent data store.
func (p *PersistentDataStore) SetupIsInitialized(fn func() (bool, error)) {
	p.lock.Lock()
	p.isInitializedFn = fn
	p.lock.Unlock()
}

func (p *PersistentDataStore) doInit(params cf.DataStoreInitParams) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.initFn != nil {
		return p.initFn(params)
	}
	return p.failed(unexpectedPersistentDataStoreMethodErr("Init"))
}

func (p *PersistentDataStore) doGet(params cf.DataStoreGetParams) (cf.DataStoreGetResponse, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.getFn != nil {
		return p.getFn(params)
	}
	return cf.DataStoreGetResponse{}, p.failed(unexpectedPersistentDataStoreMethodErr("Get"))
}

func (p *PersistentDataStore) doGetAll(params cf.DataStoreGetAllParams) (cf.DataStoreGetAllResponse, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.getAllFn != nil {
		return p.getAllFn(params)
	}
	return cf.DataStoreGetAllResponse{}, p.failed(unexpectedPersistentDataStoreMethodErr("GetAll"))
}

func (p *PersistentDataStore) doIsInitialized() (bool, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if p.isInitializedFn != nil {
		return p.isInitializedFn()
	}
	return false, p.failed(unexpectedPersistentDataStoreMethodErr("IsInitialized"))
}

func (p *PersistentDataStore) failed(err error) error {
	if err != nil {
		p.t.Errorf(err.Error())
	}
	return err
}

func unexpectedPersistentDataStoreMethodErr(name string) error {
	return fmt.Errorf("test data store fixture got an unexpected %s call", name)
}
