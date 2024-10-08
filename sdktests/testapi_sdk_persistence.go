package sdktests

import (
	"errors"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

type Persistence struct {
	Store o.Maybe[servicedef.SDKConfigDataSystemPersistenceStore]
	Cache o.Maybe[servicedef.SDKConfigDataSystemPersistenceCache]
}

func NewPersistence() *Persistence {
	return &Persistence{}
}

func (p *Persistence) SetStore(store servicedef.SDKConfigDataSystemPersistenceStore) {
	p.Store = o.Some(store)
}

func (p *Persistence) SetCache(cache servicedef.SDKConfigDataSystemPersistenceCache) {
	p.Cache = o.Some(cache)
}

func (p Persistence) Configure(target *servicedef.SDKConfigParams) error {
	if !p.Store.IsDefined() || !p.Cache.IsDefined() {
		return errors.New("Persistence must have a store configuration")
	}

	target.Persistence = o.Some(servicedef.SDKConfigDataSystemPersistence{
		Store: p.Store.Value(),
		Cache: p.Cache.Value(),
	})

	return nil
}
