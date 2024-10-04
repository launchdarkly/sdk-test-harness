package sdktests

import (
	"errors"

	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

type DataSystem struct {
	synchronizers []servicedef.SDKConfigDataSystemDataSource
	initializers  []servicedef.SDKConfigDataSystemDataSource
	persistence   o.Maybe[servicedef.SDKConfigDataSystemPersistence]
}

func NewDataSystem() *DataSystem {
	return &DataSystem{}
}

func (d *DataSystem) AddSynchronizer(synchronizer servicedef.SDKConfigDataSystemDataSource) {
	d.synchronizers = append(d.synchronizers, synchronizer)
}

func (d *DataSystem) AddInitializer(initializer servicedef.SDKConfigDataSystemDataSource) {
	d.initializers = append(d.initializers, initializer)
}

func (d *DataSystem) AddPersistence(persistence servicedef.SDKConfigDataSystemPersistence) {
	d.persistence = o.Some(persistence)
}

func (d DataSystem) Configure(target *servicedef.SDKConfigParams) error {
	if len(d.synchronizers) == 0 && len(d.initializers) == 0 && !d.persistence.IsDefined() {
		return errors.New("DataSystem must have at least one synchronizer, initializer, or persistence configuration")
	}

	target.Streaming = o.None[servicedef.SDKConfigStreamingParams]()
	target.Polling = o.None[servicedef.SDKConfigPollingParams]()
	target.DataSystem = o.Some(servicedef.SDKConfigDataSystem{
		Synchronizers: d.synchronizers,
		Initializers:  d.initializers,
		Persistence:   d.persistence,
	})

	return nil
}
