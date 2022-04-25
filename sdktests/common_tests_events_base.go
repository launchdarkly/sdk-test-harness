package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

const defaultEventTimeout = time.Second * 5

// CommonEventTests groups together event-related test methods that are shared between server-side and client-side.
type CommonEventTests struct {
	isClientSide   bool
	sdkConfigurers []SDKConfigurer
	contextFactory *data.ContextFactory
}

func NewClientSideEventTests(testName string, baseSDKConfigurers ...SDKConfigurer) CommonEventTests {
	contextFactory := data.NewContextFactory(testName)
	return CommonEventTests{
		isClientSide: true,
		sdkConfigurers: append(
			[]SDKConfigurer{
				WithClientSideConfig(servicedef.SDKConfigClientSideParams{
					InitialContext: contextFactory.NextUniqueContext(),
				}),
			},
			baseSDKConfigurers...,
		),
		contextFactory: contextFactory,
	}
}

func NewServerSideEventTests(testName string, baseSDKConfigurers ...SDKConfigurer) CommonEventTests {
	contextFactory := data.NewContextFactory(testName)
	return CommonEventTests{
		isClientSide:   false,
		sdkConfigurers: baseSDKConfigurers,
		contextFactory: contextFactory,
	}
}

func (c CommonEventTests) baseSDKConfigurationPlus(configurers ...SDKConfigurer) []SDKConfigurer {
	return append(c.sdkConfigurers, configurers...)
}

func (c CommonEventTests) discardIdentifyEventIfClientSide(t *ldtest.T, client *SDKClient, events *SDKEventSink) {
	if c.isClientSide {
		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, time.Second)
		m.In(t).Assert(payload, m.Items(IsIdentifyEvent()))
	}
}

func (c CommonEventTests) initialEventPayloadExpectations() []m.Matcher {
	// Server-side SDKs do not send any events in the first payload unless some action are taken
	if !c.isClientSide {
		return nil
	}
	// Client-side SDKs always send an initial identify event
	return []m.Matcher{IsIdentifyEvent()}
}
