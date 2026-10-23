package sdktests

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// The tests in this file verify that an SDK remains functional when LaunchDarkly serves it a flag
// or segment definition that is valid JSON but has the wrong shape-- for instance, a flag with no
// "variations" property, a target or clause with no "values" property, or a segment whose
// "included" property is not an array. LaunchDarkly would never deliberately serve such data, but
// SDKs must be robust against it: evaluating the malformed flag must produce a well-formed
// evaluation result instead of crashing the application, hanging, or breaking evaluations of
// unrelated flags.
//
// For each shape, the acceptable outcomes of evaluating the malformed flag are:
//
//  1. an error result with reason MALFORMED_FLAG, EXCEPTION, or FLAG_NOT_FOUND (the SDK rejected
//     the definition, either at ingest time or at evaluation time), or
//  2. for shapes where a lenient evaluator can legitimately produce a value, that expected value
//     (for instance, a target with no values simply does not match, so the fallthrough applies).
//
// It is also acceptable for an SDK to reject an entire payload because one item in it is malformed;
// in that case the SDK keeps its previous data and the malformed flag behaves like an unknown flag
// (case 1 with FLAG_NOT_FOUND). A crash, a hang, or an error response from the test service is
// always a test failure-- the evaluation helpers in this file fail the test immediately if the
// test service returns an error.

const (
	malformedDataBadFlagKey      = "bad-flag"
	malformedDataBadSegmentKey   = "bad-segment"
	malformedDataGoodFlagKey     = "good-flag"
	malformedDataSentinelFlagKey = "sentinel-flag"

	malformedDataSentinelTimeout = 10 * time.Second
	malformedDataSentinelPoll    = 100 * time.Millisecond
)

var malformedDataGoodFlagJSON = json.RawMessage( //nolint:gochecknoglobals
	`{"key":"good-flag","version":1,"on":true,"salt":"salt","targets":[],"rules":[],"prerequisites":[],` +
		`"variations":["good","other"],` +
		`"fallthrough":{"variation":0},"offVariation":0}`)

var malformedDataSentinelFlagJSON = json.RawMessage( //nolint:gochecknoglobals
	`{"key":"sentinel-flag","version":1,"on":true,"salt":"salt","targets":[],"rules":[],"prerequisites":[],` +
		`"variations":["sentinel"],` +
		`"fallthrough":{"variation":0},"offVariation":0}`)

type malformedDataScenario struct {
	name string
	// flagJSON is the wrong-shaped flag definition for "bad-flag".
	flagJSON json.RawMessage
	// segmentJSON is the wrong-shaped segment definition for "bad-segment", or nil if the
	// scenario does not involve a segment.
	segmentJSON json.RawMessage
	// acceptableOutcomes are matchers for the evaluation detail response of the malformed flag;
	// the test passes if any of them matches.
	acceptableOutcomes []m.Matcher
}

// malformedFlagErrorOutcomes returns the error outcomes that are always acceptable for a malformed
// flag, plus any additional matchers (such as an expected served value for lenient evaluators).
func malformedFlagErrorOutcomes(extra ...m.Matcher) []m.Matcher {
	matchers := make([]m.Matcher, 0, 3+len(extra))
	for _, errorKind := range []ldreason.EvalErrorKind{
		ldreason.EvalErrorMalformedFlag,
		ldreason.EvalErrorException,
		ldreason.EvalErrorFlagNotFound,
	} {
		matchers = append(matchers,
			EvalResponseReason().Should(EqualReason(ldreason.NewEvalReasonError(errorKind))))
	}
	return append(matchers, extra...)
}

func servedValueOutcome(value string) m.Matcher {
	return EvalResponseValue().Should(m.JSONEqual(ldvalue.String(value)))
}

//nolint:lll
func malformedDataScenarios() []malformedDataScenario {
	return []malformedDataScenario{
		{
			name:               "flag without variations",
			flagJSON:           json.RawMessage(`{"key":"bad-flag","version":1,"on":true,"salt":"salt","targets":[],"rules":[],"prerequisites":[],"fallthrough":{"variation":0},"offVariation":0}`),
			acceptableOutcomes: malformedFlagErrorOutcomes(),
		},
		{
			name: "target without values",
			// A target with no values matches nothing, so a lenient evaluator serves the
			// fallthrough value "b".
			flagJSON:           json.RawMessage(`{"key":"bad-flag","version":1,"on":true,"salt":"salt","variations":["a","b"],"targets":[{"variation":0}],"rules":[],"prerequisites":[],"fallthrough":{"variation":1},"offVariation":1}`),
			acceptableOutcomes: malformedFlagErrorOutcomes(servedValueOutcome("b")),
		},
		{
			name: "clause without values",
			// A clause with no values matches nothing, so a lenient evaluator serves the
			// fallthrough value "b".
			flagJSON:           json.RawMessage(`{"key":"bad-flag","version":1,"on":true,"salt":"salt","variations":["a","b"],"targets":[],"prerequisites":[],"rules":[{"id":"rule-0","variation":0,"trackEvents":false,"clauses":[{"attribute":"key","op":"in"}]}],"fallthrough":{"variation":1},"offVariation":1}`),
			acceptableOutcomes: malformedFlagErrorOutcomes(servedValueOutcome("b")),
		},
		{
			name: "segment included not an array",
			// If an SDK drops only the malformed segment at ingest, the well-formed flag remains;
			// its segmentMatch clause then errors (the canonical behavior) or, in a lenient
			// evaluator that treats the clause as non-matching, falls through to "b".
			flagJSON:           json.RawMessage(`{"key":"bad-flag","version":1,"on":true,"salt":"salt","variations":["a","b"],"targets":[],"prerequisites":[],"rules":[{"id":"rule-0","variation":0,"trackEvents":false,"clauses":[{"attribute":"","op":"segmentMatch","values":["bad-segment"]}]}],"fallthrough":{"variation":1},"offVariation":1}`),
			segmentJSON:        json.RawMessage(`{"key":"bad-segment","version":1,"included":{"user-key":true},"excluded":[],"rules":[],"salt":"salt"}`),
			acceptableOutcomes: malformedFlagErrorOutcomes(servedValueOutcome("b")),
		},
		{
			name: "segment with generated sets",
			// generated_includedSet is an SDK-internal representation that must never appear in
			// JSON data. SDKs that ignore the unknown property see a valid segment that includes
			// the context, so the rule matches and serves "a". An SDK that instead drops the
			// segment at ingest errors on the segmentMatch clause or, in a lenient evaluator,
			// treats it as non-matching and serves "b".
			flagJSON:           json.RawMessage(`{"key":"bad-flag","version":1,"on":true,"salt":"salt","variations":["a","b"],"targets":[],"prerequisites":[],"rules":[{"id":"rule-0","variation":0,"trackEvents":false,"clauses":[{"attribute":"","op":"segmentMatch","values":["bad-segment"]}]}],"fallthrough":{"variation":1},"offVariation":1}`),
			segmentJSON:        json.RawMessage(`{"key":"bad-segment","version":1,"included":["user-key"],"generated_includedSet":{"user-key":true},"excluded":[],"rules":[],"salt":"salt"}`),
			acceptableOutcomes: malformedFlagErrorOutcomes(servedValueOutcome("a"), servedValueOutcome("b")),
		},
		{
			name:               "many rules",
			flagJSON:           makeMalformedManyRulesFlagJSON(51),
			acceptableOutcomes: malformedFlagErrorOutcomes(servedValueOutcome("b")),
		},
	}
}

// makeMalformedManyRulesFlagJSON builds a flag that has many rules whose clauses all lack a
// "values" property. In SDKs that defer per-rule work, iterating over a large number of malformed
// rules has historically been able to crash the process, so it is worth covering explicitly.
func makeMalformedManyRulesFlagJSON(ruleCount int) json.RawMessage {
	rules := make([]string, 0, ruleCount)
	for i := 0; i < ruleCount; i++ {
		rules = append(rules, fmt.Sprintf(
			`{"id":"rule-%d","variation":0,"trackEvents":false,"clauses":[{"attribute":"key","op":"in"}]}`, i))
	}
	return json.RawMessage(fmt.Sprintf(
		`{"key":"bad-flag","version":1,"on":true,"salt":"salt","variations":["a","b"],"targets":[],"prerequisites":[],`+
			`"rules":[%s],`+
			`"fallthrough":{"variation":1},"offVariation":1}`,
		strings.Join(rules, ",")))
}

func doServerSideMalformedDataTests(t *ldtest.T) {
	t.Run("streaming", doServerSideMalformedDataStreamingTests)
	t.Run("polling", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityServerSidePolling)
		doServerSideMalformedDataPollingTests(t)
	})
}

func doServerSideMalformedDataStreamingTests(t *ldtest.T) {
	for _, scenario := range malformedDataScenarios() {
		t.Run(scenario.name, func(t *ldtest.T) {
			context := ldcontext.New("user-key")
			defaultValue := ldvalue.String("default-value")

			initialData := mockld.NewServerSDKDataBuilder().
				RawFlag(malformedDataGoodFlagKey, malformedDataGoodFlagJSON).Build()
			dataSystem := NewSDKDataSystem(t, initialData, DataSystemOptionStreaming())
			client := NewSDKClient(t, dataSystem)

			m.In(t).Require(basicEvaluateFlag(t, client, malformedDataGoodFlagKey, context, defaultValue),
				m.JSONEqual(ldvalue.String("good")))

			// Push the sentinel flag first: if the SDK drops the stream upon receiving the
			// malformed item, the payload is discarded either way, but if it processes items
			// individually the sentinel has the best chance of being seen.
			streaming := dataSystem.Synchronizers[0].streaming
			streaming.PushUpdate("flag", malformedDataSentinelFlagKey, 1, malformedDataSentinelFlagJSON)
			streaming.PushUpdate("flag", malformedDataBadFlagKey, 1, scenario.flagJSON)
			if scenario.segmentJSON != nil {
				streaming.PushUpdate("segment", malformedDataBadSegmentKey, 1, scenario.segmentJSON)
			}
			streaming.PushPayloadTransferred("update", 2)

			logSentinelOutcome(t, waitForSentinelFlag(t, client, context))

			checkMalformedDataScenario(t, client, scenario, context)
		})
	}
}

func doServerSideMalformedDataPollingTests(t *ldtest.T) {
	for _, scenario := range malformedDataScenarios() {
		t.Run(scenario.name, func(t *ldtest.T) {
			context := ldcontext.New("user-key")
			defaultValue := ldvalue.String("default-value")

			// The polling initializer serves only the good flag, so the SDK always initializes
			// with valid data. The polling synchronizer serves the full payload including the
			// malformed item(s) and the sentinel, so the SDK receives the malformed data on its
			// first poll, which happens immediately after initialization. (Delivering the data
			// via a later poll would not work within a test-friendly timespan, because SDKs
			// enforce a minimum polling interval of 30 seconds.)
			initialData := mockld.NewServerSDKDataBuilder().
				RawFlag(malformedDataGoodFlagKey, malformedDataGoodFlagJSON).Build()

			fullDataBuilder := mockld.NewServerSDKDataBuilder().
				RawFlag(malformedDataGoodFlagKey, malformedDataGoodFlagJSON).
				RawFlag(malformedDataSentinelFlagKey, malformedDataSentinelFlagJSON).
				RawFlag(malformedDataBadFlagKey, scenario.flagJSON)
			if scenario.segmentJSON != nil {
				fullDataBuilder.RawSegment(malformedDataBadSegmentKey, scenario.segmentJSON)
			}
			// Use a distinct payload state so the SDK cannot mistake this payload for the one it
			// already received from the initializer.
			fullData := mockld.FDv2SDKDataFromServerSDKData(
				fullDataBuilder.Build(), "xfer-full", "update", "malformed-data-payload")

			dataSystem := NewSDKDataSystemCustom(t, fullData,
				DataSystemOptionPollingInitializer(initialData),
				DataSystemOptionPolling())
			dataSystem.CreateEndpoints()
			client := NewSDKClient(t, dataSystem)

			m.In(t).Require(basicEvaluateFlag(t, client, malformedDataGoodFlagKey, context, defaultValue),
				m.JSONEqual(ldvalue.String("good")))

			logSentinelOutcome(t, waitForSentinelFlag(t, client, context))

			checkMalformedDataScenario(t, client, scenario, context)
		})
	}
}

// waitForSentinelFlag polls the sentinel flag until it returns the sentinel value or the deadline
// passes, and returns whether it appeared. The sentinel is part of the same payload as the
// malformed item(s), so its appearance means the SDK accepted and applied that payload. Its
// continued absence means the SDK rejected the whole payload-- which is an acceptable outcome--
// so this must not fail the test. An error response from the test service still fails the test,
// via the evaluation helper's own error checking.
func waitForSentinelFlag(t *ldtest.T, client *SDKClient, context ldcontext.Context) bool {
	deadline := time.Now().Add(malformedDataSentinelTimeout)
	for {
		value := basicEvaluateFlag(t, client, malformedDataSentinelFlagKey, context,
			ldvalue.String("default-value"))
		if value.Equal(ldvalue.String("sentinel")) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(malformedDataSentinelPoll)
	}
}

func logSentinelOutcome(t *ldtest.T, sentinelReceived bool) {
	if sentinelReceived {
		t.DebugLogger().Printf("sentinel flag was served: the SDK applied the payload containing the malformed item(s)")
	} else {
		t.DebugLogger().Printf("sentinel flag was never served: the SDK rejected the malformed payload")
	}
}

// checkMalformedDataScenario asserts the contract that is common to both data sources: evaluating
// the malformed flag produces an acceptable outcome, unrelated flags keep working, and an
// all-flags evaluation succeeds and includes the good flag. Nothing is asserted about the
// malformed flag's entry in the all-flags result, because SDKs legitimately differ (a null value
// with an error reason, or no entry at all if the definition was rejected at ingest).
func checkMalformedDataScenario(
	t *ldtest.T,
	client *SDKClient,
	scenario malformedDataScenario,
	context ldcontext.Context,
) {
	defaultValue := ldvalue.String("default-value")

	result := evaluateFlagDetail(t, client, malformedDataBadFlagKey, context, defaultValue)
	m.In(t).For("evaluation of malformed flag").Assert(result, m.AnyOf(scenario.acceptableOutcomes...))

	m.In(t).For("evaluation of unrelated flag").
		Require(basicEvaluateFlag(t, client, malformedDataGoodFlagKey, context, defaultValue),
			m.JSONEqual(ldvalue.String("good")))

	allFlagsResult := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{Context: o.Some(context)})
	m.In(t).For("all-flags evaluation").
		Assert(allFlagsResult, EvalAllFlagsValueForKeyShouldEqual(malformedDataGoodFlagKey, ldvalue.String("good")))
}
