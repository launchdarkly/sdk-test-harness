package sdktests

import (
	"testing"

	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/stretchr/testify/assert"
)

func TestValidateSDKConfigChecksDataProtocol(t *testing.T) {
	streaming := o.Some(servicedef.SDKConfigStreamingParams{BaseURI: "http://localhost/stream"})
	polling := o.Some(servicedef.SDKConfigPollingParams{BaseURI: "http://localhost/poll"})
	dataSystem := o.Some(servicedef.DataSystem{})

	for _, p := range []struct {
		name     string
		protocol mockld.DataProtocol
		config   servicedef.SDKConfigParams
		valid    bool
	}{
		{"FDv1 streaming", mockld.FDv1, servicedef.SDKConfigParams{Streaming: streaming}, true},
		{"FDv1 polling", mockld.FDv1, servicedef.SDKConfigParams{Polling: polling}, true},
		{"FDv1 streaming URI from service endpoints", mockld.FDv1, servicedef.SDKConfigParams{
			ServiceEndpoints: o.Some(servicedef.SDKConfigServiceEndpointsParams{Streaming: "http://localhost"}),
		}, true},
		{"FDv1 data system", mockld.FDv1, servicedef.SDKConfigParams{Streaming: streaming, DataSystem: dataSystem}, false},
		{"FDv1 no data source", mockld.FDv1, servicedef.SDKConfigParams{}, false},
		{"FDv1 streaming without base URI", mockld.FDv1, servicedef.SDKConfigParams{
			Streaming: o.Some(servicedef.SDKConfigStreamingParams{}),
		}, false},
		{"FDv2 data system", mockld.FDv2, servicedef.SDKConfigParams{DataSystem: dataSystem}, true},
		{"FDv2 streaming", mockld.FDv2, servicedef.SDKConfigParams{Streaming: streaming, DataSystem: dataSystem}, false},
		{"FDv2 polling", mockld.FDv2, servicedef.SDKConfigParams{Polling: polling, DataSystem: dataSystem}, false},
		{"FDv2 no data source", mockld.FDv2, servicedef.SDKConfigParams{}, false},
	} {
		t.Run(p.name, func(t *testing.T) {
			err := validateSDKConfig(p.config, p.protocol)
			if p.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
