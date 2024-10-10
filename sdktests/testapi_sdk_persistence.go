package sdktests

import (
	"errors"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

type Persistence struct {
	Store o.Maybe[servicedef.SDKConfigPersistenceStore]
	Cache o.Maybe[servicedef.SDKConfigPersistenceCache]
}

func NewPersistence() *Persistence {
	return &Persistence{}
}

func (p *Persistence) SetStore(store servicedef.SDKConfigPersistenceStore) {
	p.Store = o.Some(store)
}

func (p *Persistence) SetCache(cache servicedef.SDKConfigPersistenceCache) {
	p.Cache = o.Some(cache)
}

func (p Persistence) Configure(target *servicedef.SDKConfigParams) error {
	if !p.Store.IsDefined() || !p.Cache.IsDefined() {
		return errors.New("Persistence must have a store configuration")
	}

	target.PersistenceDataStore = o.Some(servicedef.SDKConfigPersistenceDataStoreParams{
		Store: p.Store.Value(),
		Cache: p.Cache.Value(),
	})

	return nil
}
