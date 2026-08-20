package sdktests

import (
	"errors"

	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

type Persistence struct {
	Store     o.Maybe[servicedef.SDKConfigPersistentStore]
	StoreMode o.Maybe[servicedef.DataStoreMode]
	Cache     o.Maybe[servicedef.SDKConfigPersistentCache]
}

func NewPersistence() *Persistence {
	return &Persistence{}
}

func (p *Persistence) SetStore(store servicedef.SDKConfigPersistentStore) {
	p.Store = o.Some(store)
}

func (p *Persistence) SetStoreMode(mode servicedef.DataStoreMode) {
	p.StoreMode = o.Some(mode)
}

func (p *Persistence) SetCache(cache servicedef.SDKConfigPersistentCache) {
	p.Cache = o.Some(cache)
}

func (p Persistence) Configure(target *servicedef.SDKConfigParams) error {
	if !p.Store.IsDefined() || !p.Cache.IsDefined() {
		return errors.New("Persistence must have a store and cache configuration")
	}

	dataSystem := target.DataSystem.OrElse(servicedef.DataSystem{})
	dataSystem.Store = o.Some(servicedef.DataStore{
		PersistentDataStore: o.Some(servicedef.SDKConfigPersistentDataStoreParams{
			Store: p.Store.Value(),
			Cache: p.Cache.Value(),
		}),
	})
	dataSystem.StoreMode = p.StoreMode.OrElse(servicedef.DataStoreModeRead)

	target.DataSystem = o.Some(dataSystem)

	return nil
}
