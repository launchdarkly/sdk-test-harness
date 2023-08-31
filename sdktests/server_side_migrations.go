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

	t.Run("identifies correct stage from flag", identifyCorrectStageFromStringFlag)
	t.Run("executes reads", executesReads)
	t.Run("executes origins in correct order", executesOriginsInCorrectOrder)
	t.Run("tracks invoked", tracksInvoked)
	t.Run("tracks latency", tracksLatency)
	t.Run("tracks error metrics for failures", writeFailuresShouldGenerateErrorMetrics)
	t.Run("tracks no errors on success", successfulHandlersShouldNotGenerateErrorMetrics)
	t.Run("tracks consistency", trackConsistency)
}

func identifyCorrectStageFromStringFlag(t *ldtest.T) {
	stages := []ldmigration.Stage{ldmigration.Off, ldmigration.DualWrite, ldmigration.Shadow, ldmigration.Live, ldmigration.RampDown, ldmigration.Complete}

	for _, stage := range stages {
		client, events := createClient(t, stageToVariationIndex(stage))
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

		assert.EqualValues(t, stage, response.Result)
	}
}

func executesOriginsInCorrectOrder(t *ldtest.T) {
	testParams := []struct {
		Operation        ldmigration.Operation
		Stage            ldmigration.Stage
		ExpectedResult   string
		ExpectedRequests []ldmigration.Origin
	}{

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
			client, _ := createClient(t, stageToVariationIndex(testParam.Stage))

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

func executesReads(t *ldtest.T) {
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
	}

	for _, testParam := range testParams {
		t.Run(fmt.Sprintf("%s %s", testParam.Operation, testParam.Stage), func(t *ldtest.T) {
			client, _ := createClient(t, stageToVariationIndex(testParam.Stage))

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

			for _, callHistory := range service.GetCallHistory() {
				assert.Contains(t, testParam.ExpectedRequests, callHistory.GetOrigin())
			}

			assert.Equal(t, len(testParam.ExpectedRequests), len(service.GetCallHistory()))
			assert.Equal(t, testParam.ExpectedResult, response.Result)
		})
	}
}

func tracksInvoked(t *ldtest.T) {
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
		t.Run(fmt.Sprintf("%s invoked for %s", testParam.Operation, testParam.Stage), func(t *ldtest.T) {
			client, events := createClient(t, stageToVariationIndex(testParam.Stage))

			callback := func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }

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
			}

			_ = client.MigrationOperation(t, params)
			client.FlushEvents(t)

			opEventMatchers := []m.Matcher{
				m.JSONOptProperty("samplingRatio").Should(m.BeNil()),
				m.JSONProperty("operation").Should(m.Equal(string(testParam.Operation))),
				m.JSONProperty("evaluation").Should(
					m.AllOf(
						m.JSONProperty("key").Should(m.Equal("migration-key")),
						m.JSONProperty("default").Should(m.Equal("dualwrite")),
						m.JSONProperty("value").Should(m.Equal(string(testParam.Stage))),
						m.JSONProperty("variation").Should(m.Equal(stageToVariationIndex(testParam.Stage))),
						m.JSONProperty("reason").Should(
							m.JSONProperty("kind").Should(m.Equal("FALLTHROUGH")),
						),
					),
				),
				m.JSONProperty("measurements").Should(
					m.ItemsInAnyOrder(
						m.AllOf(
							m.JSONProperty("key").Should(m.Equal("invoked")),
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

func tracksLatency(t *ldtest.T) {
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
			client, events := createClient(t, stageToVariationIndex(testParam.Stage))

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
				m.JSONProperty("operation").Should(m.Equal(string(testParam.Operation))),
				m.JSONProperty("evaluation").Should(
					m.AllOf(
						m.JSONProperty("key").Should(m.Equal("migration-key")),
						m.JSONProperty("default").Should(m.Equal("dualwrite")),
						m.JSONProperty("value").Should(m.Equal(string(testParam.Stage))),
						m.JSONProperty("variation").Should(m.Equal(stageToVariationIndex(testParam.Stage))),
						m.JSONProperty("reason").Should(
							m.JSONProperty("kind").Should(m.Equal("FALLTHROUGH")),
						),
					),
				),
				m.JSONProperty("measurements").Should(
					m.ItemsInAnyOrder(
						m.JSONProperty("key").Should(m.Equal("invoked")),
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

func writeFailuresShouldGenerateErrorMetrics(t *ldtest.T) {
	hasError := func(label string) m.Matcher { return m.JSONOptProperty(label).Should(m.Equal(true)) }
	isMissingOrNoError := func(label string) m.Matcher { return JSONPropertyNullOrAbsentOrEqualTo(label, false) }

	failureHandler := func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusConflict) }
	successfulHandler := func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }

	testParams := []struct {
		Operation      ldmigration.Operation
		Stage          ldmigration.Stage
		ValuesMatchers []m.Matcher
		OldHandler     http.HandlerFunc
		NewHandler     http.HandlerFunc
	}{
		// Write operations with authoritative failure
		{Operation: ldmigration.Write, Stage: ldmigration.Off, OldHandler: failureHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{hasError("old"), isMissingOrNoError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.DualWrite, OldHandler: failureHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{hasError("old"), isMissingOrNoError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Shadow, OldHandler: failureHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{hasError("old"), isMissingOrNoError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Live, OldHandler: successfulHandler, NewHandler: failureHandler, ValuesMatchers: []m.Matcher{isMissingOrNoError("old"), hasError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.RampDown, OldHandler: successfulHandler, NewHandler: failureHandler, ValuesMatchers: []m.Matcher{isMissingOrNoError("old"), hasError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Complete, OldHandler: successfulHandler, NewHandler: failureHandler, ValuesMatchers: []m.Matcher{isMissingOrNoError("old"), hasError("new")}},

		// Write operations with non-authoritative failure
		{Operation: ldmigration.Write, Stage: ldmigration.DualWrite, OldHandler: successfulHandler, NewHandler: failureHandler, ValuesMatchers: []m.Matcher{isMissingOrNoError("old"), hasError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Shadow, OldHandler: successfulHandler, NewHandler: failureHandler, ValuesMatchers: []m.Matcher{isMissingOrNoError("old"), hasError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.Live, OldHandler: failureHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{hasError("old"), isMissingOrNoError("new")}},
		{Operation: ldmigration.Write, Stage: ldmigration.RampDown, OldHandler: failureHandler, NewHandler: successfulHandler, ValuesMatchers: []m.Matcher{hasError("old"), isMissingOrNoError("new")}},
	}

	for _, testParam := range testParams {
		t.Run(fmt.Sprintf("%s errors for %s", testParam.Operation, testParam.Stage), func(t *ldtest.T) {
			client, events := createClient(t, stageToVariationIndex(testParam.Stage))

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
				m.JSONProperty("operation").Should(m.Equal(string(testParam.Operation))),
				m.JSONProperty("evaluation").Should(
					m.AllOf(
						m.JSONProperty("key").Should(m.Equal("migration-key")),
						m.JSONProperty("default").Should(m.Equal("dualwrite")),
						m.JSONProperty("value").Should(m.Equal(string(testParam.Stage))),
						m.JSONProperty("variation").Should(m.Equal(stageToVariationIndex(testParam.Stage))),
						m.JSONProperty("reason").Should(
							m.JSONProperty("kind").Should(m.Equal("FALLTHROUGH")),
						),
					),
				),
				m.JSONProperty("measurements").Should(
					m.ItemsInAnyOrder(
						m.JSONProperty("key").Should(m.Equal("invoked")),
						m.AllOf(
							m.JSONProperty("key").Should(m.Equal("error")),
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

func successfulHandlersShouldNotGenerateErrorMetrics(t *ldtest.T) {
	successfulHandler := func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }

	testParams := []struct {
		Operation ldmigration.Operation
		Stage     ldmigration.Stage
	}{
		// Read operations
		{Operation: ldmigration.Read, Stage: ldmigration.Off},
		{Operation: ldmigration.Read, Stage: ldmigration.DualWrite},
		{Operation: ldmigration.Read, Stage: ldmigration.Shadow},
		{Operation: ldmigration.Read, Stage: ldmigration.Live},
		{Operation: ldmigration.Read, Stage: ldmigration.RampDown},
		{Operation: ldmigration.Read, Stage: ldmigration.Complete},

		// Write operations
		{Operation: ldmigration.Write, Stage: ldmigration.Off},
		{Operation: ldmigration.Write, Stage: ldmigration.DualWrite},
		{Operation: ldmigration.Write, Stage: ldmigration.Shadow},
		{Operation: ldmigration.Write, Stage: ldmigration.Live},
		{Operation: ldmigration.Write, Stage: ldmigration.RampDown},
		{Operation: ldmigration.Write, Stage: ldmigration.Complete},
	}

	for _, testParam := range testParams {
		t.Run(fmt.Sprintf("%s errors for %s", testParam.Operation, testParam.Stage), func(t *ldtest.T) {
			client, events := createClient(t, stageToVariationIndex(testParam.Stage))

			service := mockld.NewMigrationCallbackService(requireContext(t).harness, t.DebugLogger(), successfulHandler, successfulHandler)
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
				m.JSONProperty("operation").Should(m.Equal(string(testParam.Operation))),
				m.JSONProperty("evaluation").Should(
					m.AllOf(
						m.JSONProperty("key").Should(m.Equal("migration-key")),
						m.JSONProperty("default").Should(m.Equal("dualwrite")),
						m.JSONProperty("value").Should(m.Equal(string(testParam.Stage))),
						m.JSONProperty("variation").Should(m.Equal(stageToVariationIndex(testParam.Stage))),
						m.JSONProperty("reason").Should(
							m.JSONProperty("kind").Should(m.Equal("FALLTHROUGH")),
						),
					),
				),
				m.JSONProperty("measurements").Should(m.Length().Should(m.Equal(1))),
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

func trackConsistency(t *ldtest.T) {
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
			client, events := createClient(t, stageToVariationIndex(testParam.Stage))

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
					m.JSONProperty("key").Should(m.Equal("invoked")),
					m.AllOf(
						m.JSONProperty("key").Should(m.Equal("consistent")),
						m.JSONProperty("value").Should(m.Equal(consistent)),
						m.JSONProperty("samplingRatio").Should(m.Equal(1)),
					),
				)
			} else {
				matcher = m.Length().Should(m.Equal(1))
			}

			opEventMatchers := []m.Matcher{
				m.JSONOptProperty("samplingRatio").Should(m.BeNil()),
				m.JSONProperty("operation").Should(m.Equal(string(testParam.Operation))),
				m.JSONProperty("evaluation").Should(
					m.AllOf(
						m.JSONProperty("key").Should(m.Equal("migration-key")),
						m.JSONProperty("default").Should(m.Equal("dualwrite")),
						m.JSONProperty("value").Should(m.Equal(string(testParam.Stage))),
						m.JSONProperty("variation").Should(m.Equal(stageToVariationIndex(testParam.Stage))),
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
