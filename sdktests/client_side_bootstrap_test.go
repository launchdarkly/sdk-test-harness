package sdktests

import (
	"encoding/json"
	"testing"

	"github.com/launchdarkly/sdk-test-harness/v2/framework"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingTestLogger captures the IDs that ldtest reports as started.
type recordingTestLogger struct {
	started []string
}

func (r *recordingTestLogger) TestStarted(id ldtest.TestID) {
	r.started = append(r.started, id.String())
}
func (r *recordingTestLogger) TestError(ldtest.TestID, error) {}
func (r *recordingTestLogger) TestFinished(ldtest.TestID, ldtest.TestResult, framework.CapturedOutput) {
}
func (r *recordingTestLogger) TestSkipped(ldtest.TestID, string) {}
func (r *recordingTestLogger) EndLog(ldtest.Results) error       { return nil }

// registeredSubtestNames runs a suite function with a Filter that matches nothing, so every
// t.Run call reports its ID as started but its body never executes. ldtest.T.Run consults
// the Filter before invoking the subtest action, so this needs no SDK test service and no
// mock endpoints.
func registeredSubtestNames(capabilities []string, suite func(*ldtest.T)) []string {
	logger := &recordingTestLogger{}
	_ = ldtest.Run(ldtest.TestConfiguration{
		Capabilities: capabilities,
		Filter:       ldtest.FilterFunc(func(ldtest.TestID) bool { return false }),
		TestLogger:   logger,
	}, suite)
	return logger.started
}

func TestClientSideBootstrapTestsRequireBootstrapCapability(t *testing.T) {
	assert.Empty(t, registeredSubtestNames(nil, doClientSideBootstrapTests),
		"expected no bootstrap subtests to be registered when the test service lacks the %q capability",
		servicedef.CapabilityBootstrap)
}

func TestClientSideBootstrapSubtestsAreRegistered(t *testing.T) {
	assert.ElementsMatch(t, []string{
		"bootstrap flags are evaluable with no network data source",
		"valid bootstrap data satisfies initialization without a network response",
		"bootstrap flag metadata is used for events but not returned by all flags",
		"bootstrap flag metadata prerequisites produce an event for the prerequisite flag",
		"bootstrap flag with a null value evaluates to the caller-supplied default",
		"bootstrap payload with $valid false is ingested without error",
		"legacy bootstrap payload without $flagsState is ingested",
		"bootstrap data is replaced by the first data set from a live synchronizer",
	}, registeredSubtestNames([]string{servicedef.CapabilityBootstrap}, doClientSideBootstrapTests))
}

func TestClientSideSuiteRegistersBootstrapTests(t *testing.T) {
	assert.Contains(t,
		registeredSubtestNames([]string{servicedef.CapabilityClientSide}, doAllClientSideTests),
		"bootstrap")
}

func TestBootstrapPayloadBuilderMatchesTypicalSpecExample(t *testing.T) {
	payload := newBootstrapPayloadBuilder().
		Flag("flagA", ldvalue.Bool(true), &bootstrapFlagMetadata{
			Variation: o.Some(1),
			Version:   o.Some(100),
		}).
		Flag("flagB", ldvalue.String("green"), &bootstrapFlagMetadata{
			Variation:            o.Some(2),
			Version:              o.Some(42),
			TrackEvents:          true,
			DebugEventsUntilDate: o.Some(ldtime.UnixMillisecondTime(1893456000000)),
		}).
		Flag("flagC", ldvalue.Null(), &bootstrapFlagMetadata{Version: o.Some(7)}).
		Valid(true).
		Build()

	actual, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"flagA": true,
		"flagB": "green",
		"flagC": null,
		"$flagsState": {
			"flagA": {"variation": 1, "version": 100},
			"flagB": {
				"variation": 2, "version": 42, "trackEvents": true,
				"debugEventsUntilDate": 1893456000000
			},
			"flagC": {"version": 7}
		},
		"$valid": true
	}`, string(actual))
}

func TestBootstrapPayloadBuilderMatchesWithReasonsSpecExample(t *testing.T) {
	payload := newBootstrapPayloadBuilder().
		Flag("flagA", ldvalue.Bool(true), &bootstrapFlagMetadata{
			Variation: o.Some(1),
			Version:   o.Some(100),
			Reason:    o.Some(ldreason.NewEvalReasonFallthrough()),
		}).
		Flag("flagB", ldvalue.String("green"), &bootstrapFlagMetadata{
			Variation: o.Some(2),
			Version:   o.Some(42),
			Reason:    o.Some(ldreason.NewEvalReasonRuleMatch(0, "rule-abc")),
		}).
		Valid(true).
		Build()

	actual, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"flagA": true,
		"flagB": "green",
		"$flagsState": {
			"flagA": {"variation": 1, "version": 100, "reason": {"kind": "FALLTHROUGH"}},
			"flagB": {
				"variation": 2, "version": 42,
				"reason": {"kind": "RULE_MATCH", "ruleIndex": 0, "ruleId": "rule-abc"}
			}
		},
		"$valid": true
	}`, string(actual))
}

func TestBootstrapPayloadBuilderMatchesInvalidSpecExample(t *testing.T) {
	payload := newBootstrapPayloadBuilder().Valid(false).Build()

	actual, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{"$flagsState": {}, "$valid": false}`, string(actual))
}

func TestBootstrapPayloadBuilderMatchesLegacySpecExample(t *testing.T) {
	payload := newBootstrapPayloadBuilder().
		Flag("flagA", ldvalue.Bool(true), nil).
		Flag("flagB", ldvalue.String("green"), nil).
		OmitFlagsState().
		Build()

	actual, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{"flagA": true, "flagB": "green"}`, string(actual))
}

func TestBootstrapPayloadBuilderOmitsTrackingFlagsWhenFalse(t *testing.T) {
	payload := newBootstrapPayloadBuilder().
		Flag("flagA", ldvalue.Bool(true), &bootstrapFlagMetadata{
			Variation:   o.Some(0),
			Version:     o.Some(1),
			TrackEvents: false,
			TrackReason: false,
		}).
		Build()

	actual, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"flagA": true,
		"$flagsState": {"flagA": {"variation": 0, "version": 1}}
	}`, string(actual))
}

func TestBootstrapPayloadBuilderMatchesPrerequisitesSpecExample(t *testing.T) {
	payload := newBootstrapPayloadBuilder().
		Flag("flagA", ldvalue.Bool(true), &bootstrapFlagMetadata{
			Variation:     o.Some(1),
			Version:       o.Some(100),
			Prerequisites: []string{"flagB"},
		}).
		Flag("flagB", ldvalue.Bool(true), &bootstrapFlagMetadata{
			Variation: o.Some(1),
			Version:   o.Some(42),
		}).
		Valid(true).
		Build()

	actual, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"flagA": true,
		"flagB": true,
		"$flagsState": {
			"flagA": {"variation": 1, "version": 100, "prerequisites": ["flagB"]},
			"flagB": {"variation": 1, "version": 42}
		},
		"$valid": true
	}`, string(actual))
}
