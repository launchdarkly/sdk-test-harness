package sdktests

import (
	"net/http"
	"time"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/v3/data"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// experimentExposureFlagKey is the key of the only flag in the data of these tests.
const experimentExposureFlagKey = "experiment-flag"

// experimentExposureUpdateVersion is the payload-transferred version of a stream update. The
// initial payload has version 1.
const experimentExposureUpdateVersion = 2

// experimentExposureFlagBefore is the flag before it joins an experiment. The flag does not
// track events, so an evaluation of it gives no feature event.
func experimentExposureFlagBefore() mockld.ClientSDKFlag {
	return mockld.ClientSDKFlag{
		Value:       ldvalue.String("control"),
		Variation:   o.Some(0),
		Reason:      o.Some(ldreason.NewEvalReasonFallthrough()),
		Version:     10,
		FlagVersion: o.Some(1),
	}
}

// experimentExposureFlagAfter is the flag after it joins an experiment. The value and the
// variation do not change. The reason, the versions, and the event tracking fields change.
func experimentExposureFlagAfter() mockld.ClientSDKFlag {
	flag := experimentExposureFlagBefore()
	flag.Reason = o.Some(ldreason.NewEvalReasonFallthroughExperiment(true))
	flag.Version = 11
	flag.FlagVersion = o.Some(2)
	flag.TrackEvents = true
	flag.TrackReason = true
	return flag
}

// experimentExposureData is the client-side data set that contains only the given flag.
func experimentExposureData(flag mockld.ClientSDKFlag) mockld.ClientSDKData {
	return mockld.NewClientSDKDataBuilder().Flag(experimentExposureFlagKey, flag).Build()
}

// experimentExposureSource is a set of mock services that serve the flag before it joins the
// experiment, and that can deliver one update of the flag to the SDK.
type experimentExposureSource struct {
	// configurers point the SDK client to the mock services.
	configurers []SDKConfigurer
	// deliver sends the update. It returns when the update is on its way to the SDK.
	deliver func(t *ldtest.T)
}

// experimentExposurePath is one way for a flag update to get to the SDK.
type experimentExposurePath struct {
	name string
	// capability is a capability that the path needs in addition to those of the suite. It is
	// empty if the path needs no other capability.
	capability string
	// longRunning is true if the path must wait for a second poll request.
	longRunning bool
	// newSource creates the mock services. The services deliver the updated flag as the update.
	newSource func(t *ldtest.T, updated mockld.ClientSDKFlag) experimentExposureSource
}

func experimentExposurePaths() []experimentExposurePath {
	return []experimentExposurePath{
		{name: "FDv2 stream partial payload", newSource: newExperimentExposureStreamSource(false)},
		{name: "FDv2 stream full payload", newSource: newExperimentExposureStreamSource(true)},
		{name: "FDv2 polling synchronizer", longRunning: true, newSource: newExperimentExposureFDv2PollSource},
		{
			name:        "FDv1 fallback synchronizer",
			capability:  servicedef.CapabilityFDv1Fallback,
			longRunning: true,
			newSource:   newExperimentExposureFDv1PollSource,
		},
	}
}

// doClientSideExperimentExposureTests checks that a client-side SDK notifies its general flag
// change listener when a flag joins an experiment and the value of the flag does not change.
// Wrappers evaluate a flag again only after a notification. If the SDK does not notify, the
// experiment does not get the exposure events of the users that stay on the same variation.
func doClientSideExperimentExposureTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityFlagChangeListeners)
	t.RequireCapability(servicedef.CapabilityExperimentExposureChange)

	for _, path := range experimentExposurePaths() {
		t.Run(path.name, func(t *ldtest.T) {
			if path.capability != "" {
				t.RequireCapability(path.capability)
			}
			if path.longRunning {
				t.LongRunning()
			}
			t.Run("notifies when flag joins experiment", func(t *ldtest.T) {
				experimentExposureJoinsExperiment(t, path)
			})
			t.Run("does not notify for identical update", func(t *ldtest.T) {
				experimentExposureIdenticalUpdate(t, path)
			})
		})
	}
}

// experimentExposureFixture is an SDK client that has the flag before it joins the experiment,
// with a general flag change listener and an event sink.
type experimentExposureFixture struct {
	client    *SDKClient
	context   ldcontext.Context
	eventSink *SDKEventSink
	callback  *ListenerCallback
	source    experimentExposureSource
}

func newExperimentExposureFixture(
	t *ldtest.T,
	path experimentExposurePath,
	updated mockld.ClientSDKFlag,
) *experimentExposureFixture {
	context := data.NewContextFactory("experiment-exposure").NextUniqueContext()
	source := path.newSource(t, updated)
	eventSink := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))

	// The long flush interval keeps all events in one payload until the test flushes them.
	configurers := make([]SDKConfigurer, 0, 3+len(source.configurers))
	configurers = append(configurers,
		WithClientSideInitialContext(context),
		WithEventsConfig(baseEventsConfig()),
		eventSink,
	)
	configurers = append(configurers, source.configurers...)
	client := NewSDKClient(t, configurers...)

	return &experimentExposureFixture{
		client:    client,
		context:   context,
		eventSink: eventSink,
		callback:  registerFlagChangeListener(t, client),
		source:    source,
	}
}

func experimentExposureJoinsExperiment(t *ldtest.T, path experimentExposurePath) {
	updated := experimentExposureFlagAfter()
	defaultValue := ldvalue.String("default")
	f := newExperimentExposureFixture(t, path, updated)

	f.source.deliver(t)
	f.callback.ExpectFlagChangeNotification(t, experimentExposureFlagKey)

	// Before the update, the flag does not track events. Thus the only feature event comes from
	// this evaluation, and it must have the reason and the flag version of the update.
	value := basicEvaluateFlag(t, f.client, experimentExposureFlagKey, f.context, defaultValue)
	m.In(t).Assert(value, m.JSONEqual(updated.Value))

	f.client.FlushEvents(t)
	payload := f.eventSink.ExpectAnalyticsEvents(t, time.Second)

	matchFeatureEvent := IsValidFeatureEventWithConditions(
		t, false, f.context,
		m.JSONProperty("key").Should(m.Equal(experimentExposureFlagKey)),
		m.JSONProperty("version").Should(m.Equal(updated.FlagVersion.Value())),
		m.JSONProperty("value").Should(m.JSONEqual(updated.Value)),
		m.JSONProperty("variation").Should(m.Equal(updated.Variation.Value())),
		m.JSONProperty("reason").Should(m.JSONEqual(updated.Reason.Value())),
		m.JSONProperty("default").Should(m.JSONEqual(defaultValue)),
		JSONPropertyNullOrAbsent("prereqOf"),
	)
	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIdentifyEvent(),
		matchFeatureEvent,
		IsSummaryEvent(),
	))
}

// experimentExposureIdenticalUpdate is the control test. The update keeps the versions of the
// flag on purpose, because a new version is a real change that the general listener can report.
// Thus the test checks that the SDK does not notify for data that it already has.
func experimentExposureIdenticalUpdate(t *ldtest.T, path experimentExposurePath) {
	f := newExperimentExposureFixture(t, path, experimentExposureFlagBefore())

	f.source.deliver(t)
	f.callback.ExpectNoNotification(t, experimentExposureFlagKey)
}

// newExperimentExposureStreamSource returns a function that creates a source that pushes the
// update on the FDv2 stream. If full is true, the update is a full payload (xfer-full) that
// replaces all data. If full is false, the update is a partial payload that changes one flag.
func newExperimentExposureStreamSource(
	full bool,
) func(t *ldtest.T, updated mockld.ClientSDKFlag) experimentExposureSource {
	return func(t *ldtest.T, updated mockld.ClientSDKFlag) experimentExposureSource {
		dataSystem := NewSDKDataSystem(t, experimentExposureData(experimentExposureFlagBefore()))
		synchronizer := dataSystem.Synchronizers[0]
		flagJSON := jsonhelpers.ToJSON(mockld.ClientSDKFlagWithKey{
			ClientSDKFlag: updated,
			Key:           experimentExposureFlagKey,
		})

		return experimentExposureSource{
			configurers: []SDKConfigurer{dataSystem},
			deliver: func(t *ldtest.T) {
				// A missing stream connection fails here with a clear message, and not later
				// as a missing notification.
				requireStreamConnection(t, synchronizer.Endpoint(), time.Second*5)
				if full {
					synchronizer.streaming.PushServerIntent("xfer-full", "stale")
				}
				synchronizer.streaming.PushUpdate("flag", experimentExposureFlagKey, updated.Version, flagJSON)
				synchronizer.streaming.PushPayloadTransferred("updated", experimentExposureUpdateVersion)
			},
		}
	}
}

// experimentExposurePollWait is the longest time to wait for the next poll request when the
// SDK polls at the given interval.
func experimentExposurePollWait(interval time.Duration) time.Duration {
	return interval + pollIntervalEpsilon + 5*time.Second
}

// newExperimentExposureFDv2PollSource creates a source that serves the update from the FDv2
// polling synchronizer. Client-side SDKs clamp the poll interval to a minimum, so the source asks
// for the minimum interval.
func newExperimentExposureFDv2PollSource(t *ldtest.T, updated mockld.ClientSDKFlag) experimentExposureSource {
	interval := NewCommonPollingTests(t, "experimentExposure").pollIntervals(t).Min
	before := experimentExposureData(experimentExposureFlagBefore())
	dataSystem := NewSDKDataSystemCustom(t, before,
		DataSystemOptionConnectionMode("polling",
			DataSystemOptionPollingInitializer(before),
			DataSystemOptionPolling(),
			DataSystemOptionPollInterval(interval),
		),
		DataSystemOptionInitialConnectionMode("polling"),
	)
	dataSystem.CreateEndpoints()
	synchronizer := dataSystem.Synchronizers[0]
	update := mockld.FDv2SDKDataFromClientSDKData(experimentExposureData(updated), "xfer-full", "stale", "updated")

	return experimentExposureSource{
		configurers: []SDKConfigurer{dataSystem},
		deliver: func(t *ldtest.T) {
			// The first request can come before the test registers the listener, so the test
			// changes the data only after that request. The SDK gets the update on the first or
			// the second poll, always after the listener is registered. The second request makes
			// sure that the update is on its way to the SDK.
			synchronizer.Endpoint().RequireConnection(t, experimentExposurePollWait(interval))
			synchronizer.polling.SetData(update)
			synchronizer.Endpoint().RequireConnection(t, experimentExposurePollWait(interval))
		},
	}
}

// newExperimentExposureEndpoint creates a mock endpoint that closes when the test scope exits.
func newExperimentExposureEndpoint(t *ldtest.T, handler http.Handler, description string) *harness.MockEndpoint {
	endpoint := requireContext(t).harness.NewMockEndpoint(handler, t.DebugLogger(),
		harness.MockEndpointDescription(description))
	t.Defer(endpoint.Close)
	return endpoint
}

// newExperimentExposureFDv1PollSource creates a source that serves the update on the second
// request to the FDv1 fallback synchronizer. The SDK can send the first FDv1 request before the
// test registers the listener, so the first response is the flag before the update. The polling
// initializer serves the flag before the update together with the FDv1 fallback directive, so
// the SDK does not start the FDv2 synchronizer and polls the FDv1 endpoint.
func newExperimentExposureFDv1PollSource(t *ldtest.T, updated mockld.ClientSDKFlag) experimentExposureSource {
	interval := NewCommonPollingTests(t, "experimentExposure").pollIntervals(t).Min
	before := experimentExposureData(experimentExposureFlagBefore())

	initService := mockld.NewPollingService(
		before.ConvertToFDv2SDKClientData(t, "initial"), requireContext(t).sdkKind, t.DebugLogger())
	initEndpoint := newExperimentExposureEndpoint(t,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-LD-FD-Fallback", "true")
			initService.ServeHTTP(w, r)
		}),
		"polling initializer with FDv1 fallback directive")

	// The directive stops the FDv2 data sources, so the SDK does not use this endpoint.
	streamEndpoint := newExperimentExposureEndpoint(t, httphelpers.HandlerWithStatus(http.StatusInternalServerError),
		"streaming synchronizer (not used)")

	jsonHeader := http.Header{"Content-Type": []string{"application/json"}}
	fdv1Endpoint := newExperimentExposureEndpoint(t,
		httphelpers.SequentialHandler(
			httphelpers.HandlerWithResponse(http.StatusOK, jsonHeader, before.Serialize()),
			httphelpers.HandlerWithResponse(http.StatusOK, jsonHeader, experimentExposureData(updated).Serialize()),
		),
		"FDv1 fallback polling service")

	intervalMS := uint64(interval.Milliseconds()) //nolint:gosec // poll intervals are positive

	return experimentExposureSource{
		configurers: []SDKConfigurer{
			WithDataInitializer(servicedef.DataInitializer{
				Polling: o.Some(servicedef.SDKConfigPollingParams{BaseURI: initEndpoint.BaseURL()}),
			}),
			WithStreamingSynchronizer(servicedef.SDKConfigStreamingParams{BaseURI: streamEndpoint.BaseURL()}),
			WithFDv1Fallback(servicedef.SDKConfigPollingParams{
				BaseURI:        fdv1Endpoint.BaseURL(),
				PollIntervalMS: o.Some(ldtime.UnixMillisecondTime(intervalMS)),
			}),
			WithServiceEndpoints(servicedef.SDKConfigServiceEndpointsParams{
				Streaming: streamEndpoint.BaseURL(),
				Polling:   fdv1Endpoint.BaseURL(),
			}),
		},
		deliver: func(t *ldtest.T) {
			fdv1Endpoint.RequireConnection(t, experimentExposurePollWait(interval))
			fdv1Endpoint.RequireConnection(t, experimentExposurePollWait(interval))
		},
	}
}
