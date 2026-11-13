package sdktests

import (
	"testing"

	"github.com/launchdarkly/sdk-test-harness/v3/mockld"

	"github.com/stretchr/testify/assert"
)

func TestExperimentExposureFlagAfterKeepsValueAndVariation(t *testing.T) {
	before := experimentExposureFlagBefore()
	after := experimentExposureFlagAfter()

	assert.Equal(t, before.Value, after.Value)
	assert.Equal(t, before.Variation, after.Variation)
}

func TestExperimentExposureFlagAfterChangesExperimentFields(t *testing.T) {
	before := experimentExposureFlagBefore()
	after := experimentExposureFlagAfter()

	assert.False(t, before.TrackEvents)
	assert.False(t, before.TrackReason)
	assert.False(t, before.Reason.Value().IsInExperiment())

	assert.True(t, after.TrackEvents)
	assert.True(t, after.TrackReason)
	assert.True(t, after.Reason.Value().IsInExperiment())
	assert.Greater(t, after.Version, before.Version)
	assert.Greater(t, after.FlagVersion.Value(), before.FlagVersion.Value())
}

func TestExperimentExposureDataHasOnlyTheFlag(t *testing.T) {
	flag := experimentExposureFlagAfter()

	assert.Equal(t, mockld.ClientSDKData{experimentExposureFlagKey: flag}, experimentExposureData(flag))
}
