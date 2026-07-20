package sdktests

import (
	"fmt"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/data"
	h "github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

// This file is very similar to server_side_events_eval.go, except:
//
// - The test data generation works differently because of the different flag model.
// - We're not using a unique user per evaluation.
// - There are no prerequisite events.

func doClientSideFeatureEventTests(t *ldtest.T) {
	valueFactories := data.MakeValueFactoriesBySDKValueType(2)
	flagValues, defaultValues := valueFactories[0], valueFactories[1]
	context := data.NewContextFactory("doClientSideFeatureEventTests").NextUniqueContext()
	expectedReason := ldreason.NewEvalReasonFallthrough()
	untrackedFlags := data.NewClientSideFlagFactory(
		"untracked-flag",
		flagValues,
		data.ClientSideFlagShouldHaveEvalReason(expectedReason),
	)
	trackedFlags := data.NewClientSideFlagFactory(
		"tracked-flag",
		flagValues,
		data.ClientSideFlagShouldHaveEvalReason(expectedReason),
		data.ClientSideFlagShouldHaveFullEventTracking,
	)

	dataBuilder := mockld.NewClientSDKDataBuilder()
	for _, valueType := range getValueTypesToTest(t) {
		dataBuilder.FullFlag(untrackedFlags.MakeFlagForValueType(valueType))
		dataBuilder.FullFlag(trackedFlags.MakeFlagForValueType(valueType))
	}

	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))

	client := NewSDKClient(t,
		WithClientSideInitialContext(context),
		dataSource, events)

	client.FlushEvents(t)
	_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event

	t.Run("only summary event for untracked flag", func(t *ldtest.T) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				flag := untrackedFlags.ReuseFlagForValueType(valueType)

				resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flag.Key,
					ValueType:    valueType,
					DefaultValue: defaultValues(valueType),
				})

				// If the evaluation didn't return the expected value, then the rest of the test is moot
				if !m.In(t).Assert(flag.Value, m.JSONEqual(resp.Value)) {
					require.Fail(t, "evaluation unexpectedly returned wrong value")
				}

				client.FlushEvents(t)
				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					IsSummaryEvent(),
				))
			})
		}
	})

	doFeatureEventTest := func(t *ldtest.T, withReason bool) {
		for _, valueType := range getValueTypesToTest(t) {
			t.Run(testDescFromType(valueType), func(t *ldtest.T) {
				flag := trackedFlags.ReuseFlagForValueType(valueType)
				expectedValue := flag.Value
				expectedVariation := flag.Variation
				resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
					FlagKey:      flag.Key,
					ValueType:    valueType,
					DefaultValue: defaultValues(valueType),
					Detail:       withReason,
				})

				// If the evaluation didn't return the expected value, then the rest of the test is moot
				if !m.In(t).Assert(expectedValue, m.JSONEqual(resp.Value)) {
					require.Fail(t, "evaluation unexpectedly returned wrong value")
				}

				client.FlushEvents(t)

				matchFeatureEvent := IsValidFeatureEventWithConditions(
					t, false, context,
					m.JSONProperty("key").Should(m.Equal(flag.Key)),
					m.JSONProperty("version").Should(m.Equal(flag.FlagVersion.Value())),
					m.JSONProperty("value").Should(m.JSONEqual(expectedValue)),
					m.JSONOptProperty("variation").Should(m.JSONEqual(expectedVariation)),
					maybeReason(withReason, expectedReason),
					m.JSONProperty("default").Should(m.JSONEqual(defaultValues(valueType))),
					JSONPropertyNullOrAbsent("prereqOf"),
				)

				payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
				m.In(t).Assert(payload, m.ItemsInAnyOrder(
					matchFeatureEvent,
					IsSummaryEvent(),
				))
			})
		}
	}

	t.Run("full feature event for tracked flag", func(t *ldtest.T) {
		for _, withReason := range []bool{false, true} {
			t.Run(h.IfElse(withReason, "with reason", "without reason"), func(t *ldtest.T) {
				doFeatureEventTest(t, withReason)
			})
		}
	})

	if t.Capabilities().Has(servicedef.CapabilityAnonymousRedaction) {
		t.Run("single-kind anonymous context redacts all attributes", func(t *ldtest.T) {
			anonymousFactory := data.NewContextFactory("anonymous", func(b *ldcontext.Builder) {
				b.Anonymous(true)
				b.Name("Example name")
				b.SetString("setup", "Why do programmers always confused Halloween and Christmas?")
				b.SetString("punchline", "Because OCT 31 = DEC 25")
			})

			for _, valueType := range getValueTypesToTest(t) {
				t.Run(testDescFromType(valueType), func(t *ldtest.T) {
					flag := trackedFlags.ReuseFlagForValueType(valueType)
					expectedValue := flag.Value
					anonymousContext := anonymousFactory.NextUniqueContext()

					client.SendIdentifyEvent(t, anonymousContext)
					client.FlushEvents(t)
					_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event

					resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
						FlagKey:      flag.Key,
						ValueType:    valueType,
						DefaultValue: defaultValues(valueType),
						Detail:       true,
					})

					// If the evaluation didn't return the expected value, then the rest of the test is moot
					if !m.In(t).Assert(expectedValue, m.JSONEqual(resp.Value)) {
						require.Fail(t, "evaluation unexpectedly returned wrong value")
					}

					client.FlushEvents(t)

					expectedContext := ldcontext.NewBuilderFromContext(anonymousContext).
						SetValue("name", ldvalue.Null()).
						SetValue("setup", ldvalue.Null()).
						SetValue("punchline", ldvalue.Null()).
						Build()

					matcher := JSONMatchesEventContext(expectedContext, map[string][]string{"user": {"name", "setup", "punchline"}})

					payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						IsValidFeatureEventWithConditions(t, false, anonymousContext, m.JSONProperty("context").Should(matcher)),
						EventHasKind("summary"),
					))
				})
			}
		})

		t.Run("multi-kind with anonymous context redacts attributes appropriately", func(t *ldtest.T) {
			userContextFactory := data.NewContextFactory("user", func(b *ldcontext.Builder) {
				b.Anonymous(true)
				b.Kind("user")
				b.Name("User name")
				b.SetString("setup", "Why do programmers always confused Halloween and Christmas?")
				b.SetString("punchline", "Because OCT 31 = DEC 25")
			})
			orgContextFactory := data.NewContextFactory("org", func(b *ldcontext.Builder) {
				b.Name("Org name")
				b.Kind("org")
				b.SetString("setup", "Why did the edge server go bankrupt?")
				b.SetString("punchline", "Because it ran out of cache")
			})

			for _, valueType := range getValueTypesToTest(t) {
				t.Run(testDescFromType(valueType), func(t *ldtest.T) {
					userContext := userContextFactory.NextUniqueContext()
					orgContext := orgContextFactory.NextUniqueContext()

					multiContext := ldcontext.NewMultiBuilder().Add(userContext).Add(orgContext).Build()

					client.SendIdentifyEvent(t, multiContext)
					client.FlushEvents(t)
					_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event

					flag := trackedFlags.ReuseFlagForValueType(valueType)
					expectedValue := flagValues(valueType)
					resp := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
						FlagKey:      flag.Key,
						ValueType:    valueType,
						DefaultValue: defaultValues(valueType),
						Detail:       false,
					})

					// If the evaluation didn't return the expected value, then the rest of the test is moot
					if !m.In(t).Assert(expectedValue, m.JSONEqual(resp.Value)) {
						require.Fail(t, "evaluation unexpectedly returned wrong value")
					}

					client.FlushEvents(t)

					expectedUser := ldcontext.NewBuilderFromContext(userContext).
						SetValue("name", ldvalue.Null()).
						SetValue("setup", ldvalue.Null()).
						SetValue("punchline", ldvalue.Null()).
						Build()

					expectedMultiKind := ldcontext.NewMultiBuilder().Add(expectedUser).Add(orgContext).Build()

					matcher := JSONMatchesEventContext(expectedMultiKind, map[string][]string{"user": {"name", "setup", "punchline"}})

					payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)
					m.In(t).Assert(payload, m.ItemsInAnyOrder(
						IsValidFeatureEventWithConditions(t, false, multiContext, m.JSONProperty("context").Should(matcher)),
						EventHasKind("summary"),
					))
				})
			}
		})

		// Restore the client to the effective context object prior to this block.
		client.SendIdentifyEvent(t, context)
		client.FlushEvents(t)
		_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout)
	}

	t.Run("evaluating all flags generates no events", func(t *ldtest.T) {
		_ = client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{})
		client.FlushEvents(t)
		events.ExpectNoAnalyticsEvents(t, time.Millisecond*200)
	})
}

func doClientSideInOrderPrereqEventTests(t *ldtest.T) {
	dataBuilder := mockld.NewClientSDKDataBuilder()
	dataBuilder.
		Flag("topLevel", mockld.ClientSDKFlag{
			Value:         ldvalue.String("value1"),
			Variation:     o.Some(0),
			TrackEvents:   true,
			Prerequisites: []string{"prereq1", "preqreq2"},
		}).
		Flag("prereq1", mockld.ClientSDKFlag{
			Value:         ldvalue.String("value2"),
			TrackEvents:   true,
			Variation:     o.Some(0),
			Prerequisites: []string{"prereq2"},
		}).
		Flag("prereq2", mockld.ClientSDKFlag{
			Value:       ldvalue.String("value3"),
			TrackEvents: true,
			Variation:   o.Some(0),
		})
	dataSource := NewSDKDataSource(t, dataBuilder.Build())
	context := ldcontext.New("user")

	events := NewSDKEventSink(t)
	client := NewSDKClient(t,
		WithClientSideInitialContext(context),
		dataSource, events)

	_ = client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
		FlagKey:      "topLevel",
		DefaultValue: ldvalue.Null(),
		ValueType:    servicedef.ValueTypeAny,
	})

	client.FlushEvents(t)
	payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

	prereq2FeatureEvent := IsValidFeatureEventWithConditions(
		t, false, context,
		m.JSONProperty("key").Should(m.Equal("prereq2")),
		m.JSONProperty("version").Should(m.Equal(0)),
		m.JSONProperty("value").Should(m.JSONEqual(ldvalue.String("value3"))),
		m.JSONOptProperty("variation").Should(m.JSONEqual(0)),
		JSONPropertyNullOrAbsent("prereqOf"),
	)

	prereq1FeatureEvent := IsValidFeatureEventWithConditions(
		t, false, context,
		m.JSONProperty("key").Should(m.Equal("prereq1")),
		m.JSONProperty("version").Should(m.Equal(0)),
		m.JSONProperty("value").Should(m.JSONEqual(ldvalue.String("value2"))),
		m.JSONOptProperty("variation").Should(m.JSONEqual(0)),
		JSONPropertyNullOrAbsent("prereqOf"),
	)

	topLevelFeatureEvent := IsValidFeatureEventWithConditions(
		t, false, context,
		m.JSONProperty("key").Should(m.Equal("topLevel")),
		m.JSONProperty("version").Should(m.Equal(0)),
		m.JSONProperty("value").Should(m.JSONEqual(ldvalue.String("value1"))),
		m.JSONOptProperty("variation").Should(m.JSONEqual(0)),
		JSONPropertyNullOrAbsent("prereqOf"),
	)

	m.In(t).Assert(payload, m.ItemsInAnyOrder(
		IsIdentifyEventForContext(context),
		prereq2FeatureEvent,
		prereq1FeatureEvent,
		topLevelFeatureEvent,
		IsSummaryEvent(),
	))
}

func doClientSideDebugEventTests(t *ldtest.T) {
	// These tests could misbehave if the system clocks of the host that's running the test harness
	// and the host that's running the test service are out of sync by at least an hour. However,
	// in normal usage those are the same host.

	valueFactories := data.MakeValueFactoriesBySDKValueType(2)
	flagValues, defaultValues := valueFactories[0], valueFactories[1]
	contexts := data.NewContextFactory("doClientSideDebugEventTests")
	expectedReason := ldreason.NewEvalReasonFallthrough()

	doDebugTest := func(
		t *ldtest.T,
		shouldSeeDebugEvent bool,
		flagDebugUntil time.Time,
		lastKnownTimeFromLD time.Time,
	) {
		context := contexts.NextUniqueContext()
		flags := data.NewClientSideFlagFactory(
			"flag",
			flagValues,
			data.ClientSideFlagShouldHaveEvalReason(expectedReason),
			data.ClientSideFlagShouldHaveDebuggingEnabledUntil(flagDebugUntil),
		)
		dataBuilder := mockld.NewClientSDKDataBuilder()
		for _, valueType := range getValueTypesToTest(t) {
			dataBuilder.FullFlag(flags.MakeFlagForValueType(valueType))
		}
		dataSource := NewSDKDataSource(t, dataBuilder.Build())

		events := NewSDKEventSinkWithGzip(t, t.Capabilities().Has(servicedef.CapabilityEventGzip))
		if !lastKnownTimeFromLD.IsZero() {
			events.Service().SetHostTimeOverride(lastKnownTimeFromLD)
		}

		client := NewSDKClient(t,
			WithClientSideInitialContext(context),
			dataSource, events)

		client.FlushEvents(t)
		_ = events.ExpectAnalyticsEvents(t, defaultEventTimeout) // discard initial identify event
		// note, this initial flush also causes the SDK to see the Date header in the mock event service's response

		if !lastKnownTimeFromLD.IsZero() {
			// Hacky arbitrary sleep to avoid a race condition where the test code runs fast enough
			// that the SDK has not had a chance to process the HTTP response yet - the fact that
			// we've received the event payload from them doesn't mean the SDK has done that work
			time.Sleep(time.Millisecond * 10)
		}

		for _, withReasons := range []bool{false, true} {
			t.Run(h.IfElse(withReasons, "with reasons", "without reasons"), func(t *ldtest.T) {
				for _, valueType := range getValueTypesToTest(t) {
					t.Run(testDescFromType(valueType), func(t *ldtest.T) {
						flag := flags.ReuseFlagForValueType(valueType)
						result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
							FlagKey:      flag.Key,
							ValueType:    valueType,
							DefaultValue: defaultValues(valueType),
							Detail:       withReasons,
						})
						m.In(t).Assert(result.Value, m.JSONEqual(flag.Value))

						client.FlushEvents(t)
						payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

						if shouldSeeDebugEvent {
							matchDebugEvent := m.AllOf(
								JSONPropertyKeysCanOnlyBe("kind", "creationDate", "key", "context",
									"version", "value", "variation", "reason", "default"),
								IsDebugEvent(),
								HasAnyCreationDate(),
								m.JSONProperty("key").Should(m.Equal(flag.Key)),
								HasContextObjectWithMatchingKeys(context),
								m.JSONProperty("version").Should(m.Equal(flag.FlagVersion.Value())),
								m.JSONProperty("value").Should(m.JSONEqual(result.Value)),
								m.JSONProperty("variation").Should(m.JSONEqual(flag.Variation)),
								maybeReason(withReasons, expectedReason),
								m.JSONProperty("default").Should(m.JSONEqual(defaultValues(valueType))),
							)
							m.In(t).Assert(payload, m.ItemsInAnyOrder(
								matchDebugEvent,
								EventHasKind("summary"),
							))
						} else {
							m.In(t).Assert(payload, m.ItemsInAnyOrder(
								EventHasKind("summary"),
							))
						}
					})
				}
			})
		}
	}

	doDebugEventTestCases(t, doDebugTest)
}

// doClientSidePrereqCycleTests exercises CSPE 1.2.5, 1.2.5.1, and 1.2.5.2: the SDK must
// not enter unbounded recursion when the prerequisites graph reachable from the evaluated
// flag contains a cycle. On cycle detection the SDK skips the offending edge, continues
// processing remaining prerequisites at the current level, and returns the requested
// flag's cached value and reason unchanged.
//
// These cases cover feature-event emission. Companion summary-counter cases live in
// client_side_events_summary.go as doClientSideSummaryPrereqCycleTests.
func doClientSidePrereqCycleTests(t *ldtest.T) {
	context := ldcontext.New("user")

	// runCase evaluates evalKey against the given flag data and asserts:
	//   - the HTTP round-trip to the test service completes (implicit: no SDK crash),
	//   - Requirement 1.2.5.1: the returned value equals expectedValue (the cached
	//     evaluation result of the requested flag, unchanged),
	//   - feature events are emitted for the flags in expectedFeatureEventKeys, plus
	//     the usual identify and summary events. The expected list is finite, so a
	//     failed cycle guard that emits an unbounded event stream would fail the match.
	runCase := func(
		t *ldtest.T,
		dataBuilder *mockld.ClientSDKDataBuilder,
		evalKey string,
		expectedValue ldvalue.Value,
		expectedFeatureEventKeys []string,
	) {
		dataSource := NewSDKDataSource(t, dataBuilder.Build())
		events := NewSDKEventSink(t)
		client := NewSDKClient(t,
			WithClientSideInitialContext(context),
			dataSource, events)

		result := client.EvaluateFlag(t, servicedef.EvaluateFlagParams{
			FlagKey:      evalKey,
			DefaultValue: ldvalue.Null(),
			ValueType:    servicedef.ValueTypeAny,
		})

		// Requirement 1.2.5.1: value MUST equal the flag's cached evaluation result.
		m.In(t).Assert(result.Value, m.JSONEqual(expectedValue))

		client.FlushEvents(t)
		payload := events.ExpectAnalyticsEvents(t, defaultEventTimeout)

		expected := []m.Matcher{IsIdentifyEventForContext(context)}
		for _, key := range expectedFeatureEventKeys {
			key := key
			expected = append(expected, IsValidFeatureEventWithConditions(
				t, false, context,
				m.JSONProperty("key").Should(m.Equal(key)),
			))
		}
		expected = append(expected, IsSummaryEvent())
		m.In(t).Assert(payload, m.ItemsInAnyOrder(expected...))
	}

	valTrue := ldvalue.Bool(true)

	makeFlag := func(prereqs ...string) mockld.ClientSDKFlag {
		return mockld.ClientSDKFlag{
			Value:         valTrue,
			Variation:     o.Some(0),
			TrackEvents:   true,
			Prerequisites: prereqs,
		}
	}

	t.Run("self-loop", func(t *ldtest.T) {
		dataBuilder := mockld.NewClientSDKDataBuilder().
			Flag("A", makeFlag("A"))
		// A's only prerequisite is itself; the cycle guard skips it. Only A emits a
		// feature event (as the top-level evaluation).
		runCase(t, dataBuilder, "A", valTrue, []string{"A"})
	})

	t.Run("self-loop with sibling prerequisite", func(t *ldtest.T) {
		dataBuilder := mockld.NewClientSDKDataBuilder().
			Flag("A", makeFlag("A", "B")).
			Flag("B", makeFlag())
		// The self-prereq on A is skipped; B is still evaluated as a sibling.
		runCase(t, dataBuilder, "A", valTrue, []string{"B", "A"})
	})

	t.Run("two-cycle, evaluating A", func(t *ldtest.T) {
		dataBuilder := mockld.NewClientSDKDataBuilder().
			Flag("A", makeFlag("B")).
			Flag("B", makeFlag("A"))
		// A -> B -> [A skipped by cycle guard]. Events: B (as prereq of A), A.
		runCase(t, dataBuilder, "A", valTrue, []string{"B", "A"})
	})

	t.Run("two-cycle, evaluating B", func(t *ldtest.T) {
		dataBuilder := mockld.NewClientSDKDataBuilder().
			Flag("A", makeFlag("B")).
			Flag("B", makeFlag("A"))
		// Symmetric: same graph, but B is the entry point.
		runCase(t, dataBuilder, "B", valTrue, []string{"A", "B"})
	})

	t.Run("three-cycle", func(t *ldtest.T) {
		dataBuilder := mockld.NewClientSDKDataBuilder().
			Flag("A", makeFlag("B")).
			Flag("B", makeFlag("C")).
			Flag("C", makeFlag("A"))
		// A -> B -> C -> [A skipped]. Events (deepest first): C, B, A.
		runCase(t, dataBuilder, "A", valTrue, []string{"C", "B", "A"})
	})

	t.Run("cycle within a subgraph", func(t *ldtest.T) {
		// A has one acyclic branch (X) and one branch that contains a cycle (B <-> C).
		// The cycle must not affect processing of the sibling branch.
		dataBuilder := mockld.NewClientSDKDataBuilder().
			Flag("A", makeFlag("X", "B")).
			Flag("X", makeFlag()).
			Flag("B", makeFlag("C")).
			Flag("C", makeFlag("B"))
		// A -> [X (leaf), B -> C -> [B skipped]]. Events: X, C, B, A.
		runCase(t, dataBuilder, "A", valTrue, []string{"X", "C", "B", "A"})
	})

	t.Run("diamond (non-cyclic control)", func(t *ldtest.T) {
		// Ancestor-set (Requirement 1.2.5.2) semantics: D is reached twice via
		// independent paths and MUST emit a prerequisite event on each. A naive
		// "visited across the whole walk" implementation would silently drop the
		// second D event; this case catches that regression.
		dataBuilder := mockld.NewClientSDKDataBuilder().
			Flag("A", makeFlag("B", "C")).
			Flag("B", makeFlag("D")).
			Flag("C", makeFlag("D")).
			Flag("D", makeFlag())
		// Events: D (via B), B, D (via C), C, A. D appears twice.
		runCase(t, dataBuilder, "A", valTrue, []string{"D", "B", "D", "C", "A"})
	})

	t.Run("deep chain (non-cyclic control)", func(t *ldtest.T) {
		// Verifies that the cycle-detection implementation does not accidentally
		// impose a shallow depth limit that would break legitimate deep prereq trees.
		const depth = 20
		dataBuilder := mockld.NewClientSDKDataBuilder()
		for i := 0; i < depth; i++ {
			key := fmt.Sprintf("f%d", i)
			flag := mockld.ClientSDKFlag{
				Value:       valTrue,
				Variation:   o.Some(0),
				TrackEvents: true,
			}
			if i < depth-1 {
				flag.Prerequisites = []string{fmt.Sprintf("f%d", i+1)}
			}
			dataBuilder.Flag(key, flag)
		}
		// Deepest first: f19, f18, ..., f0.
		expected := make([]string, depth)
		for i := 0; i < depth; i++ {
			expected[i] = fmt.Sprintf("f%d", depth-1-i)
		}
		runCase(t, dataBuilder, "f0", valTrue, expected)
	})
}
