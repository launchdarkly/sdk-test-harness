package sdktests

import (
	"time"

	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// overrideDocument is the file format for flag overrides: full flag definitions in "flags",
// simple values in "flagValues" (each expands to a flag returning that value for every context),
// and full segment definitions in "segments".
type overrideDocument struct {
	Flags      map[string]ldmodel.FeatureFlag `json:"flags,omitempty"`
	FlagValues map[string]ldvalue.Value       `json:"flagValues,omitempty"`
	Segments   map[string]ldmodel.Segment     `json:"segments,omitempty"`
}

func (d overrideDocument) String() string {
	return string(jsonhelpers.ToJSON(d))
}

// reasonIsOverrideAffected matches a raw JSON evaluation reason that has the given kind and
// "overrideAffected": true. The SDK sets this indicator when the evaluation read at least one
// definition from the override store.
func reasonIsOverrideAffected(kind string) m.Matcher {
	return m.AllOf(
		m.JSONProperty("kind").Should(m.Equal(kind)),
		m.JSONProperty("overrideAffected").Should(m.Equal(true)),
	)
}

// reasonIsNotOverrideAffected matches a raw JSON evaluation reason that has the given kind and no
// "overrideAffected" property. The SDK omits the property, and does not write false, when the
// evaluation read no definition from the override store.
func reasonIsNotOverrideAffected(kind string) m.Matcher {
	return m.AllOf(
		m.JSONProperty("kind").Should(m.Equal(kind)),
		m.JSONOptProperty("overrideAffected").Should(m.BeNil()),
	)
}

func doServerSideFlagOverridesTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityFlagOverrides)

	t.Run("static configuration", doServerSideFlagOverridesStaticTests)
	t.Run("uninitialized client", doServerSideFlagOverridesUninitializedTests)
	t.Run("summary events", doServerSideFlagOverridesSummaryEventTest)
	t.Run("transitive marking", doServerSideFlagOverridesTransitiveMarkingTests)
	t.Run("multiple files", doServerSideFlagOverridesMultiFileTest)
	t.Run("YAML document", doServerSideFlagOverridesYAMLTest)
	t.Run("hot reload", doServerSideFlagOverridesHotReloadTests)
}

func doServerSideFlagOverridesStaticTests(t *ldtest.T) {
	context := ldcontext.New("user-key")
	defaultValue := ldvalue.String("default")

	// Flags and segment served by the mock LaunchDarkly services.
	ldFlagPrecedence := ldbuilders.NewFlagBuilder("flag-precedence").Version(100).
		On(false).OffVariation(0).Variations(ldvalue.String("ld-value")).Build()
	ldFlagNormal := ldbuilders.NewFlagBuilder("flag-normal").Version(100).
		On(false).OffVariation(0).Variations(ldvalue.String("normal-value")).Build()
	ldSegment := ldbuilders.NewSegmentBuilder("overridden-segment").Version(100).Build() // does not include the context

	// Full flag definitions and segment provided by the override file. The overridden segment
	// includes the context, unlike the LaunchDarkly version of the same segment.
	overrideRuleFlag := ldbuilders.NewFlagBuilder("flag-rule").Version(1).
		On(true).OffVariation(0).FallthroughVariation(0).
		Variations(ldvalue.String("fallthrough-value"), ldvalue.String("rule-value")).
		AddRule(ldbuilders.NewRuleBuilder().ID("override-rule").Variation(1).Clauses(
			ldbuilders.Clause(ldattr.KeyAttr, ldmodel.OperatorIn, ldvalue.String(context.Key())),
		)).
		Build()
	overrideSegmentFlag := makeFlagToCheckSegmentMatch("flag-segment-check", "overridden-segment",
		ldvalue.String("not-included"), ldvalue.String("included"))
	overrideSegment := ldbuilders.NewSegmentBuilder("overridden-segment").Version(101).
		Included(context.Key()).Build()

	overrides := overrideDocument{
		FlagValues: map[string]ldvalue.Value{
			ldFlagPrecedence.Key: ldvalue.String("override-value"),
		},
		Flags: map[string]ldmodel.FeatureFlag{
			overrideRuleFlag.Key:    overrideRuleFlag,
			overrideSegmentFlag.Key: overrideSegmentFlag,
		},
		Segments: map[string]ldmodel.Segment{
			overrideSegment.Key: overrideSegment,
		},
	}

	data := mockld.NewServerSDKDataBuilder().
		Flag(ldFlagPrecedence, ldFlagNormal).Segment(ldSegment).Build()
	dataSystem := NewSDKDataSystem(t, data)
	overrideFile := NewOverrideFile(t, overrides.String())
	client := NewSDKClient(t, dataSystem,
		WithFileOverrides(servicedef.SDKConfigOverridesParams{FilePaths: []string{overrideFile.Path}}))

	t.Run("flagValues override takes precedence over LaunchDarkly data", func(t *ldtest.T) {
		result := evaluateFlagDetailRawReason(t, client, servicedef.EvaluateFlagParams{
			FlagKey: ldFlagPrecedence.Key, Context: o.Some(context), DefaultValue: defaultValue})
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("override-value")))
		m.In(t).Assert(result.VariationIndex, m.Equal(o.Some(0)))
		m.In(t).Assert(result.Reason, reasonIsOverrideAffected("OFF"))
	})

	t.Run("full flag override evaluates targeting rules", func(t *ldtest.T) {
		result := evaluateFlagDetailRawReason(t, client, servicedef.EvaluateFlagParams{
			FlagKey: overrideRuleFlag.Key, Context: o.Some(context), DefaultValue: defaultValue})
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("rule-value")))
		m.In(t).Assert(result.VariationIndex, m.Equal(o.Some(1)))
		m.In(t).Assert(result.Reason, m.AllOf(
			reasonIsOverrideAffected("RULE_MATCH"),
			m.JSONProperty("ruleId").Should(m.Equal("override-rule")),
		))
	})

	t.Run("non-overridden flag is unaffected", func(t *ldtest.T) {
		result := evaluateFlagDetailRawReason(t, client, servicedef.EvaluateFlagParams{
			FlagKey: ldFlagNormal.Key, Context: o.Some(context), DefaultValue: defaultValue})
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("normal-value")))
		m.In(t).Assert(result.Reason, reasonIsNotOverrideAffected("OFF"))
	})

	t.Run("overridden flag rule can reference overridden segment", func(t *ldtest.T) {
		result := evaluateFlagDetailRawReason(t, client, servicedef.EvaluateFlagParams{
			FlagKey: overrideSegmentFlag.Key, Context: o.Some(context), DefaultValue: defaultValue})
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("included")))
		m.In(t).Assert(result.Reason, reasonIsOverrideAffected("RULE_MATCH"))
	})

	t.Run("evaluate all flags reflects overrides", func(t *ldtest.T) {
		result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{
			Context: o.Some(context),
		})
		m.In(t).Assert(result, m.AllOf(
			EvalAllFlagsValueForKeyShouldEqual(ldFlagPrecedence.Key, ldvalue.String("override-value")),
			EvalAllFlagsValueForKeyShouldEqual(ldFlagNormal.Key, ldvalue.String("normal-value")),
			EvalAllFlagsValueForKeyShouldEqual(overrideRuleFlag.Key, ldvalue.String("rule-value")),
			EvalAllFlagsValueForKeyShouldEqual(overrideSegmentFlag.Key, ldvalue.String("included")),
		))
	})
}

func doServerSideFlagOverridesUninitializedTests(t *ldtest.T) {
	context := ldcontext.New("user-key")
	defaultValue := ldvalue.String("default")

	overrides := overrideDocument{
		FlagValues: map[string]ldvalue.Value{
			"overridden-flag": ldvalue.String("override-value"),
		},
	}

	// The mock services never provide any data, so the client can never initialize.
	dataSystem := NewSDKDataSystem(t, mockld.BlockingUnavailableSDKData(mockld.ServerSideSDK))
	overrideFile := NewOverrideFile(t, overrides.String())
	client := NewSDKClient(t,
		WithConfig(servicedef.SDKConfigParams{StartWaitTimeMS: o.Some(ldtime.UnixMillisecondTime(1)),
			InitCanFail: true}),
		dataSystem,
		WithFileOverrides(servicedef.SDKConfigOverridesParams{FilePaths: []string{overrideFile.Path}}))

	t.Run("overridden flag returns override value", func(t *ldtest.T) {
		result := evaluateFlagDetailRawReason(t, client, servicedef.EvaluateFlagParams{
			FlagKey: "overridden-flag", Context: o.Some(context), DefaultValue: defaultValue})
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("override-value")))
		m.In(t).Assert(result.Reason, reasonIsOverrideAffected("OFF"))
	})

	t.Run("non-overridden flag returns default with client-not-ready error", func(t *ldtest.T) {
		result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      "non-overridden-flag",
			Context:      o.Some(context),
			ValueType:    servicedef.ValueTypeAny,
			DefaultValue: defaultValue,
			Detail:       true,
		})
		m.In(t).Assert(result, m.AllOf(
			EvalResponseValue().Should(m.JSONEqual(defaultValue)),
			EvalResponseVariation().Should(m.Equal(o.None[int]())),
			EvalResponseReason().Should(
				EqualReason(ldreason.NewEvalReasonError(ldreason.EvalErrorClientNotReady))),
		))
	})
}

func doServerSideFlagOverridesSummaryEventTest(t *ldtest.T) {
	context := ldcontext.New("user-key")
	default1 := ldvalue.String("default1")
	default2 := ldvalue.String("default2")

	// The overridden flag is a full flag definition that requests individual feature events and
	// debug events. An ordinary flag with this configuration produces both for each evaluation.
	// The SDK marks each evaluation of an overridden flag as override-affected, so it must produce
	// no individual feature event and no debug event. Only index and summary events appear, and
	// the summary counter carries the override-affected marker.
	trackedOverrideFlag := ldbuilders.NewFlagBuilder("flag-tracked-override").Version(300).
		On(false).OffVariation(0).Variations(ldvalue.String("override-value")).
		TrackEvents(true).
		DebugEventsUntilDate(ldtime.UnixMillisNow() + 100000).
		Build()
	normalFlag := ldbuilders.NewFlagBuilder("flag-normal").Version(100).
		On(false).OffVariation(0).Variations(ldvalue.String("normal-value")).Build()

	overrides := overrideDocument{
		Flags: map[string]ldmodel.FeatureFlag{
			trackedOverrideFlag.Key: trackedOverrideFlag,
		},
	}

	data := mockld.NewServerSDKDataBuilder().Flag(normalFlag).Build()
	dataSystem := NewSDKDataSystem(t, data)
	events := NewSDKEventSink(t)
	overrideFile := NewOverrideFile(t, overrides.String())
	client := NewSDKClient(t, dataSystem, events,
		WithFileOverrides(servicedef.SDKConfigOverridesParams{FilePaths: []string{overrideFile.Path}}))

	// Two evaluations of the overridden flag accumulate into one marked counter.
	for i := 0; i < 2; i++ {
		_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: trackedOverrideFlag.Key,
			Context: o.Some(context), DefaultValue: default1})
	}
	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: normalFlag.Key,
		Context: o.Some(context), DefaultValue: default2})

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIndexEvent(),
		IsValidSummaryEventWithFlags(
			false,
			m.KV(trackedOverrideFlag.Key, m.MapOf(
				m.KV("default", m.JSONEqual(default1)),
				m.KV("counters", m.ItemsInAnyOrder(
					overrideAffectedFlagCounter("override-value", 0, trackedOverrideFlag.Version, 2),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
			m.KV(normalFlag.Key, m.MapOf(
				m.KV("default", m.JSONEqual(default2)),
				m.KV("counters", m.ItemsInAnyOrder(
					flagCounter("normal-value", 0, normalFlag.Version, 1),
				)),
				m.KV("contextKinds", anyContextKindsList()),
			)),
		),
	))
}

func doServerSideFlagOverridesMultiFileTest(t *ldtest.T) {
	context := ldcontext.New("user-key")
	defaultValue := ldvalue.String("default")

	doc1 := overrideDocument{
		FlagValues: map[string]ldvalue.Value{
			"multi-flag": ldvalue.String("first-value"),
		},
	}
	doc2 := overrideDocument{
		FlagValues: map[string]ldvalue.Value{
			"multi-flag":       ldvalue.String("second-value"),
			"second-file-flag": ldvalue.String("second-file-value"),
		},
	}

	dataSystem := NewSDKDataSystem(t, mockld.NewServerSDKDataBuilder().Build())
	file1 := NewOverrideFile(t, doc1.String())
	file2 := NewOverrideFile(t, doc2.String())
	client := NewSDKClient(t, dataSystem,
		WithFileOverrides(servicedef.SDKConfigOverridesParams{
			FilePaths:             []string{file1.Path, file2.Path},
			DuplicateKeysHandling: o.Some("ignore"),
		}))

	t.Run("first file wins for duplicate keys with ignore handling", func(t *ldtest.T) {
		value := basicEvaluateFlag(t, client, "multi-flag", context, defaultValue)
		m.In(t).Assert(value, m.JSONEqual(ldvalue.String("first-value")))
	})

	t.Run("non-duplicate keys from later files are merged", func(t *ldtest.T) {
		value := basicEvaluateFlag(t, client, "second-file-flag", context, defaultValue)
		m.In(t).Assert(value, m.JSONEqual(ldvalue.String("second-file-value")))
	})

	t.Run("a configured file that does not exist contributes no overrides", func(t *ldtest.T) {
		missing := NewMissingOverrideFile(t)
		clientWithMissing := NewSDKClient(t, dataSystem,
			WithFileOverrides(servicedef.SDKConfigOverridesParams{
				FilePaths: []string{file1.Path, missing.Path},
			}))
		value := basicEvaluateFlag(t, clientWithMissing, "multi-flag", context, defaultValue)
		m.In(t).Assert(value, m.JSONEqual(ldvalue.String("first-value")))
	})
}

func doServerSideFlagOverridesYAMLTest(t *ldtest.T) {
	context := ldcontext.New("user-key")
	defaultValue := ldvalue.String("default")

	yamlContents := "flagValues:\n  yaml-flag: \"override-value\"\n"

	dataSystem := NewSDKDataSystem(t, mockld.NewServerSDKDataBuilder().Build())
	overrideFile := NewOverrideFileWithSuffix(t, ".yaml", yamlContents)
	client := NewSDKClient(t, dataSystem,
		WithFileOverrides(servicedef.SDKConfigOverridesParams{FilePaths: []string{overrideFile.Path}}))

	result := evaluateFlagDetailRawReason(t, client, servicedef.EvaluateFlagParams{
		FlagKey: "yaml-flag", Context: o.Some(context), DefaultValue: defaultValue})
	m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("override-value")))
	m.In(t).Assert(result.Reason, reasonIsOverrideAffected("OFF"))
}

func doServerSideFlagOverridesHotReloadTests(t *ldtest.T) {
	const reloadTimeout = 10 * time.Second
	const reloadPollInterval = 100 * time.Millisecond

	flagKey := "reload-flag"
	context := ldcontext.New("user-key")
	defaultValue := ldvalue.String("default")
	ldValue := ldvalue.String("ld-value")
	overrideValueB := ldvalue.String("override-value-b")
	overrideValueC := ldvalue.String("override-value-c")

	ldFlag := ldbuilders.NewFlagBuilder(flagKey).Version(100).
		On(false).OffVariation(0).Variations(ldValue).Build()
	data := mockld.NewServerSDKDataBuilder().Flag(ldFlag).Build()

	docWith := func(value ldvalue.Value) string {
		return overrideDocument{FlagValues: map[string]ldvalue.Value{flagKey: value}}.String()
	}

	modes := []struct {
		name       string
		makeParams func(paths ...string) servicedef.SDKConfigOverridesParams
	}{
		{"watching mode", func(paths ...string) servicedef.SDKConfigOverridesParams {
			return servicedef.SDKConfigOverridesParams{
				FilePaths:       paths,
				ChangeDetection: o.Some("watching"),
			}
		}},
		{"polling mode", func(paths ...string) servicedef.SDKConfigOverridesParams {
			return servicedef.SDKConfigOverridesParams{
				FilePaths:       paths,
				ChangeDetection: o.Some("polling"),
				PollIntervalMS:  o.Some(1000),
			}
		}},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *ldtest.T) {
			setup := func(t *ldtest.T, initialContents string) (*OverrideFile, *SDKClient) {
				dataSystem := NewSDKDataSystem(t, data)
				overrideFile := NewOverrideFile(t, initialContents)
				client := NewSDKClient(t, dataSystem, WithFileOverrides(mode.makeParams(overrideFile.Path)))
				return overrideFile, client
			}

			requireValue := func(t *ldtest.T, client *SDKClient, expected ldvalue.Value) {
				m.In(t).Require(basicEvaluateFlag(t, client, flagKey, context, defaultValue),
					m.JSONEqual(expected))
			}

			awaitValue := func(t *ldtest.T, client *SDKClient, previous, updated ldvalue.Value) {
				h.RequireEventually(t,
					checkForUpdatedValue(t, client, flagKey, context, previous, updated, defaultValue),
					reloadTimeout, reloadPollInterval,
					"timed out waiting for the SDK to reload the override file")
			}

			t.Run("adding an override takes effect", func(t *ldtest.T) {
				overrideFile, client := setup(t, "{}")
				requireValue(t, client, ldValue)
				overrideFile.Replace(t, docWith(overrideValueB))
				awaitValue(t, client, ldValue, overrideValueB)
			})

			t.Run("changing an override takes effect", func(t *ldtest.T) {
				overrideFile, client := setup(t, docWith(overrideValueB))
				requireValue(t, client, overrideValueB)
				overrideFile.Replace(t, docWith(overrideValueC))
				awaitValue(t, client, overrideValueB, overrideValueC)
			})

			t.Run("removing an override restores LaunchDarkly data", func(t *ldtest.T) {
				overrideFile, client := setup(t, docWith(overrideValueB))
				requireValue(t, client, overrideValueB)
				overrideFile.Clear(t)
				awaitValue(t, client, overrideValueB, ldValue)
			})

			t.Run("deleting the file removes its overrides", func(t *ldtest.T) {
				overrideFile, client := setup(t, docWith(overrideValueB))
				requireValue(t, client, overrideValueB)
				overrideFile.Delete(t)
				awaitValue(t, client, overrideValueB, ldValue)
			})

			t.Run("a file that does not exist yet takes effect when it appears", func(t *ldtest.T) {
				dataSystem := NewSDKDataSystem(t, data)
				overrideFile := NewMissingOverrideFile(t)
				client := NewSDKClient(t, dataSystem, WithFileOverrides(mode.makeParams(overrideFile.Path)))
				requireValue(t, client, ldValue)
				overrideFile.Replace(t, docWith(overrideValueB))
				awaitValue(t, client, ldValue, overrideValueB)
			})

			t.Run("malformed file retains last good overrides", func(t *ldtest.T) {
				overrideFile, client := setup(t, docWith(overrideValueB))
				requireValue(t, client, overrideValueB)
				overrideFile.Replace(t, `{"flagValues"`)
				h.RequireNever(t,
					func() bool {
						return !basicEvaluateFlag(t, client, flagKey, context, defaultValue).Equal(overrideValueB)
					},
					1500*time.Millisecond, reloadPollInterval,
					"SDK stopped serving the last good overrides after reading a malformed file")
				overrideFile.Replace(t, docWith(overrideValueC))
				awaitValue(t, client, overrideValueB, overrideValueC)
			})
		})
	}
}

func doServerSideFlagOverridesTransitiveMarkingTests(t *ldtest.T) {
	// An evaluation is override-affected when any definition that it reads comes from the override
	// store: the evaluated flag, a prerequisite flag at any depth, or a segment consulted during
	// rule matching. Every flag in these tests comes from LaunchDarkly. Only one segment and one
	// prerequisite flag are overridden. A marked evaluation produces no individual feature event
	// and no debug event, and its summary counter carries the override-affected marker. The
	// record for an intermediate prerequisite evaluation is marked by that prerequisite's own
	// reads, so an unaffected prerequisite is recorded as usual even inside a marked evaluation.
	context := ldcontext.New("user-key")
	otherContext := ldcontext.New("other-user-key")
	defaultValue := ldvalue.String("default")
	debugUntil := ldtime.UnixMillisNow() + 100000

	const segmentKey = "overridden-segment"

	// The LaunchDarkly segment does not include the context. The override segment does.
	ldSegment := ldbuilders.NewSegmentBuilder(segmentKey).Version(100).Build()
	overrideSegment := ldbuilders.NewSegmentBuilder(segmentKey).Version(200).
		Included(context.Key()).Build()

	// segmentFlag comes from LaunchDarkly. Its rule references the overridden segment. It requests
	// individual feature events and debug events.
	segmentFlag := ldbuilders.NewFlagBuilder("flag-with-overridden-segment").Version(100).
		On(true).OffVariation(0).FallthroughVariation(0).
		Variations(ldvalue.String("not-included"), ldvalue.String("included")).
		AddRule(ldbuilders.NewRuleBuilder().ID("segment-rule").Variation(1).Clauses(
			ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String(segmentKey)),
		)).
		TrackEvents(true).DebugEventsUntilDate(debugUntil).
		Build()

	// The LaunchDarkly definition of overriddenPrereq is off, so it fails as a prerequisite. The
	// override definition is on and serves variation 1, so it satisfies the prerequisite.
	prereqVariations := []ldvalue.Value{ldvalue.String("prereq-ld-value"), ldvalue.String("prereq-override-value")}
	ldOverriddenPrereq := ldbuilders.NewFlagBuilder("overridden-prereq").Version(100).
		On(false).OffVariation(0).Variations(prereqVariations...).
		TrackEvents(true).DebugEventsUntilDate(debugUntil).
		Build()
	overriddenPrereq := ldbuilders.NewFlagBuilder("overridden-prereq").Version(200).
		On(true).OffVariation(0).FallthroughVariation(1).Variations(prereqVariations...).
		TrackEvents(true).DebugEventsUntilDate(debugUntil).
		Build()

	// plainPrereq comes from LaunchDarkly and is not overridden. It requests individual feature
	// events, so an unaffected evaluation of it produces one.
	plainPrereq := ldbuilders.NewFlagBuilder("plain-prereq").Version(100).
		On(true).OffVariation(0).FallthroughVariation(1).
		Variations(ldvalue.String("plain-prereq-off"), ldvalue.String("plain-prereq-value")).
		TrackEvents(true).
		Build()

	// overriddenPrereqFlag comes from LaunchDarkly and depends only on the overridden prerequisite.
	overriddenPrereqFlag := ldbuilders.NewFlagBuilder("flag-with-overridden-prereq").Version(100).
		On(true).OffVariation(0).FallthroughVariation(1).
		AddPrerequisite(overriddenPrereq.Key, 1).
		Variations(ldvalue.String("prereq-failed"), ldvalue.String("affected-value")).
		TrackEvents(true).DebugEventsUntilDate(debugUntil).
		Build()

	// mixedPrereqFlag comes from LaunchDarkly and depends on both prerequisites.
	mixedPrereqFlag := ldbuilders.NewFlagBuilder("flag-with-mixed-prereqs").Version(100).
		On(true).OffVariation(0).FallthroughVariation(1).
		AddPrerequisite(overriddenPrereq.Key, 1).
		AddPrerequisite(plainPrereq.Key, 1).
		Variations(ldvalue.String("prereq-failed"), ldvalue.String("mixed-value")).
		TrackEvents(true).DebugEventsUntilDate(debugUntil).
		Build()

	// controlFlag comes from LaunchDarkly and depends only on the unaffected prerequisite.
	controlFlag := ldbuilders.NewFlagBuilder("flag-with-plain-prereq").Version(100).
		On(true).OffVariation(0).FallthroughVariation(1).
		AddPrerequisite(plainPrereq.Key, 1).
		Variations(ldvalue.String("prereq-failed"), ldvalue.String("control-value")).
		TrackEvents(true).
		Build()

	data := mockld.NewServerSDKDataBuilder().
		Flag(segmentFlag, ldOverriddenPrereq, plainPrereq, overriddenPrereqFlag, mixedPrereqFlag, controlFlag).
		Segment(ldSegment).
		Build()
	overrides := overrideDocument{
		Flags:    map[string]ldmodel.FeatureFlag{overriddenPrereq.Key: overriddenPrereq},
		Segments: map[string]ldmodel.Segment{overrideSegment.Key: overrideSegment},
	}

	// Each test uses its own client and event sink, so each event payload contains only the events
	// from that test. The payload assertions list every expected event, so an unexpected feature
	// event or debug event fails the test.
	setup := func(t *ldtest.T) (*SDKClient, *SDKEventSink) {
		dataSystem := NewSDKDataSystem(t, data)
		events := NewSDKEventSink(t)
		overrideFile := NewOverrideFile(t, overrides.String())
		client := NewSDKClient(t, dataSystem, events,
			WithFileOverrides(servicedef.SDKConfigOverridesParams{FilePaths: []string{overrideFile.Path}}))
		return client, events
	}

	evaluate := func(t *ldtest.T, client *SDKClient, flagKey string, ctx ldcontext.Context) evaluateFlagRawReasonResponse {
		return evaluateFlagDetailRawReason(t, client, servicedef.EvaluateFlagParams{
			FlagKey: flagKey, Context: o.Some(ctx), DefaultValue: defaultValue})
	}

	flushAndGetEvents := func(t *ldtest.T, client *SDKClient, events *SDKEventSink) mockld.Events {
		client.FlushEvents(t)
		return events.ExpectAnalyticsEvents(t, defaultEventTimeout)
	}

	// summaryEntry matches the summary for a flag that the test evaluated directly.
	summaryEntry := func(counter m.Matcher) m.Matcher {
		return m.MapOf(
			m.KV("default", m.JSONEqual(defaultValue)),
			m.KV("counters", m.Items(counter)),
			m.KV("contextKinds", anyContextKindsList()),
		)
	}

	// prereqSummaryEntry matches the summary for a flag that the SDK evaluated as a prerequisite.
	// The default for a prerequisite is always null, so "default" may be present or absent.
	prereqSummaryEntry := func(counter m.Matcher) m.Matcher {
		return m.MapIncluding(
			m.KV("counters", m.Items(counter)),
			m.KV("contextKinds", anyContextKindsList()),
		)
	}

	// prereqFeatureEvent matches the individual feature event for an unaffected prerequisite
	// evaluation. The reason has no override-affected indicator.
	prereqFeatureEvent := func(t *ldtest.T, flag ldmodel.FeatureFlag, value string, prereqOf string) m.Matcher {
		return IsValidFeatureEventWithConditions(
			t, false, context,
			m.JSONProperty("key").Should(m.Equal(flag.Key)),
			m.JSONProperty("version").Should(m.Equal(flag.Version)),
			m.JSONProperty("value").Should(m.Equal(value)),
			m.JSONProperty("variation").Should(m.Equal(1)),
			m.JSONProperty("reason").Should(reasonIsNotOverrideAffected("FALLTHROUGH")),
			JSONPropertyNullOrAbsent("default"),
			m.JSONOptProperty("prereqOf").Should(m.Equal(prereqOf)),
		)
	}

	t.Run("flag with a rule on an overridden segment is marked", func(t *ldtest.T) {
		client, events := setup(t)

		result := evaluate(t, client, segmentFlag.Key, context)
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("included")))
		m.In(t).Assert(result.VariationIndex, m.Equal(o.Some(1)))
		m.In(t).Assert(result.Reason, m.AllOf(
			reasonIsOverrideAffected("RULE_MATCH"),
			m.JSONProperty("ruleId").Should(m.Equal("segment-rule")),
		))

		payload := flushAndGetEvents(t, client, events)
		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			IsIndexEvent(),
			IsValidSummaryEventWithFlags(false,
				m.KV(segmentFlag.Key, summaryEntry(
					overrideAffectedFlagCounter("included", 1, segmentFlag.Version, 1))),
			),
		))
	})

	t.Run("overridden segment that does not match still marks the evaluation", func(t *ldtest.T) {
		// The SDK reads the overridden segment to test the rule. The read marks the evaluation
		// even though the segment does not include this context and the rule does not match.
		client, events := setup(t)

		result := evaluate(t, client, segmentFlag.Key, otherContext)
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("not-included")))
		m.In(t).Assert(result.VariationIndex, m.Equal(o.Some(0)))
		m.In(t).Assert(result.Reason, reasonIsOverrideAffected("FALLTHROUGH"))

		payload := flushAndGetEvents(t, client, events)
		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			IsIndexEvent(),
			IsValidSummaryEventWithFlags(false,
				m.KV(segmentFlag.Key, summaryEntry(
					overrideAffectedFlagCounter("not-included", 0, segmentFlag.Version, 1))),
			),
		))
	})

	t.Run("flag with an overridden prerequisite is marked", func(t *ldtest.T) {
		client, events := setup(t)

		result := evaluate(t, client, overriddenPrereqFlag.Key, context)
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("affected-value")))
		m.In(t).Assert(result.VariationIndex, m.Equal(o.Some(1)))
		m.In(t).Assert(result.Reason, reasonIsOverrideAffected("FALLTHROUGH"))

		// Neither the flag nor its overridden prerequisite produces an individual event. Both
		// summary counters carry the marker. The prerequisite counter uses the override version.
		payload := flushAndGetEvents(t, client, events)
		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			IsIndexEvent(),
			IsValidSummaryEventWithFlags(false,
				m.KV(overriddenPrereqFlag.Key, summaryEntry(
					overrideAffectedFlagCounter("affected-value", 1, overriddenPrereqFlag.Version, 1))),
				m.KV(overriddenPrereq.Key, prereqSummaryEntry(
					overrideAffectedFlagCounter("prereq-override-value", 1, overriddenPrereq.Version, 1))),
			),
		))
	})

	t.Run("unaffected prerequisite inside a marked evaluation is recorded as usual", func(t *ldtest.T) {
		client, events := setup(t)

		result := evaluate(t, client, mixedPrereqFlag.Key, context)
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("mixed-value")))
		m.In(t).Assert(result.Reason, reasonIsOverrideAffected("FALLTHROUGH"))

		// The top-level flag and the overridden prerequisite are marked and produce no individual
		// event. The unaffected prerequisite read nothing from the override store, so it produces
		// its individual event and an ordinary summary counter.
		payload := flushAndGetEvents(t, client, events)
		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			IsIndexEvent(),
			prereqFeatureEvent(t, plainPrereq, "plain-prereq-value", mixedPrereqFlag.Key),
			IsValidSummaryEventWithFlags(false,
				m.KV(mixedPrereqFlag.Key, summaryEntry(
					overrideAffectedFlagCounter("mixed-value", 1, mixedPrereqFlag.Version, 1))),
				m.KV(overriddenPrereq.Key, prereqSummaryEntry(
					overrideAffectedFlagCounter("prereq-override-value", 1, overriddenPrereq.Version, 1))),
				m.KV(plainPrereq.Key, prereqSummaryEntry(
					flagCounter("plain-prereq-value", 1, plainPrereq.Version, 1))),
			),
		))
	})

	t.Run("flag with an unaffected prerequisite is not marked", func(t *ldtest.T) {
		// This is the control case. Nothing in this evaluation comes from the override store, so
		// the reason has no indicator, both flags produce individual events, and the counters
		// have no marker.
		client, events := setup(t)

		result := evaluate(t, client, controlFlag.Key, context)
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("control-value")))
		m.In(t).Assert(result.VariationIndex, m.Equal(o.Some(1)))
		m.In(t).Assert(result.Reason, reasonIsNotOverrideAffected("FALLTHROUGH"))

		payload := flushAndGetEvents(t, client, events)
		m.In(t).Assert(payload, m.ItemsInAnyOrder(
			IsIndexEvent(),
			IsValidFeatureEventWithConditions(
				t, false, context,
				m.JSONProperty("key").Should(m.Equal(controlFlag.Key)),
				m.JSONProperty("version").Should(m.Equal(controlFlag.Version)),
				m.JSONProperty("value").Should(m.Equal("control-value")),
				m.JSONProperty("variation").Should(m.Equal(1)),
				m.JSONProperty("reason").Should(reasonIsNotOverrideAffected("FALLTHROUGH")),
				m.JSONProperty("default").Should(m.JSONEqual(defaultValue)),
				JSONPropertyNullOrAbsent("prereqOf"),
			),
			prereqFeatureEvent(t, plainPrereq, "plain-prereq-value", controlFlag.Key),
			IsValidSummaryEventWithFlags(false,
				m.KV(controlFlag.Key, summaryEntry(
					flagCounter("control-value", 1, controlFlag.Version, 1))),
				m.KV(plainPrereq.Key, prereqSummaryEntry(
					flagCounter("plain-prereq-value", 1, plainPrereq.Version, 1))),
			),
		))
	})
}
