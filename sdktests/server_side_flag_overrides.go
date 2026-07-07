package sdktests

import (
	"time"

	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

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

// reasonIsOverride matches a raw JSON evaluation reason with the given kind and isOverride: true.
func reasonIsOverride(kind string) m.Matcher {
	return m.AllOf(
		m.JSONProperty("kind").Should(m.Equal(kind)),
		m.JSONProperty("isOverride").Should(m.Equal(true)),
	)
}

// reasonIsNotOverride matches a raw JSON evaluation reason with the given kind and no isOverride
// property (the property must be omitted, not false, when the evaluation was not overridden).
func reasonIsNotOverride(kind string) m.Matcher {
	return m.AllOf(
		m.JSONProperty("kind").Should(m.Equal(kind)),
		m.JSONOptProperty("isOverride").Should(m.BeNil()),
	)
}

func doServerSideFlagOverridesTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityFlagOverrides)

	t.Run("static configuration", doServerSideFlagOverridesStaticTests)
	t.Run("uninitialized client", doServerSideFlagOverridesUninitializedTests)
	t.Run("summary events", doServerSideFlagOverridesSummaryEventTest)
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
		m.In(t).Assert(result.Reason, reasonIsOverride("OFF"))
	})

	t.Run("full flag override evaluates targeting rules", func(t *ldtest.T) {
		result := evaluateFlagDetailRawReason(t, client, servicedef.EvaluateFlagParams{
			FlagKey: overrideRuleFlag.Key, Context: o.Some(context), DefaultValue: defaultValue})
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("rule-value")))
		m.In(t).Assert(result.VariationIndex, m.Equal(o.Some(1)))
		m.In(t).Assert(result.Reason, m.AllOf(
			reasonIsOverride("RULE_MATCH"),
			m.JSONProperty("ruleId").Should(m.Equal("override-rule")),
		))
	})

	t.Run("non-overridden flag is unaffected", func(t *ldtest.T) {
		result := evaluateFlagDetailRawReason(t, client, servicedef.EvaluateFlagParams{
			FlagKey: ldFlagNormal.Key, Context: o.Some(context), DefaultValue: defaultValue})
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("normal-value")))
		m.In(t).Assert(result.Reason, reasonIsNotOverride("OFF"))
	})

	t.Run("overridden flag rule can reference overridden segment", func(t *ldtest.T) {
		result := evaluateFlagDetailRawReason(t, client, servicedef.EvaluateFlagParams{
			FlagKey: overrideSegmentFlag.Key, Context: o.Some(context), DefaultValue: defaultValue})
		m.In(t).Assert(result.Value, m.JSONEqual(ldvalue.String("included")))
		m.In(t).Assert(result.Reason, reasonIsOverride("RULE_MATCH"))
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
		m.In(t).Assert(result.Reason, reasonIsOverride("OFF"))
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

	// The overridden flag is a full flag definition with trackEvents: true, which for an ordinary
	// flag would produce an individual feature event for each evaluation. Override evaluations
	// must not produce individual feature or debug events-- only index and summary events.
	trackedOverrideFlag := ldbuilders.NewFlagBuilder("flag-tracked-override").Version(300).
		On(false).OffVariation(0).Variations(ldvalue.String("override-value")).
		TrackEvents(true).
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

	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{FlagKey: trackedOverrideFlag.Key,
		Context: o.Some(context), DefaultValue: default1})
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
					overrideFlagCounter("override-value", 0, trackedOverrideFlag.Version, 1),
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
	m.In(t).Assert(result.Reason, reasonIsOverride("OFF"))
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
		{"watch mode", func(paths ...string) servicedef.SDKConfigOverridesParams {
			return servicedef.SDKConfigOverridesParams{FilePaths: paths} // watch is the default
		}},
		{"poll mode", func(paths ...string) servicedef.SDKConfigOverridesParams {
			return servicedef.SDKConfigOverridesParams{
				FilePaths:      paths,
				Watch:          o.Some(false),
				Poll:           o.Some(true),
				PollIntervalMS: o.Some(1000),
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
