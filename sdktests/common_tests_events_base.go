package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

const defaultEventTimeout = time.Second * 5

// CommonEventTests groups together event-related test methods that are shared between server-side and client-side.
type CommonEventTests struct {
	isClientSide   bool
	sdkConfigurers []SDKConfigurer
	userFactory    *UserFactory
}

func NewClientSideEventTests(testName string, baseSDKConfigurers ...SDKConfigurer) CommonEventTests {
	userFactory := NewUserFactory(testName)
	return CommonEventTests{
		isClientSide: true,
		sdkConfigurers: append(
			[]SDKConfigurer{
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialUser: userFactory.NextUniqueUser(),
				}),
			},
			baseSDKConfigurers...,
		),
		userFactory: userFactory,
	}
}

func NewServerSideEventTests(testName string, baseSDKConfigurers ...SDKConfigurer) CommonEventTests {
	userFactory := NewUserFactory(testName)
	return CommonEventTests{
		isClientSide:   false,
		sdkConfigurers: baseSDKConfigurers,
		userFactory:    userFactory,
	}
}

func (c CommonEventTests) baseSDKConfigurationPlus(configurers ...SDKConfigurer) []SDKConfigurer {
	return append(c.sdkConfigurers, configurers...)
}

func (c CommonEventTests) initialEventPayloadExpectations() []m.Matcher {
	// Server-side SDKs do not send any events in the first payload unless some action are taken
	if !c.isClientSide {
		return nil
	}
	// Client-side SDKs always send an initial identify event
	return []m.Matcher{IsIdentifyEvent()}
}
