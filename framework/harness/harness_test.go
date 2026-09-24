package harness

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

func TestStripPersistenceCapabilities(t *testing.T) {
	capabilities := framework.Capabilities{
		servicedef.CapabilityServerSide,
		servicedef.CapabilityPersistentDataStoreRedis,
		servicedef.CapabilityPersistentDataStoreConsul,
		servicedef.CapabilityPersistentDataStoreDynamoDB,
		servicedef.CapabilityPersistentDataStoreRecovery,
	}

	// NullLogger drops the per-capability disable log; the test only checks the filtered list
	filtered := stripPersistenceCapabilities(capabilities, framework.NullLogger())

	assert.Equal(t, framework.Capabilities{servicedef.CapabilityServerSide}, filtered)
}

func TestStripPersistenceCapabilitiesKeepsUnrelated(t *testing.T) {
	capabilities := framework.Capabilities{
		servicedef.CapabilityServerSide,
		servicedef.CapabilityMigrations,
	}

	filtered := stripPersistenceCapabilities(capabilities, framework.NullLogger())

	assert.Equal(t, capabilities, filtered)
}
