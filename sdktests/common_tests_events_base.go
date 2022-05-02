package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

const defaultEventTimeout = time.Second * 5

// CommonEventTests groups together event-related test methods that are shared between server-side and client-side.
type CommonEventTests struct {
	commonTestsBase
}

func NewCommonEventTests(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) CommonEventTests {
	return CommonEventTests{newCommonTestsBase(t, testName, baseSDKConfigurers...)}
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
