package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

func doClientSideAutoEnvAttributesTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityAutoEnvAttributes)
	t.Run("no collisions", doClientSideAutoEnvAttributesEventsNoCollisionsTests)
	t.Run("collisions", doClientSideAutoEnvAttributesEventsCollisionsTests)
	t.Run("tags", doClientSideAutoEnvAttributesHeaderTests)
}

func doClientSideAutoEnvAttributesEventsNoCollisionsTests(t *ldtest.T) {
	NewCommonEventTests(t, "doClientSideAutoEnvAttributesEventsNoCollisionsTests").
		AutoEnvAttributesNoCollisions(t)
}

func doClientSideAutoEnvAttributesEventsCollisionsTests(t *ldtest.T) {
	NewCommonEventTests(t, "doClientSideAutoEnvAttributesEventsCollisionsTests").
		AutoEnvAttributesCollisions(t)
}

func doClientSideAutoEnvAttributesHeaderTests(t *ldtest.T) {
	NewCommonEventTests(t, "doClientSideAutoEnvAttributesHeaderTests").
		AutoEnvAttributesTags(t)
}
