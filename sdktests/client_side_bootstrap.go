package sdktests

import (
	"time"

	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// This file covers the client-side consumer half of bootstrapping: an application hands the
// SDK pre-fetched flag data (the JSON produced by serializing a server-side SDK's
// allFlagsState result) at start() time, and the SDK must apply it to its flag store before
// making any network calls.

// bootstrapFlagMetadata is one entry in a bootstrap payload's "$flagsState" object. Each
// field is emitted only when it's set, so a zero-valued bootstrapFlagMetadata renders as an
// empty JSON object.
type bootstrapFlagMetadata struct {
	Variation            o.Maybe[int]
	Version              o.Maybe[int]
	TrackEvents          bool
	TrackReason          bool
	DebugEventsUntilDate o.Maybe[ldtime.UnixMillisecondTime]
	Reason               o.Maybe[ldreason.EvaluationReason]
	Prerequisites        []string
}

// toValue renders the metadata entry as a JSON object, omitting each field that isn't set.
func (meta *bootstrapFlagMetadata) toValue() ldvalue.Value {
	if meta == nil {
		return ldvalue.ObjectBuild().Build()
	}
	b := ldvalue.ObjectBuild()
	if meta.Variation.IsDefined() {
		b.SetInt("variation", meta.Variation.Value())
	}
	if meta.Version.IsDefined() {
		b.SetInt("version", meta.Version.Value())
	}
	if meta.TrackEvents {
		b.SetBool("trackEvents", true)
	}
	if meta.TrackReason {
		b.SetBool("trackReason", true)
	}
	if meta.DebugEventsUntilDate.IsDefined() {
		b.SetFloat64("debugEventsUntilDate", float64(meta.DebugEventsUntilDate.Value()))
	}
	if meta.Reason.IsDefined() {
		b.Set("reason", ldvalue.FromJSONMarshal(meta.Reason.Value()))
	}
	if len(meta.Prerequisites) > 0 {
		prereqs := ldvalue.ArrayBuild()
		for _, key := range meta.Prerequisites {
			prereqs.Add(ldvalue.String(key))
		}
		b.Set("prerequisites", prereqs.Build())
	}
	return b.Build()
}

// bootstrapPayloadBuilder builds a bootstrap object in the wire shape that serializing a
// server-side SDK's allFlagsState result produces.
type bootstrapPayloadBuilder struct {
	keys           []string
	values         map[string]ldvalue.Value
	metadata       map[string]*bootstrapFlagMetadata
	valid          o.Maybe[bool]
	omitFlagsState bool
}

func newBootstrapPayloadBuilder() *bootstrapPayloadBuilder {
	return &bootstrapPayloadBuilder{
		values:   make(map[string]ldvalue.Value),
		metadata: make(map[string]*bootstrapFlagMetadata),
	}
}

// Flag adds a top-level flag value and its "$flagsState" entry. A nil metadata renders as an
// empty metadata object.
func (b *bootstrapPayloadBuilder) Flag(
	key string, value ldvalue.Value, metadata *bootstrapFlagMetadata,
) *bootstrapPayloadBuilder {
	if _, exists := b.values[key]; !exists {
		b.keys = append(b.keys, key)
	}
	b.values[key] = value
	b.metadata[key] = metadata
	return b
}

// Valid sets "$valid" explicitly. Leave it unset to omit the property, which is equivalent
// to true.
func (b *bootstrapPayloadBuilder) Valid(valid bool) *bootstrapPayloadBuilder {
	b.valid = o.Some(valid)
	return b
}

// OmitFlagsState suppresses the "$flagsState" key entirely, producing the legacy value-only
// payload shape emitted by older server-side SDKs.
func (b *bootstrapPayloadBuilder) OmitFlagsState() *bootstrapPayloadBuilder {
	b.omitFlagsState = true
	return b
}

// Build returns the payload in the form expected by
// servicedef.SDKConfigClientSideParams.Bootstrap.
func (b *bootstrapPayloadBuilder) Build() map[string]ldvalue.Value {
	payload := make(map[string]ldvalue.Value, len(b.keys)+2)
	for _, key := range b.keys {
		payload[key] = b.values[key]
	}
	if !b.omitFlagsState {
		flagsState := ldvalue.ObjectBuildWithCapacity(len(b.keys))
		for _, key := range b.keys {
			flagsState.Set(key, b.metadata[key].toValue())
		}
		payload["$flagsState"] = flagsState.Build()
	}
	if b.valid.IsDefined() {
		payload["$valid"] = ldvalue.Bool(b.valid.Value())
	}
	return payload
}

// withBootstrap returns an SDKConfigurer that sets the client-side "bootstrap" configuration
// property. It merges into any existing client-side config rather than replacing it, so it
// can be combined with WithClientSideInitialContext.
func withBootstrap(payload map[string]ldvalue.Value) SDKConfigurer {
	return h.ConfigOptionFunc[servicedef.SDKConfigParams](func(config *servicedef.SDKConfigParams) error {
		clientSide := config.ClientSide.Value()
		clientSide.Bootstrap = o.Some(payload)
		config.ClientSide = o.Some(clientSide)
		return nil
	})
}

// evaluateBootstrapFlag evaluates a single flag on a client-side SDK client. The evaluate
// command's "context" property is always omitted for client-side SDKs (see
// docs/service_spec.md); the SDK evaluates against the context it currently holds.
func evaluateBootstrapFlag(
	t *ldtest.T, client *SDKClient, flagKey string, defaultValue ldvalue.Value,
) servicedef.EvaluateFlagResponse {
	return client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flagKey,
		ValueType:    servicedef.ValueTypeAny,
		DefaultValue: defaultValue,
	})
}

// evaluateBootstrapFlagDetail is evaluateBootstrapFlag using the SDK's VariationDetail
// method instead of Variation.
func evaluateBootstrapFlagDetail(
	t *ldtest.T, client *SDKClient, flagKey string, defaultValue ldvalue.Value,
) servicedef.EvaluateFlagResponse {
	return client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      flagKey,
		ValueType:    servicedef.ValueTypeAny,
		DefaultValue: defaultValue,
		Detail:       true,
	})
}

func doClientSideBootstrapTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityBootstrap)

	t.Run("bootstrap flags are evaluable with no network data source",
		doClientSideBootstrapEvaluableImmediatelyTest)
	t.Run("valid bootstrap data satisfies initialization without a network response",
		doClientSideBootstrapSatisfiesInitializationTest)
	t.Run("bootstrap flag metadata is used for events but not returned by all flags",
		doClientSideBootstrapMetadataInEventsTest)
	t.Run("bootstrap flag metadata prerequisites produce an event for the prerequisite flag",
		doClientSideBootstrapPrerequisiteMetadataTest)
	t.Run("bootstrap flag with a null value evaluates to the caller-supplied default",
		doClientSideBootstrapNullFlagValueTest)
	t.Run("bootstrap payload with $valid false is ingested without error",
		doClientSideBootstrapInvalidPayloadTest)
	t.Run("legacy bootstrap payload without $flagsState is ingested",
		doClientSideBootstrapLegacyPayloadTest)
	t.Run("bootstrap data is replaced by the first data set from a live synchronizer",
		doClientSideBootstrapReplacedByLiveDataTest)
}

// doClientSideBootstrapEvaluableImmediatelyTest verifies that the SDK populates its flag
// store from the bootstrap object before making any network calls, and that the data is
// usable for evaluations immediately after initialization. The client has no initializer
// and no synchronizer, so the only possible source for a non-default value is the bootstrap
// object.
func doClientSideBootstrapEvaluableImmediatelyTest(t *ldtest.T) {
	const stringFlagKey = "bootstrap-string-flag"
	const boolFlagKey = "bootstrap-bool-flag"
	stringFlagValue := ldvalue.String("bootstrap-string-value")
	boolFlagValue := ldvalue.Bool(true)
	stringFallback := ldvalue.String("string-fallback")
	boolFallback := ldvalue.Bool(false)

	bootstrap := newBootstrapPayloadBuilder().
		Flag(stringFlagKey, stringFlagValue, &bootstrapFlagMetadata{
			Variation: o.Some(1),
			Version:   o.Some(100),
		}).
		Flag(boolFlagKey, boolFlagValue, &bootstrapFlagMetadata{
			Variation: o.Some(0),
			Version:   o.Some(200),
		}).
		Valid(true).
		Build()

	client := NewSDKClient(t, withOfflineDataSystem(), withBootstrap(bootstrap))

	m.In(t).For("string flag value from bootstrap").
		Assert(evaluateBootstrapFlag(t, client, stringFlagKey, stringFallback).Value,
			m.JSONEqual(stringFlagValue))
	m.In(t).For("bool flag value from bootstrap").
		Assert(evaluateBootstrapFlag(t, client, boolFlagKey, boolFallback).Value,
			m.JSONEqual(boolFlagValue))
}

// doClientSideBootstrapSatisfiesInitializationTest verifies that valid bootstrap data alone
// satisfies the SDK's initialization condition, without waiting for a network response.
//
// The data system here is live, not offline: it has a single connection mode with only a
// streaming synchronizer, which accepts the connection but never sends any data.
// startWaitTimeMs is short, so if the SDK incorrectly waited for a network response before
// considering itself initialized, the streaming connection would hang, the test service
// would time out waiting for initialization, and NewSDKClient would fail. If the SDK
// correctly treats bootstrap as sufficient for initialization, it resolves promptly and the
// flag evaluates to the bootstrap value rather than the fallback.
func doClientSideBootstrapSatisfiesInitializationTest(t *ldtest.T) {
	const flagKey = "bootstrap-init-flag"
	flagValue := ldvalue.String("bootstrap-init-value")
	fallbackValue := ldvalue.String("init-fallback")

	bootstrap := newBootstrapPayloadBuilder().
		Flag(flagKey, flagValue, &bootstrapFlagMetadata{
			Variation: o.Some(0),
			Version:   o.Some(300),
		}).
		Valid(true).
		Build()

	sdkKind := requireContext(t).sdkKind
	dataSystem := NewSDKDataSystemCustom(t, mockld.BlockingUnavailableSDKData(sdkKind),
		DataSystemOptionConnectionMode("streaming", DataSystemOptionStreaming()),
		DataSystemOptionInitialConnectionMode("streaming"),
	)
	dataSystem.CreateEndpoints()

	client := NewSDKClient(t,
		dataSystem,
		WithWaitToStart(time.Second, false),
		withBootstrap(bootstrap))

	m.In(t).For("flag value after initialization").
		Assert(evaluateBootstrapFlag(t, client, flagKey, fallbackValue).Value, m.JSONEqual(flagValue))
}

// doClientSideBootstrapMetadataInEventsTest verifies that the SDK merges the "$flagsState"
// metadata (variation, version, trackEvents, trackReason, reason) into its internal flag
// representation and uses it for analytics events. debugEventsUntilDate is not exercised
// here; debug events are out of scope for this test.
func doClientSideBootstrapMetadataInEventsTest(t *ldtest.T) {
	const reasonFlagKey = "bootstrap-tracked-flag"
	const reasonFlagVersion = 400
	const reasonFlagVariation = 2
	reasonFlagValue := ldvalue.String("bootstrap-tracked-value")
	reasonFallbackValue := ldvalue.String("tracked-fallback")
	expectedReason := ldreason.NewEvalReasonFallthrough()

	const noReasonFlagKey = "bootstrap-tracked-no-reason-flag"
	const noReasonFlagVersion = 401
	const noReasonFlagVariation = 3
	noReasonFlagValue := ldvalue.String("bootstrap-tracked-no-reason-value")
	noReasonFallbackValue := ldvalue.String("tracked-no-reason-fallback")

	context := data.NewContextFactory("doClientSideBootstrapMetadataInEventsTest").NextUniqueContext()

	bootstrap := newBootstrapPayloadBuilder().
		Flag(reasonFlagKey, reasonFlagValue, &bootstrapFlagMetadata{
			Variation:   o.Some(reasonFlagVariation),
			Version:     o.Some(reasonFlagVersion),
			TrackEvents: true,
			TrackReason: true,
			Reason:      o.Some(expectedReason),
		}).
		Flag(noReasonFlagKey, noReasonFlagValue, &bootstrapFlagMetadata{
			Variation:   o.Some(noReasonFlagVariation),
			Version:     o.Some(noReasonFlagVersion),
			TrackEvents: true,
			TrackReason: false,
		}).
		Valid(true).
		Build()

	events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
	// "offline" only scopes the data system (no initializer/synchronizer); it does not affect
	// the event processor, so event assertions below still apply normally.
	client := NewSDKClient(t,
		withOfflineDataSystem(),
		WithClientSideInitialContext(context),
		withBootstrap(bootstrap),
		events)

	client.FlushEvents(t)
	_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard the initial identify event

	reasonResult := evaluateBootstrapFlag(t, client, reasonFlagKey, reasonFallbackValue)
	if !m.In(t).For("evaluated value (reason flag)").Assert(reasonResult.Value, m.JSONEqual(reasonFlagValue)) {
		require.Fail(t, "evaluation did not return the bootstrap value, so the event assertions are moot")
	}

	noReasonResult := evaluateBootstrapFlag(t, client, noReasonFlagKey, noReasonFallbackValue)
	if !m.In(t).For("evaluated value (no-reason flag)").
		Assert(noReasonResult.Value, m.JSONEqual(noReasonFlagValue)) {
		require.Fail(t, "evaluation did not return the bootstrap value, so the event assertions are moot")
	}

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).For("analytics events from evaluations of bootstrapped flags").Assert(payload, m.ItemsInAnyOrder(
		IsValidFeatureEventWithConditions(
			t, false, context,
			m.JSONProperty("key").Should(m.Equal(reasonFlagKey)),
			m.JSONProperty("version").Should(m.Equal(reasonFlagVersion)),
			m.JSONProperty("value").Should(m.JSONEqual(reasonFlagValue)),
			m.JSONOptProperty("variation").Should(m.Equal(reasonFlagVariation)),
			m.JSONProperty("reason").Should(m.JSONEqual(expectedReason)),
			m.JSONProperty("default").Should(m.JSONEqual(reasonFallbackValue)),
			JSONPropertyNullOrAbsent("prereqOf"),
		),
		IsValidFeatureEventWithConditions(
			t, false, context,
			m.JSONProperty("key").Should(m.Equal(noReasonFlagKey)),
			m.JSONProperty("version").Should(m.Equal(noReasonFlagVersion)),
			m.JSONProperty("value").Should(m.JSONEqual(noReasonFlagValue)),
			m.JSONOptProperty("variation").Should(m.Equal(noReasonFlagVariation)),
			JSONPropertyNullOrAbsent("reason"),
			m.JSONProperty("default").Should(m.JSONEqual(noReasonFallbackValue)),
			JSONPropertyNullOrAbsent("prereqOf"),
		),
		IsValidSummaryEventWithFlags(
			t.Capabilities().Has(servicedef.CapabilityClientPerContextSummaries),
			m.KV(reasonFlagKey, m.MapOf(
				m.KV("default", m.JSONEqual(reasonFallbackValue)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter(reasonFlagValue, reasonFlagVariation, reasonFlagVersion, 1),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
			m.KV(noReasonFlagKey, m.MapOf(
				m.KV("default", m.JSONEqual(noReasonFallbackValue)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter(noReasonFlagValue, noReasonFlagVariation, noReasonFlagVersion, 1),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		),
	))

	allFlags := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{})
	m.In(t).For(`all-flags result must contain flag values only, with no "$flagsState" metadata`).
		Assert(allFlags.State, m.MapOf(
			m.KV(reasonFlagKey, m.JSONEqual(reasonFlagValue)),
			m.KV(noReasonFlagKey, m.JSONEqual(noReasonFlagValue)),
		))
}

// doClientSideBootstrapPrerequisiteMetadataTest verifies that the SDK reads a flag's
// "prerequisites" list out of its bootstrap "$flagsState" entry and, on evaluating that
// flag, generates an event for each listed prerequisite as if it had been evaluated
// directly, the same way it would for a flag received from a live data source. Both the
// top-level flag and its prerequisite must be present in the bootstrap payload, since a
// client-side SDK evaluates prerequisites from pre-evaluated data rather than recomputing
// them.
func doClientSideBootstrapPrerequisiteMetadataTest(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityClientPrereqEvents)

	const topFlagKey = "bootstrap-prereq-top-flag"
	const topFlagVersion = 800
	const topFlagVariation = 0
	topFlagValue := ldvalue.String("bootstrap-prereq-top-value")

	const prereqFlagKey = "bootstrap-prereq-flag"
	const prereqFlagVersion = 801
	const prereqFlagVariation = 0
	prereqFlagValue := ldvalue.String("bootstrap-prereq-value")

	context := data.NewContextFactory("doClientSideBootstrapPrerequisiteMetadataTest").NextUniqueContext()

	bootstrap := newBootstrapPayloadBuilder().
		Flag(topFlagKey, topFlagValue, &bootstrapFlagMetadata{
			Variation:     o.Some(topFlagVariation),
			Version:       o.Some(topFlagVersion),
			TrackEvents:   true,
			Prerequisites: []string{prereqFlagKey},
		}).
		Flag(prereqFlagKey, prereqFlagValue, &bootstrapFlagMetadata{
			Variation:   o.Some(prereqFlagVariation),
			Version:     o.Some(prereqFlagVersion),
			TrackEvents: true,
		}).
		Valid(true).
		Build()

	events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
	client := NewSDKClient(t,
		withOfflineDataSystem(),
		WithClientSideInitialContext(context),
		withBootstrap(bootstrap),
		events)

	client.FlushEvents(t)
	_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard the initial identify event

	result := evaluateBootstrapFlag(t, client, topFlagKey, ldvalue.Null())
	if !m.In(t).For("top-level flag value").Assert(result.Value, m.JSONEqual(topFlagValue)) {
		require.Fail(t, "evaluation did not return the bootstrap value, so the event assertions are moot")
	}

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).For("events from evaluating a flag whose bootstrap metadata lists a prerequisite").
		Assert(payload, m.ItemsInAnyOrder(
			IsValidFeatureEventWithConditions(
				t, false, context,
				m.JSONProperty("key").Should(m.Equal(prereqFlagKey)),
				m.JSONProperty("version").Should(m.Equal(prereqFlagVersion)),
				m.JSONProperty("value").Should(m.JSONEqual(prereqFlagValue)),
				m.JSONOptProperty("variation").Should(m.Equal(prereqFlagVariation)),
				JSONPropertyNullOrAbsent("prereqOf"),
			),
			IsValidFeatureEventWithConditions(
				t, false, context,
				m.JSONProperty("key").Should(m.Equal(topFlagKey)),
				m.JSONProperty("version").Should(m.Equal(topFlagVersion)),
				m.JSONProperty("value").Should(m.JSONEqual(topFlagValue)),
				m.JSONOptProperty("variation").Should(m.Equal(topFlagVariation)),
				JSONPropertyNullOrAbsent("prereqOf"),
			),
			IsValidSummaryEventWithFlags(
				t.Capabilities().Has(servicedef.CapabilityClientPerContextSummaries),
				// MapIncluding, not MapOf: unlike topFlagKey, this flag was never evaluated
				// with a caller-supplied default, so whether a "default" key is present here
				// (and what it holds) is unspecified.
				m.KV(prereqFlagKey, m.MapIncluding(
					m.KV("counters", m.ItemsInAnyOrder(
						flagCounter(prereqFlagValue, prereqFlagVariation, prereqFlagVersion, 1),
					)),
					m.KV("contextKinds", anyContextKindsList()),
				)),
				m.KV(topFlagKey, m.MapOf(
					m.KV("default", m.JSONEqual(ldvalue.Null())),
					m.KV("counters", m.ItemsInAnyOrder(
						flagCounter(topFlagValue, topFlagVariation, topFlagVersion, 1),
					)),
					m.KV("contextKinds", anyContextKindsList()),
				)),
			),
		))
}

// doClientSideBootstrapNullFlagValueTest verifies handling of a flag whose evaluation
// failed on the server side: it still appears at the top level with a null value and still
// has a "$flagsState" entry, but with no "variation" field. A null flag value means
// "evaluation failed", so Variation and VariationDetail must return the caller-supplied
// default rather than null.
func doClientSideBootstrapNullFlagValueTest(t *ldtest.T) {
	const nullFlagKey = "bootstrap-null-flag"
	const okFlagKey = "bootstrap-non-null-flag"
	okFlagValue := ldvalue.String("bootstrap-non-null-value")
	nullFlagFallback := ldvalue.String("null-flag-fallback")
	okFlagFallback := ldvalue.String("non-null-flag-fallback")

	bootstrap := newBootstrapPayloadBuilder().
		Flag(nullFlagKey, ldvalue.Null(), &bootstrapFlagMetadata{
			Version: o.Some(500), // no "variation": evaluation failed
		}).
		Flag(okFlagKey, okFlagValue, &bootstrapFlagMetadata{
			Variation: o.Some(1),
			Version:   o.Some(501),
		}).
		Valid(true).
		Build()

	client := NewSDKClient(t, withOfflineDataSystem(), withBootstrap(bootstrap))

	// The sibling flag proves the payload was ingested at all, so a default result for the
	// null flag cannot be explained by the SDK having rejected the whole payload.
	m.In(t).For("sibling non-null flag value").
		Assert(evaluateBootstrapFlag(t, client, okFlagKey, okFlagFallback).Value, m.JSONEqual(okFlagValue))

	m.In(t).For("Variation result for a null-valued bootstrap flag").
		Assert(evaluateBootstrapFlag(t, client, nullFlagKey, nullFlagFallback).Value,
			m.JSONEqual(nullFlagFallback))

	detail := evaluateBootstrapFlagDetail(t, client, nullFlagKey, nullFlagFallback)
	m.In(t).For("VariationDetail value for a null-valued bootstrap flag").
		Assert(detail.Value, m.JSONEqual(nullFlagFallback))
	m.In(t).For("VariationDetail variation index for a null-valued bootstrap flag").
		Assert(detail.VariationIndex.IsDefined(), m.Equal(false))
}

// doClientSideBootstrapInvalidPayloadTest verifies that with "$valid": false the SDK must
// still ingest and use whatever flag data is present in the payload, and must not throw or
// refuse to initialize.
//
// The payload carries one real flag, presentFlagKey, instead of being empty. An empty payload
// would only show that the SDK didn't crash: a flag falling back to its default is exactly
// what an unknown flag does too, so the two cases would look identical. presentFlagKey removes
// that ambiguity: if the SDK discarded the whole payload because "$valid" is false, it would
// fall back to its caller-supplied default just like the unknown flag does, making the two
// assertions indistinguishable. Getting the bootstrap value back instead proves the SDK
// actually parsed and used the data.
func doClientSideBootstrapInvalidPayloadTest(t *ldtest.T) {
	const unknownFlagKey = "bootstrap-absent-flag"
	unknownFlagFallback := ldvalue.String("invalid-payload-fallback")

	const presentFlagKey = "bootstrap-invalid-payload-present-flag"
	presentFlagValue := ldvalue.String("invalid-payload-present-value")
	presentFlagFallback := ldvalue.String("invalid-payload-present-fallback")

	bootstrap := newBootstrapPayloadBuilder().
		Flag(presentFlagKey, presentFlagValue, &bootstrapFlagMetadata{
			Variation: o.Some(0),
			Version:   o.Some(700),
		}).
		Valid(false).
		Build()

	client := NewSDKClient(t,
		withOfflineDataSystem(),
		WithWaitToStart(time.Second, true),
		withBootstrap(bootstrap))

	m.In(t).For("evaluation result for a flag present in an invalid bootstrap payload").
		Assert(evaluateBootstrapFlag(t, client, presentFlagKey, presentFlagFallback).Value,
			m.JSONEqual(presentFlagValue))

	m.In(t).For("evaluation result for a flag absent from an invalid bootstrap payload").
		Assert(evaluateBootstrapFlag(t, client, unknownFlagKey, unknownFlagFallback).Value,
			m.JSONEqual(unknownFlagFallback))
}

// doClientSideBootstrapLegacyPayloadTest verifies that when "$flagsState" is absent, as in
// payloads from older server-side SDKs, the SDK must still ingest the flag values. The
// payload also omits "$valid", which an SDK must treat as true without a warning.
func doClientSideBootstrapLegacyPayloadTest(t *ldtest.T) {
	const stringFlagKey = "legacy-string-flag"
	const boolFlagKey = "legacy-bool-flag"
	stringFlagValue := ldvalue.String("legacy-string-value")
	boolFlagValue := ldvalue.Bool(true)
	stringFallback := ldvalue.String("legacy-string-fallback")
	boolFallback := ldvalue.Bool(false)

	bootstrap := newBootstrapPayloadBuilder().
		Flag(stringFlagKey, stringFlagValue, nil).
		Flag(boolFlagKey, boolFlagValue, nil).
		OmitFlagsState().
		Build()

	client := NewSDKClient(t, withOfflineDataSystem(), withBootstrap(bootstrap))

	m.In(t).For("string flag value from a legacy bootstrap payload").
		Assert(evaluateBootstrapFlag(t, client, stringFlagKey, stringFallback).Value,
			m.JSONEqual(stringFlagValue))
	m.In(t).For("bool flag value from a legacy bootstrap payload").
		Assert(evaluateBootstrapFlag(t, client, boolFlagKey, boolFallback).Value,
			m.JSONEqual(boolFlagValue))
}

// doClientSideBootstrapReplacedByLiveDataTest verifies that bootstrap data does not persist
// past the first successful network update.
func doClientSideBootstrapReplacedByLiveDataTest(t *ldtest.T) {
	const flagKey = "bootstrap-replaced-flag"
	bootstrapValue := ldvalue.String("bootstrap-value")
	liveValue := ldvalue.String("live-synchronizer-value")
	fallbackValue := ldvalue.String("replaced-flag-fallback")

	// bootstrapOnlyFlagKey is present in the bootstrap payload but deliberately absent from
	// liveData. Bootstrap data must be replaced, not merged, by the live synchronizer's first
	// payload; if the SDK merged instead of replaced, this flag would still evaluate to
	// bootstrapOnlyValue after the live payload arrives, instead of falling back to the
	// caller-supplied default.
	const bootstrapOnlyFlagKey = "bootstrap-only-flag"
	bootstrapOnlyValue := ldvalue.String("bootstrap-only-value")
	bootstrapOnlyFallbackValue := ldvalue.String("bootstrap-only-fallback")

	bootstrap := newBootstrapPayloadBuilder().
		Flag(flagKey, bootstrapValue, &bootstrapFlagMetadata{
			Variation: o.Some(0),
			Version:   o.Some(600),
		}).
		Flag(bootstrapOnlyFlagKey, bootstrapOnlyValue, &bootstrapFlagMetadata{
			Variation: o.Some(0),
			Version:   o.Some(602),
		}).
		Valid(true).
		Build()

	liveData := mockld.NewClientSDKDataBuilder().
		Flag(flagKey, mockld.ClientSDKFlag{
			Value:       liveValue,
			Variation:   o.Some(1),
			Version:     601,
			FlagVersion: o.Some(601),
		}).
		Build()

	t.Run("bootstrap value is served when there is no data source", func(t *ldtest.T) {
		client := NewSDKClient(t, withOfflineDataSystem(), withBootstrap(bootstrap))

		m.In(t).For("flag value with no data source").
			Assert(evaluateBootstrapFlag(t, client, flagKey, fallbackValue).Value, m.JSONEqual(bootstrapValue))
	})

	t.Run("live synchronizer data replaces the bootstrap value", func(t *ldtest.T) {
		dataSystem := NewSDKDataSystem(t, liveData)
		client := NewSDKClient(t, dataSystem, withBootstrap(bootstrap))

		h.RequireEventually(t, func() bool {
			actual := evaluateBootstrapFlag(t, client, flagKey, fallbackValue).Value
			return actual.Equal(liveValue)
		}, time.Second, time.Millisecond*20,
			"bootstrap data was never replaced by the live synchronizer's data")

		m.In(t).For("bootstrap-only flag value after the live synchronizer's data has replaced bootstrap").
			Assert(evaluateBootstrapFlag(t, client, bootstrapOnlyFlagKey, bootstrapOnlyFallbackValue).Value,
				m.JSONEqual(bootstrapOnlyFallbackValue))
	})
}
