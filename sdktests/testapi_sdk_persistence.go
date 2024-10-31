package sdktests

import (
	"errors"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

type Persistence struct {
	Store o.Maybe[servicedef.SDKConfigPersistentStore]
	Cache o.Maybe[servicedef.SDKConfigPersistentCache]
}

func NewPersistence() *Persistence {
	return &Persistence{}
}

func (p *Persistence) SetStore(store servicedef.SDKConfigPersistentStore) {
	p.Store = o.Some(store)
}

func (p *Persistence) SetCache(cache servicedef.SDKConfigPersistentCache) {
	p.Cache = o.Some(cache)
}

func (p Persistence) Configure(target *servicedef.SDKConfigParams) error {
	if !p.Store.IsDefined() || !p.Cache.IsDefined() {
		return errors.New("Persistence must have a store and cache configuration")
	}

	target.PersistentDataStore = o.Some(servicedef.SDKConfigPersistentDataStoreParams{
		Store: p.Store.Value(),
		Cache: p.Cache.Value(),
	})

	return nil
}
