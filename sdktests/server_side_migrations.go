package sdktests

import (
	"fmt"
	"net/http"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldbuilders"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v2/data"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"
)

func doServerSideMigrationTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityMigrations)

	t.Run("migration variation", runMigrationVariationTests)
	t.Run("use correct origins", runUseCorrectOriginsTests)
	t.Run("track latency correctly", runTrackLatencyTests)
	t.Run("track errors correctly", runTrackErrorsTests)
	t.Run("track consistency correctly", runTrackConsistencyTests)
}

func runMigrationVariationTests(t *ldtest.T) {
	stages := []ldmigration.Stage{ldmigration.Off, ldmigration.DualWrite, ldmigration.Shadow, ldmigration.Live, ldmigration.RampDown, ldmigration.Complete}

	for _, stage := range stages {
		client, events := createClient(t, int(stage))
		context := ldcontext.New("key")

		params := servicedef.MigrationVariationParams{
			Key:          "migration-key",
			Context:      context,
			DefaultStage: ldmigration.Off,
		}
		response := client.MigrationVariation(t, params)

		client.FlushEvents(t)

		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			IsIndexEventForContext(context),
			IsSummaryEvent(),
		))

		assert.Equal(t, stage.String(), response.Result)
	}
}

func runUseCorrectOriginsTests(t *ldtest.T) {
	testParams := []struct {
		Operation        ldmigration.Operation
		Stage            ldmigration.Stage
		ExpectedResult   string
		ExpectedRequests []ldmigration.Origin
	}{
		// Read operations
		{Operation: ldmigration.Read, Stage: ldmigration.Off, ExpectedResult: "old read", ExpectedRequests: []ldmigration.Origin{ldmigration.Old}},
		{Operation: ldmigration.Read, Stage: ldmigration.DualWrite, ExpectedResult: "old read", ExpectedRequests: []ldmigration.Origin{ldmigration.Old}},
		{Operation: ldmigration.Read, Stage: ldmigration.Shadow, ExpectedResult: "old read", ExpectedRequests: []ldmigration.Origin{ldmigration.Old, ldmigration.New}},
		{Operation: ldmigration.Read, Stage: ldmigration.Live, ExpectedResult: "new read", ExpectedRequests: []ldmigration.Origin{ldmigration.New, ldmigration.Old}},
		{Operation: ldmigration.Read, Stage: ldmigration.RampDown, ExpectedResult: "new read", ExpectedRequests: []ldmigration.Origin{ldmigration.New}},
		{Operation: ldmigration.Read, Stage: ldmigration.Complete, ExpectedResult: "new read", ExpectedRequests: []ldmigration.Origin{ldmigration.New}},

		// Write operations
		{Operation: ldmigration.Write, Stage: ldmigration.Off, ExpectedResult: "old read", ExpectedRequests: []ldmigration.Origin{ldmigration.Old}},
		{Operation: ldmigration.Write, Stage: ldmigration.DualWrite, ExpectedResult: "old read", ExpectedRequests: []ldmigration.Origin{ldmigration.Old, ldmigration.New}},
		{Operation: ldmigration.Write, Stage: ldmigration.Shadow, ExpectedResult: "old read", ExpectedRequests: []ldmigration.Origin{ldmigration.Old, ldmigration.New}},
		{Operation: ldmigration.Write, Stage: ldmigration.Live, ExpectedResult: "new read", ExpectedRequests: []ldmigration.Origin{ldmigration.New, ldmigration.Old}},
		{Operation: ldmigration.Write, Stage: ldmigration.RampDown, ExpectedResult: "new read", ExpectedRequests: []ldmigration.Origin{ldmigration.New, ldmigration.Old}},
		{Operation: ldmigration.Write, Stage: ldmigration.Complete, ExpectedResult: "new read", ExpectedRequests: []ldmigration.Origin{ldmigration.New}},
	}

	for _, testParam := range testParams {
		t.Run(fmt.Sprintf("%s %s", testParam.Operation, testParam.Stage), func(t *ldtest.T) {
			client, _ := createClient(t, int(testParam.Stage))

			service := mockld.NewMigrationCallbackService(
				requireContext(t).harness,
				t.DebugLogger(),
				func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("old read"))
				},
				func(w http.ResponseWriter, req *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("new read"))
				},
			)
			t.Defer(service.Close)

			context := ldcontext.New("key")

			params := servicedef.MigrationOperationParams{
				Key:                "migration-key",
				Context:            context,
				DefaultStage:       ldmigration.Off,
				ReadExecutionOrder: ldmigration.Serial, // NOTE: Execute this in serial so we can verify request order as well
				Operation:          testParam.Operation,
				OldEndpoint:        service.OldEndpoint().BaseURL(),
				NewEndpoint:        service.NewEndpoint().BaseURL(),
			}

			response := client.MigrationOperation(t, params)

			if slices.Contains(testParam.ExpectedRequests, ldmigration.Old) {
				service.OldEndpoint().RequireConnection(t, 10*time.Millisecond)
			} else {
				service.OldEndpoint().RequireNoMoreConnections(t, 10*time.Millisecond)
			}

			if slices.Contains(testParam.ExpectedRequests, ldmigration.New) {
				service.NewEndpoint().RequireConnection(t, 10*time.Millisecond)
			} else {
				service.NewEndpoint().RequireNoMoreConnections(t, 10*time.Millisecond)
			}

			for idx, callHistory := range service.GetCallHistory() {
				assert.Equal(t, testParam.ExpectedRequests[idx], callHistory.GetOrigin())
			}

			assert.Equal(t, testParam.ExpectedResult, response.Result)
		})
	}
}

func runTrackLatencyTests(t *ldtest.T) {
	onlyOld := []m.Matcher{m.JSONOptProperty("old").Should(m.Not(m.BeNil())), m.JSONOptProperty("new").Should(m.BeNil())}
	both := []m.Matcher{m.JSONOptProperty("old").Should(m.Not(m.BeNil())), m.JSONOptProperty("new").Should(m.Not(m.BeNil()))}
	onlyNew := []m.Matcher{m.JSONOptProperty("old").Should(m.BeNil()), m.JSONOptProperty("new").Should(m.Not(m.BeNil()))}

	testParams := []struct {
		Operation      ldmigration.Operation
		Stage          ldmigration.Stage
		ValuesMatchers []m.Matcher
	}{
		// Read operations
		{Operation: ldmigration.Read, Stage: ldmigration.Off, ValuesMatchers: onlyOld},
		{Operation: ldmigration.Read, Stage: ldmigration.DualWrite, ValuesMatchers: onlyOld},
		{Operation: ldmigration.Read, Stage: ldmigration.Shadow, ValuesMatchers: both},
		{Operation: ldmigration.Read, Stage: ldmigration.Live, ValuesMatchers: both},
		{Operation: ldmigration.Read, Stage: ldmigration.RampDown, ValuesMatchers: onlyNew},
		{Operation: ldmigration.Read, Stage: ldmigration.Complete, ValuesMatchers: onlyNew},

		// Write operations
		{Operation: ldmigration.Write, Stage: ldmigration.Off, ValuesMatchers: onlyOld},
		{Operation: ldmigration.Write, Stage: ldmigration.DualWrite, ValuesMatchers: both},
		{Operation: ldmigration.Write, Stage: ldmigration.Shadow, ValuesMatchers: both},
		{Operation: ldmigration.Write, Stage: ldmigration.Live, ValuesMatchers: both},
		{Operation: ldmigration.Write, Stage: ldmigration.RampDown, ValuesMatchers: both},
		{Operation: ldmigration.Write, Stage: ldmigration.Complete, ValuesMatchers: onlyNew},
	}

	for _, testParam := range testParams {
		t.Run(fmt.Sprintf("%s latency for %s", testParam.Operation, testParam.Stage), func(t *ldtest.T) {
			client, events := createClient(t, int(testParam.Stage))

			callback := func(w http.ResponseWriter, req *http.Request) {
				time.Sleep(10 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("result"))
			}

			service := mockld.NewMigrationCallbackService(requireContext(t).harness, t.DebugLogger(), callback, callback)
			t.Defer(service.Close)

			context := ldcontext.New("key")

			params := servicedef.MigrationOperationParams{
				Key:                "migration-key",
				Context:            context,
				DefaultStage:       ldmigration.DualWrite,
				ReadExecutionOrder: ldmigration.Concurrent,
				OldEndpoint:        service.OldEndpoint().BaseURL(),
				NewEndpoint:        service.NewEndpoint().BaseURL(),
				Operation:          testParam.Operation,
				TrackLatency:       true,
			}

			_ = client.MigrationOperation(t, params)
			client.FlushEvents(t)

			opEventMatchers := []m.Matcher{
				m.JSONOptProperty("samplingRatio").Should(m.BeNil()),
				m.JSONProperty("operation").Should(m.Equal(testParam.Operation.String())),
				m.JSONProperty("evaluation").Should(
					m.AllOf(
						m.JSONProperty("key").Should(m.Equal("migration-key")),
						m.JSONProperty("default").Should(m.Equal("dualwrite")),
						m.JSONProperty("value").Should(m.Equal(testParam.Stage.String())),
						m.JSONProperty("variation").Should(m.Equal(int(testParam.Stage))),
						m.JSONProperty("reason").Should(
							m.JSONProperty("kind").Should(m.Equal("FALLTHROUGH")),
						),
					),
				),
				m.JSONProperty("measurements").Should(
					m.ItemsInAnyOrder(
						m.AllOf(
							m.JSONProperty("key").Should(m.Equal("latency_ms")),
							m.JSONProperty("values").Should(
								m.AllOf(
									testParam.ValuesMatchers...,
								),
							),
						),
					),
				),
			}

			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				IsIndexEventForContext(context),
				IsSummaryEvent(),
				IsValidMigrationOpEventWithConditions(
					context,
					opEventMatchers...,
				),
			))
		})
	}
}

func runTrackErrorsTests(t *ldtest.T) {
	hasError := func(label string) m.Matcher { return m.JSONOptProperty(label).Should(m.Equal(true)) }
	noError := func(label string) m.Matcher { return m.JSONOptProperty(label).Should(m.Equal(false)) }
	isMissing := func(label string) m.Matcher { return m.JSONOptProperty(label).Should(m.BeNil()) }

	failureHandler := func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusConflict) }
	successfulHandler := func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }

	testParams := []struct {
		Operation      ldmigration.Operation
		Stage          ldmigration.Stage
		ValuesMatchers []m.Matcher
		OldHandler     http.HandlerFunc
		NewHandler     http.HandlerFunc
	}{
		// Read operations
		{Operation: ldmigration.Read, Stage: ldmigration.Off, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{noError("old"), isMissing("new")}},
		{Operation: ldmigration.Read, Stage: ldmigration.DualWrite, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{noError("old"), isMissing("new")}},
		{Operation: ldmigration.Read, Stage: ldmigration.Shadow, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{noError("old"), noError("new")}},
		{Operation: ldmigration.Read, Stage: ldmigration.Live, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{noError("old"), noError("new")}},
		{Operation: ldmigration.Read, Stage: ldmigration.RampDown, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{isMissing("old"), noError("new")}},
		{Operation: ldmigration.Read, Stage: ldmigration.Complete, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{isMissing("old"), noError("new")}},

		// Write operations
		{Operation: ldmigration.Write, Stage: ldmigration.Off, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{noError("old"), isMissing("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.DualWrite, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{noError("old"), noError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Shadow, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{noError("old"), noError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Live, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{noError("old"), noError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.RampDown, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{noError("old"), noError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Complete, OldHandler: successfulHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{isMissing("old"), noError("new")}},

		// Write operations with authoritative failure
		{Operation: ldmigration.Write, Stage: ldmigration.Off, OldHandler: failureHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{hasError("old"), isMissing("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.DualWrite, OldHandler: failureHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{hasError("old"), isMissing("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Shadow, OldHandler: failureHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{hasError("old"), isMissing("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Live, OldHandler: successfulHandler, NewHandler: failureHandler, ValuesMatchers: []m.Matcher{isMissing("old"), hasError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.RampDown, OldHandler: successfulHandler, NewHandler: failureHandler, ValuesMatchers: []m.Matcher{isMissing("old"), hasError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Complete, OldHandler: successfulHandler, NewHandler: failureHandler, ValuesMatchers: []m.Matcher{isMissing("old"), hasError("new")}},

		// Write operations with non-authoritative failure
		{Operation: ldmigration.Write, Stage: ldmigration.Off, OldHandler: successfulHandler, NewHandler: failureHandler, ValuesMatchers: []m.Matcher{noError("old"), isMissing("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.DualWrite, OldHandler: successfulHandler, NewHandler: failureHandler, ValuesMatchers: []m.Matcher{noError("old"), hasError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Shadow, OldHandler: successfulHandler, NewHandler: failureHandler, ValuesMatchers: []m.Matcher{noError("old"), hasError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Live, OldHandler: failureHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{hasError("old"), noError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.RampDown, OldHandler: failureHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{hasError("old"), noError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Complete, OldHandler: failureHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{isMissing("old"), noError("new")}},
	}

	for _, testParam := range testParams {
		t.Run(fmt.Sprintf("%s errors for %s", testParam.Operation, testParam.Stage), func(t *ldtest.T) {
			client, events := createClient(t, int(testParam.Stage))

			service := mockld.NewMigrationCallbackService(requireContext(t).harness, t.DebugLogger(), testParam.OldHandler, testParam.NewHandler)
			t.Defer(service.Close)

			context := ldcontext.New("key")

			params := servicedef.MigrationOperationParams{
				Key:                "migration-key",
				Context:            context,
				DefaultStage:       ldmigration.DualWrite,
				ReadExecutionOrder: ldmigration.Concurrent,
				OldEndpoint:        service.OldEndpoint().BaseURL(),
				NewEndpoint:        service.NewEndpoint().BaseURL(),
				Operation:          testParam.Operation,
				TrackErrors:        true,
			}

			_ = client.MigrationOperation(t, params)
			client.FlushEvents(t)

			opEventMatchers := []m.Matcher{
				m.JSONOptProperty("samplingRatio").Should(m.BeNil()),
				m.JSONProperty("operation").Should(m.Equal(testParam.Operation.String())),
				m.JSONProperty("evaluation").Should(
					m.AllOf(
						m.JSONProperty("key").Should(m.Equal("migration-key")),
						m.JSONProperty("default").Should(m.Equal("dualwrite")),
						m.JSONProperty("value").Should(m.Equal(testParam.Stage.String())),
						m.JSONProperty("variation").Should(m.Equal(int(testParam.Stage))),
						m.JSONProperty("reason").Should(
							m.JSONProperty("kind").Should(m.Equal("FALLTHROUGH")),
						),
					),
				),
				m.JSONProperty("measurements").Should(
					m.ItemsInAnyOrder(
						m.AllOf(
							m.JSONProperty("key").Should(m.Equal("errors")),
							m.JSONProperty("values").Should(
								m.AllOf(
									testParam.ValuesMatchers...,
								),
							),
						),
					),
				),
			}

			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				IsIndexEventForContext(context),
				IsSummaryEvent(),
				IsValidMigrationOpEventWithConditions(
					context,
					opEventMatchers...,
				),
			))
		})
	}
}

func runTrackConsistencyTests(t *ldtest.T) {
	handler := func(response string) func(w http.ResponseWriter, req *http.Request) {
		return func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(response))
		}
	}

	testParams := []struct {
		Operation    ldmigration.Operation
		Stage        ldmigration.Stage
		IsConsistent ldvalue.OptionalBool
		OldHandler   http.HandlerFunc
		NewHandler   http.HandlerFunc
	}{
		// Read operations
		{Operation: ldmigration.Read, Stage: ldmigration.Off, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBoolFromPointer(nil)},
		{Operation: ldmigration.Read, Stage: ldmigration.DualWrite, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBoolFromPointer(nil)},

		{Operation: ldmigration.Read, Stage: ldmigration.Shadow, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBool(true)},
		{Operation: ldmigration.Read, Stage: ldmigration.Shadow, OldHandler: handler("LaunchDarkly"), NewHandler: handler("Catamorphic"), IsConsistent: ldvalue.NewOptionalBool(false)},
		{Operation: ldmigration.Read, Stage: ldmigration.Shadow, OldHandler: handler("Catamorphic"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBool(false)},

		{Operation: ldmigration.Read, Stage: ldmigration.Live, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBool(true)},
		{Operation: ldmigration.Read, Stage: ldmigration.Live, OldHandler: handler("LaunchDarkly"), NewHandler: handler("Catamorphic"), IsConsistent: ldvalue.NewOptionalBool(false)},
		{Operation: ldmigration.Read, Stage: ldmigration.Live, OldHandler: handler("Catamorphic"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBool(false)},

		{Operation: ldmigration.Read, Stage: ldmigration.RampDown, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBoolFromPointer(nil)},
		{Operation: ldmigration.Read, Stage: ldmigration.Complete, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBoolFromPointer(nil)},

		// Write operations -- we never run a consistency check for this
		{Operation: ldmigration.Write, Stage: ldmigration.Off, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBoolFromPointer(nil)},
		{Operation: ldmigration.Write, Stage: ldmigration.DualWrite, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBoolFromPointer(nil)},
		{Operation: ldmigration.Write, Stage: ldmigration.Shadow, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBoolFromPointer(nil)},
		{Operation: ldmigration.Write, Stage: ldmigration.Live, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBoolFromPointer(nil)},
		{Operation: ldmigration.Write, Stage: ldmigration.RampDown, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBoolFromPointer(nil)},
		{Operation: ldmigration.Write, Stage: ldmigration.Complete, OldHandler: handler("LaunchDarkly"), NewHandler: handler("LaunchDarkly"), IsConsistent: ldvalue.NewOptionalBoolFromPointer(nil)},
	}

	for _, testParam := range testParams {
		t.Run(fmt.Sprintf("%s errors for %s", testParam.Operation, testParam.Stage), func(t *ldtest.T) {
			client, events := createClient(t, int(testParam.Stage))

			service := mockld.NewMigrationCallbackService(requireContext(t).harness, t.DebugLogger(), testParam.OldHandler, testParam.NewHandler)
			t.Defer(service.Close)

			context := ldcontext.New("key")

			params := servicedef.MigrationOperationParams{
				Key:                "migration-key",
				Context:            context,
				DefaultStage:       ldmigration.DualWrite,
				ReadExecutionOrder: ldmigration.Concurrent,
				OldEndpoint:        service.OldEndpoint().BaseURL(),
				NewEndpoint:        service.NewEndpoint().BaseURL(),
				Operation:          testParam.Operation,
				TrackConsistency:   true,
			}

			_ = client.MigrationOperation(t, params)
			client.FlushEvents(t)

			var matcher m.Matcher

			if consistent, ok := testParam.IsConsistent.Get(); ok {
				matcher = m.ItemsInAnyOrder(
					m.AllOf(
						m.JSONProperty("key").Should(m.Equal("consistent")),
						m.JSONProperty("value").Should(m.Equal(consistent)),
						m.JSONProperty("samplingRatio").Should(m.Equal(1)),
					),
				)
			} else {
				matcher = m.Length().Should(m.Equal(0))
			}

			opEventMatchers := []m.Matcher{
				m.JSONOptProperty("samplingRatio").Should(m.BeNil()),
				m.JSONProperty("operation").Should(m.Equal(testParam.Operation.String())),
				m.JSONProperty("evaluation").Should(
					m.AllOf(
						m.JSONProperty("key").Should(m.Equal("migration-key")),
						m.JSONProperty("default").Should(m.Equal("dualwrite")),
						m.JSONProperty("value").Should(m.Equal(testParam.Stage.String())),
						m.JSONProperty("variation").Should(m.Equal(int(testParam.Stage))),
						m.JSONProperty("reason").Should(
							m.JSONProperty("kind").Should(m.Equal("FALLTHROUGH")),
						),
					),
				),
				m.JSONProperty("measurements").Should(matcher),
			}

			payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
			m.In(t).Assert(payload, m.ItemsInAnyOrder(
				IsIndexEventForContext(context),
				IsSummaryEvent(),
				IsValidMigrationOpEventWithConditions(
					context,
					opEventMatchers...,
				),
			))
		})
	}
}

func createClient(t *ldtest.T, variationIndex int) (*SDKClient, *SDKEventSink) {
	migrationFlag := ldbuilders.NewFlagBuilder("migration-key").On(true).Variations(data.MakeStandardMigrationStages()...).FallthroughVariation(variationIndex).Build()
	dataBuilder := mockld.NewServerSDKDataBuilder()
	dataBuilder.Flag(migrationFlag)

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSink(t)
	client := NewSDKClient(t, dataSource, events)

	return client, events
}

func stageToVariationIndex(stage ldmigration.Stage) int {
	switch stage {
	case ldmigration.Off:
		return 0
	case ldmigration.DualWrite:
		return 1
	case ldmigration.Shadow:
		return 2
	case ldmigration.Live:
		return 3
	case ldmigration.RampDown:
		return 4
	case ldmigration.Complete:
		return 5
	default:
		return 0
	}
}
