package sdktests

import (
	"fmt"
	"strconv"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldmodel"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/require"
)

type bucketingTestParams struct {
	flagOrSegmentKey      string
	salt                  string
	seed                  ldvalue.OptionalInt
	contextValue          string // i.e. the user key, or whatever other attribute we might be bucketing by
	overrideExpectedValue ldvalue.OptionalInt
}

func (p bucketingTestParams) describe() string {
	return fmt.Sprintf("%+v", p)
}

func makeBucketingTestParams() []bucketingTestParams {
	return []bucketingTestParams{
		{
			flagOrSegmentKey: "hashKey",
			salt:             "saltyA",
			contextValue:     "userKeyA",
		},
		{
			flagOrSegmentKey: "hashKey",
			salt:             "saltyA",
			contextValue:     "userKeyB",
		},
		{
			flagOrSegmentKey: "hashKey",
			salt:             "saltyA",
			contextValue:     "userKeyC",
		},
	}
}

func makeBucketingTestParamsForExperiments() []bucketingTestParams {
	ret := makeBucketingTestParams()
	ret = append(ret, []bucketingTestParams{
		{
			flagOrSegmentKey: "hashKey",
			salt:             "saltyA",
			contextValue:     "userKeyA",
		},
		{
			flagOrSegmentKey: "hashKey",
			salt:             "saltyA",
			contextValue:     "userKeyB",
		},
		{
			flagOrSegmentKey: "hashKey",
			salt:             "saltyA",
			contextValue:     "userKeyC",
		},
		{
			flagOrSegmentKey: "hashKey",
			salt:             "saltyA",
			contextValue:     "userKeyA",
			seed:             ldvalue.NewOptionalInt(61),
		},
		{
			flagOrSegmentKey: "hashKey",
			salt:             "saltyA",
			contextValue:     "userKeyB",
			seed:             ldvalue.NewOptionalInt(61),
		},
		{
			flagOrSegmentKey: "hashKey",
			salt:             "saltyA",
			contextValue:     "userKeyC",
			seed:             ldvalue.NewOptionalInt(61),
		},
	}...)
	return ret
}

func RunServerSideEvalBucketingTests(t *ldtest.T) {
	unwantedVar, expectedFallthroughVar, expectedRuleVar, expectedSegmentVar := 0, 1, 2, 3
	expectedFallthroughValue := ldvalue.String("expected-value-for-fallthrough")
	expectedRuleValue := ldvalue.String("expected-value-for-rule")
	expectedSegmentValue := ldvalue.String("expected-value-for-segment-match")
	unwantedValue, defaultValue := ldvalue.String("wrong"), ldvalue.String("default")
	matchRuleAttr := "matchrule"
	flagForSegmentKeyPrefix := "matchsegment-"

	bucketValueMarginOfError := 5 // arbitrary small number (+/-0.005% of 100000) to allow for a tiny bit of rounding error

	makeRolloutVariationsToMatch := func(expectedBucketValue int, desiredVariation int) []ldmodel.WeightedVariation {
		bucketWeightBefore := expectedBucketValue - bucketValueMarginOfError
		bucketWeightAfter := 100000 - (bucketWeightBefore + (bucketValueMarginOfError * 2))
		return []ldmodel.WeightedVariation{
			{Variation: unwantedVar, Weight: bucketWeightBefore},
			{Variation: desiredVariation, Weight: bucketValueMarginOfError * 2},
			{Variation: unwantedVar, Weight: bucketWeightAfter},
		}
	}

	// These tests check for consistent computation of bucket values for rollouts/experiments across SDKs.
	// They use the hash algorithm defined in computeExpectedBucketValue rather than relying on any hard-
	// coded expected values, except in cases where we expect a specific edge-case result such as zero.
	//
	// They are very similar to some of the unit tests for the Go evaluation engine (go-server-sdk-evaluation).
	// In that project, we have access to the low-level bucket value result so we can verify it directly;
	// here, we can only check indirectly at the level of a bucket/variation. So we will set up the bucket
	// weights to bracket the expected value.
	//
	// The behavior of old-style percentage rollouts is different in some regards to the behavior of
	// experiments. That's reflected in the structure of these tests.
	//
	// The reason for having the tests in both places, instead of relying only on sdk-test-harness, is that
	// go-server-sdk-evaluation is also used outside of the SDK and is mission-critical logic, so it needs
	// to have full self-test coverage.
	//
	// There are also some parameterized tests for rollouts/experiments in the YAML data files. Those cover
	// more general aspects of the behavior, rather than checking for specific bucket values.
	// doTests := func(
	// 	t *ldtest.T,
	// 	params []bucketingTestParams,
	// 	rolloutKind ldmodel.RolloutKind,
	// 	contextKind ldcontext.Kind,
	// 	bucketBy ldattr.Ref,
	// 	makeContext func(p bucketingTestParams, shouldMatchRule bool) ldcontext.Context,
	// ) {
	// 	flagForSegmentKeyPrefix := "matchsegment-"
	// 	isExperiment := rolloutKind == ldmodel.RolloutKindExperiment

	// 	for _, p := range params {
	// 		dataBuilder := mockld.NewServerSDKDataBuilder()

	// 		flagFallthroughRollout := ldmodel.Rollout{
	// 			Kind:        rolloutKind,
	// 			ContextKind: contextKind,
	// 			BucketBy:    bucketBy,
	// 			Seed:        p.seed,
	// 			Variations:  makeRolloutVariationsToMatch(p.expectedBucketValue, expectedFallthroughVar),
	// 		}
	// 		flagRuleRollout := ldmodel.Rollout{
	// 			Kind:        rolloutKind,
	// 			ContextKind: contextKind,
	// 			BucketBy:    bucketBy,
	// 			Seed:        p.seed,
	// 			Variations:  makeRolloutVariationsToMatch(p.expectedBucketValue, expectedRuleVar),
	// 		}

	// 		flagWithRollouts := ldbuilders.NewFlagBuilder(p.flagOrSegmentKey).
	// 			On(true).
	// 			Variations(unwantedValue, expectedFallthroughValue, expectedRuleValue, expectedSegmentValue).
	// 			Salt(p.salt).
	// 			Fallthrough(ldmodel.VariationOrRollout{Rollout: flagFallthroughRollout}).
	// 			AddRule(ldbuilders.NewRuleBuilder().
	// 				ID("rule").
	// 				VariationOrRollout(ldmodel.VariationOrRollout{Rollout: flagRuleRollout}).
	// 				Clauses(ldbuilders.ClauseWithKind(contextKind, matchRuleAttr, ldmodel.OperatorIn, ldvalue.Bool(true)))).
	// 			Build()
	// 		dataBuilder.Flag(flagWithRollouts)

	// 		flagForSegmentKey := flagForSegmentKeyPrefix + p.flagOrSegmentKey
	// 		if !isExperiment {
	// 			segmentWithRollout := ldbuilders.NewSegmentBuilder(p.flagOrSegmentKey).Salt(p.salt).Build()
	// 			segmentWithRollout.Rules = []ldmodel.SegmentRule{
	// 				{
	// 					BucketBy:           bucketBy,
	// 					Weight:             ldvalue.NewOptionalInt(int(p.expectedBucketValue*100000) + 5),
	// 					Clauses:            []ldmodel.Clause{makeClauseThatAlwaysMatches()},
	// 					RolloutContextKind: contextKind,
	// 				},
	// 			}
	// 			flagForSegment := ldbuilders.NewFlagBuilder(flagForSegmentKey).
	// 				On(true).
	// 				Variations(unwantedValue, expectedFallthroughValue, expectedRuleValue, expectedSegmentValue).
	// 				FallthroughVariation(unwantedVar).
	// 				AddRule(ldbuilders.NewRuleBuilder().
	// 					ID("rule").
	// 					Variation(expectedSegmentVar).
	// 					Clauses(ldbuilders.SegmentMatchClause(p.flagOrSegmentKey))).
	// 				Build()
	// 			dataBuilder.Flag(flagForSegment).Segment(segmentWithRollout)
	// 		}

	// 		dataSource := NewSDKDataSource(t, dataBuilder.Build())
	// 		client := NewSDKClient(t, dataSource)

	// 		expectedFallthroughResult := m.AllOf(
	// 			EvalResponseValue().Should(m.JSONEqual(expectedFallthroughValue)),
	// 			EvalResponseReason().Should(EqualReason(ldreason.NewEvalReasonFallthroughExperiment(isExperiment))),
	// 		)
	// 		expectedRuleResult := m.AllOf(
	// 			EvalResponseValue().Should(m.JSONEqual(expectedRuleValue)),
	// 			EvalResponseReason().Should(EqualReason(ldreason.NewEvalReasonRuleMatchExperiment(0, "rule", isExperiment))),
	// 		)
	// 		expectedSegmentResult := m.AllOf(
	// 			EvalResponseValue().Should(m.JSONEqual(expectedSegmentValue)),
	// 			EvalResponseReason().Should(EqualReason(ldreason.NewEvalReasonRuleMatchExperiment(0, "rule", isExperiment))),
	// 		)
	// 		if bucketBy.IsDefined() && bucketBy.Err() != nil {
	// 			malformedFlagResult := m.AllOf(
	// 				EvalResponseValue().Should(m.JSONEqual(defaultValue)),
	// 				EvalResponseReason().Should(EqualReason(ldreason.NewEvalReasonError(ldreason.EvalErrorMalformedFlag))),
	// 			)
	// 			expectedFallthroughResult = malformedFlagResult
	// 			expectedRuleResult = malformedFlagResult
	// 			expectedSegmentResult = malformedFlagResult
	// 		}

	// 		// The use of .For() here provides a string description in case of a test failure. We could also do this
	// 		// by running a named subtest for each one, but that would make the test output very long due to the large
	// 		// number of parameterized tests.
	// 		fallthroughResult := evaluateFlagDetail(t, client, flagWithRollouts.Key, makeContext(p, false), defaultValue)
	// 		m.In(t).For(p.describe()+" (in flag fallthrough)").Assert(fallthroughResult, expectedFallthroughResult)

	// 		ruleResult := evaluateFlagDetail(t, client, flagWithRollouts.Key, makeContext(p, true), defaultValue)
	// 		m.In(t).For(p.describe()+" (in flag rule)").Assert(ruleResult, expectedRuleResult)

	// 		if rolloutKind != ldmodel.RolloutKindExperiment {
	// 			segmentResult := evaluateFlagDetail(t, client, flagForSegmentKey, makeContext(p, false), defaultValue)
	// 			m.In(t).For(p.describe()+" (in segment)").Assert(segmentResult, expectedSegmentResult)
	// 		}
	doTest := func(
		t *ldtest.T,
		p bucketingTestParams,
		rolloutKind ldmodel.RolloutKind,
		contextKind ldcontext.Kind,
		bucketBy ldattr.Ref,
		makeContext func(p bucketingTestParams, shouldMatchRule bool) ldcontext.Context,
	) {
		isExperiment := rolloutKind == ldmodel.RolloutKindExperiment
		dataBuilder := mockld.NewServerSDKDataBuilder()

		var expectedBucketValue int
		if p.overrideExpectedValue.IsDefined() {
			expectedBucketValue = p.overrideExpectedValue.IntValue()
		} else {
			expectedBucketValue = computeExpectedBucketValue(
				p.contextValue,
				p.flagOrSegmentKey,
				p.salt,
				ldvalue.OptionalString{},
				p.seed,
			)
		}

		flagFallthroughRollout := ldmodel.Rollout{
			Kind:        rolloutKind,
			ContextKind: contextKind,
			BucketBy:    bucketBy,
			Seed:        p.seed,
			Variations:  makeRolloutVariationsToMatch(expectedBucketValue, expectedFallthroughVar),
		}
		flagRuleRollout := ldmodel.Rollout{
			Kind:        rolloutKind,
			ContextKind: contextKind,
			BucketBy:    bucketBy,
			Seed:        p.seed,
			Variations:  makeRolloutVariationsToMatch(expectedBucketValue, expectedRuleVar),
		}

		flagWithRollouts := ldbuilders.NewFlagBuilder(p.flagOrSegmentKey).
			On(true).
			Variations(unwantedValue, expectedFallthroughValue, expectedRuleValue, expectedSegmentValue).
			Salt(p.salt).
			Fallthrough(ldmodel.VariationOrRollout{Rollout: flagFallthroughRollout}).
			AddRule(ldbuilders.NewRuleBuilder().
				ID("rule").
				VariationOrRollout(ldmodel.VariationOrRollout{Rollout: flagRuleRollout}).
				Clauses(ldbuilders.ClauseWithKind(contextKind, matchRuleAttr, ldmodel.OperatorIn, ldvalue.Bool(true)))).
			Build()
		dataBuilder.Flag(flagWithRollouts)

		flagForSegmentKey := flagForSegmentKeyPrefix + p.flagOrSegmentKey
		if !isExperiment {
			segmentWithRollout := ldbuilders.NewSegmentBuilder(p.flagOrSegmentKey).Salt(p.salt).Build()
			segmentWithRollout.Rules = []ldmodel.SegmentRule{
				{
					BucketBy:           bucketBy,
					RolloutContextKind: contextKind,
					Weight:             ldvalue.NewOptionalInt(expectedBucketValue + bucketValueMarginOfError),
					Clauses:            []ldmodel.Clause{makeClauseThatAlwaysMatches()},
				},
			}
			flagForSegment := ldbuilders.NewFlagBuilder(flagForSegmentKey).
				On(true).
				Variations(unwantedValue, expectedFallthroughValue, expectedRuleValue, expectedSegmentValue).
				FallthroughVariation(unwantedVar).
				AddRule(ldbuilders.NewRuleBuilder().
					ID("rule").
					Variation(expectedSegmentVar).
					Clauses(ldbuilders.ClauseRefWithKind(contextKind, ldattr.Ref{}, ldmodel.OperatorSegmentMatch,
						ldvalue.String(p.flagOrSegmentKey)))).
				Build()
			dataBuilder.Flag(flagForSegment).Segment(segmentWithRollout)
		}

		dataSource := NewSDKDataSource(t, dataBuilder.Build())
		client := NewSDKClient(t, dataSource)

		expectedFallthroughResult := m.AllOf(
			EvalResponseValue().Should(m.JSONEqual(expectedFallthroughValue)),
			EvalResponseReason().Should(EqualReason(ldreason.NewEvalReasonFallthroughExperiment(isExperiment))),
		)
		expectedRuleResult := m.AllOf(
			EvalResponseValue().Should(m.JSONEqual(expectedRuleValue)),
			EvalResponseReason().Should(EqualReason(ldreason.NewEvalReasonRuleMatchExperiment(0, "rule", isExperiment))),
		)
		expectedSegmentResult := m.AllOf(
			EvalResponseValue().Should(m.JSONEqual(expectedSegmentValue)),
			EvalResponseReason().Should(EqualReason(ldreason.NewEvalReasonRuleMatchExperiment(0, "rule", isExperiment))),
		)

		// The use of .For() here provides a string description in case of a test failure. We could also do this
		// by running a named subtest for each one, but that would make the test output very long due to the large
		// number of parameterized tests.
		fallthroughResult := evaluateFlagDetail(t, client, flagWithRollouts.Key, makeContext(p, false), defaultValue)
		m.In(t).For(p.describe()+" (in flag fallthrough)").Assert(fallthroughResult, expectedFallthroughResult)

		ruleResult := evaluateFlagDetail(t, client, flagWithRollouts.Key, makeContext(p, true), defaultValue)
		m.In(t).For(p.describe()+" (in flag rule)").Assert(ruleResult, expectedRuleResult)

		if rolloutKind != ldmodel.RolloutKindExperiment {
			segmentResult := evaluateFlagDetail(t, client, flagForSegmentKey, makeContext(p, false), defaultValue)
			m.In(t).For(p.describe()+" (in segment)").Assert(segmentResult, expectedSegmentResult)
		}
	}

	doTests := func(
		t *ldtest.T,
		params []bucketingTestParams,
		rolloutKind ldmodel.RolloutKind,
		contextKind ldcontext.Kind,
		bucketBy ldattr.Ref,
		makeContext func(p bucketingTestParams, shouldMatchRule bool) ldcontext.Context,
	) {
		for _, p := range params {
			doTest(t, p, rolloutKind, contextKind, bucketBy, makeContext)
		}
	}

	t.Run("basic bucketing calculations", func(t *ldtest.T) {
		makeContext := func(p bucketingTestParams, shouldMatchRule bool) ldcontext.Context {
			b := ldcontext.NewBuilder(ldvalue.CopyArbitraryValue(p.contextValue).StringValue())
			if shouldMatchRule {
				b.SetBool(matchRuleAttr, true)
			}
			return b.Build()
		}

		t.Run("rollouts", func(t *ldtest.T) {
			doTests(t, makeBucketingTestParams(), ldmodel.RolloutKindRollout, ldcontext.DefaultKind, ldattr.Ref{}, makeContext)
		})

		t.Run("experiments", func(t *ldtest.T) {
			doTests(t, makeBucketingTestParamsForExperiments(), ldmodel.RolloutKindExperiment,
				ldcontext.DefaultKind, ldattr.Ref{}, makeContext)
		})
	})

	t.Run("selection of context", func(t *ldtest.T) {
		for _, multi := range []bool{false, true} {
			desc := selectString(multi, "multi-kind", "single-kind")
			contextKind := ldcontext.Kind("org")

			t.Run(desc, func(t *ldtest.T) {
				makeContext := func(p bucketingTestParams, shouldMatchRule bool) ldcontext.Context {
					b := ldcontext.NewBuilder(p.contextValue).Kind(contextKind)
					if shouldMatchRule {
						b.SetBool(matchRuleAttr, true)
					}
					if multi {
						return ldcontext.NewMulti(ldcontext.NewWithKind("wrongkind", "wrongkey"), b.Build())
					} else {
						return b.Build()
					}
				}

				t.Run(desc, func(t *ldtest.T) {
					t.Run("rollouts", func(t *ldtest.T) {
						doTests(t, makeBucketingTestParams(), ldmodel.RolloutKindRollout, contextKind, ldattr.Ref{}, makeContext)
					})

					t.Run("experiments", func(t *ldtest.T) {
						doTests(t, makeBucketingTestParamsForExperiments(), ldmodel.RolloutKindExperiment,
							contextKind, ldattr.Ref{}, makeContext)
					})
				})
			})
		}
	})

	t.Run("secondary key", func(t *ldtest.T) {
		for _, secondary := range []string{"abcdef", ""} {
			makeContext := func(p bucketingTestParams, shouldMatchRule bool) ldcontext.Context {
				b := ldcontext.NewBuilder(p.contextValue)
				b.Secondary(secondary)
				if shouldMatchRule {
					b.SetBool(matchRuleAttr, true)
				}
				return b.Build()
			}

			desc := selectString(secondary == "", "secondary key is an empty string", "secondary key is a non-empty string")
			t.Run(desc, func(t *ldtest.T) {
				for _, isExperiment := range []bool{false, true} {
					// Note: in the SDK versions that this version of sdk-test-harness is for, the defined behavior
					// was that the secondary key could be used for either a rollout or an experiment. In later
					// versions, the secondary key is ignored in experiments and this test logic is changed.
					desc := fmt.Sprintf("affects bucketing calculation in %s", selectString(isExperiment, "experiments", "rollouts"))
					t.Run(desc, func(t *ldtest.T) {
						if secondary == "" {
							t.NonCritical(
								`If the "secondary key is an empty string" tests fail but the other tests pass, it means the SDK is` +
									` treating an empty string the same as null/undefined. That is an unlikely edge case, but the spec` +
									` says the secondary key should have an effect even if its value is "".`,
							)
						}

						var allParams []bucketingTestParams
						rolloutKind := ldmodel.RolloutKindRollout
						if isExperiment {
							allParams = makeBucketingTestParamsForExperiments()
							rolloutKind = ldmodel.RolloutKindExperiment
						} else {
							allParams = makeBucketingTestParams()
						}
						for _, p := range allParams {
							expectedValueWithoutSecondary := computeExpectedBucketValue(
								p.contextValue,
								p.flagOrSegmentKey,
								p.salt,
								ldvalue.OptionalString{},
								p.seed,
							)
							expectedValueWithSecondary := computeExpectedBucketValue(
								p.contextValue,
								p.flagOrSegmentKey,
								p.salt,
								ldvalue.NewOptionalString(secondary),
								p.seed,
							)
							require.NotEqual(t, expectedValueWithoutSecondary, expectedValueWithSecondary)
							p1 := p
							p1.overrideExpectedValue = ldvalue.NewOptionalInt(expectedValueWithSecondary)
							doTest(t, p1, rolloutKind, ldcontext.DefaultKind, ldattr.Ref{}, makeContext)
						}
					})
				}
			})
		}
	})

	// <<<<<<< HEAD
	// 	t.Run("bucket by non-key attribute (rollout only)", func(t *ldtest.T) {
	// 		contextKind := ldcontext.Kind("org")

	// 		t.Run("string value", func(t *ldtest.T) {
	// 			for _, bucketBy := range []ldattr.Ref{ldattr.NewNameRef("attr1"), ldattr.NewRef("/attr1/subprop")} {
	// 				desc := selectString(bucketBy.Depth() == 1, "simple attribute name", "complex attribute reference")

	// 				makeContext := func(p bucketingTestParams, shouldMatchRule bool) ldcontext.Context {
	// 					value := ldvalue.CopyArbitraryValue(p.contextValue)
	// 					b := ldcontext.NewBuilder(value.StringValue()).Kind(contextKind)
	// 					if bucketBy.IsDefined() && bucketBy.String() != "key" {
	// 						b.Key("arbitrary-key")
	// 						setContextValueForAttrRef(b, bucketBy, value)
	// 					}
	// 					if p.secondaryKey != "" {
	// 						b.Secondary(p.secondaryKey)
	// 					}
	// 					if shouldMatchRule {
	// 						b.SetBool(matchRuleAttr, true)
	// 					}
	// 					// For this test, we'll always use a multi-kind context to prove that the bucketBy attribute is
	// 					// being retrieved from the appropriate place depending on contextKind, and not just being
	// 					// blindly applied to the base context.
	// 					return ldcontext.NewMulti(ldcontext.NewWithKind("wrongkind", "wrongkey"), b.Build())
	// 				}

	// 				t.Run(desc, func(t *ldtest.T) {
	// 					doTests(t, makeBucketingTestParams(), ldmodel.RolloutKindRollout, contextKind, bucketBy, makeContext)
	// 				})
	// 			}
	// 		})

	// 		t.Run("non-string value", func(t *ldtest.T) {
	// 			bucketBy := ldattr.NewRef("attr1")
	// 			makeContext := func(p bucketingTestParams, shouldMatchRule bool) ldcontext.Context {
	// 				value := ldvalue.CopyArbitraryValue(p.contextValue)
	// 				b := ldcontext.NewBuilder("arbitrary-key")
	// 				setContextValueForAttrRef(b, bucketBy, value)
	// 				if shouldMatchRule {
	// 					b.SetBool(matchRuleAttr, true)
	// 				}
	// 				return b.Build()
	// 			}
	// 			doTests(t, makeBucketingTestParamsWithNonStringValues(), ldmodel.RolloutKindRollout,
	// 				ldcontext.DefaultKind, bucketBy, makeContext)
	// 		})

	// 		t.Run("attribute not found", func(t *ldtest.T) {
	// 			bucketBy := ldattr.NewRef("missingAttr")

	// 			makeContext := func(p bucketingTestParams, shouldMatchRule bool) ldcontext.Context {
	// 				b := ldcontext.NewBuilder("arbitrary-key")
	// 				if shouldMatchRule {
	// 					b.SetBool(matchRuleAttr, true)
	// 				}
	// 				return b.Build()
	// 			}
	// 			params := []bucketingTestParams{}
	// 			for _, p := range makeBucketingTestParams() {
	// 				p1 := p
	// 				p1.expectedBucketValue = 0
	// 				params = append(params, p1)
	// 			}
	// 			doTests(t, params, ldmodel.RolloutKindRollout,
	// 				ldcontext.DefaultKind, bucketBy, makeContext)
	// 		})

	// 		t.Run("invalid attribute reference", func(t *ldtest.T) {
	// 			bucketBy := ldattr.NewRef("///")

	// 			makeContext := func(p bucketingTestParams, shouldMatchRule bool) ldcontext.Context {
	// 				b := ldcontext.NewBuilder("arbitrary-key")
	// 				if shouldMatchRule {
	// 					b.SetBool(matchRuleAttr, true)
	// 				}
	// 				return b.Build()
	// 			}
	// 			params := []bucketingTestParams{}
	// 			for _, p := range makeBucketingTestParams() {
	// 				p1 := p
	// 				p1.expectedBucketValue = 0
	// 				params = append(params, p1)
	// 			}
	// 			doTests(t, params, ldmodel.RolloutKindRollout,
	// 				ldcontext.DefaultKind, bucketBy, makeContext)
	// 		})
	t.Run("bucket by non-key attribute", func(t *ldtest.T) {
		// Note: in the SDK versions that this version of sdk-test-harness is for, the defined behavior
		// was that the bucketBy property could be used for either a rollout or an experiment. In later
		// versions, bucketBy is ignored in experiments and this test logic is changed.

		for _, isExperiment := range []bool{false, true} {
			t.Run(selectString(isExperiment, "experiments", "rollouts"), func(t *ldtest.T) {
				rolloutKind := ldmodel.RolloutKindRollout
				if isExperiment {
					rolloutKind = ldmodel.RolloutKindExperiment
				}

				t.Run("string value", func(t *ldtest.T) {
					bucketBy := "attr1"
					makeContext := func(p bucketingTestParams, shouldMatchRule bool) ldcontext.Context {
						b := ldcontext.NewBuilder("arbitrary-key")
						b.SetString(bucketBy, p.contextValue)
						if shouldMatchRule {
							b.SetBool(matchRuleAttr, true)
						}
						return b.Build()
					}
					doTests(t, makeBucketingTestParams(), rolloutKind, ldcontext.DefaultKind, ldattr.NewRef(bucketBy), makeContext)
				})

				t.Run("integer value", func(t *ldtest.T) {
					bucketBy := "attr1"
					flagKey, salt := "hashKey", "saltyA"
					for _, n := range []int{33333, 99999} {
						expectedValue := computeExpectedBucketValue(
							strconv.Itoa(n),
							flagKey, salt, ldvalue.OptionalString{}, ldvalue.OptionalInt{},
						)
						p := bucketingTestParams{
							flagOrSegmentKey:      flagKey,
							salt:                  salt,
							overrideExpectedValue: ldvalue.NewOptionalInt(expectedValue),
						}
						makeContext := func(_ bucketingTestParams, shouldMatchRule bool) ldcontext.Context {
							b := ldcontext.NewBuilder("arbitrary-key")
							b.SetInt(bucketBy, n)
							if shouldMatchRule {
								b.SetBool(matchRuleAttr, true)
							}
							return b.Build()
						}
						doTest(t, p, rolloutKind, ldcontext.DefaultKind, ldattr.NewRef(bucketBy), makeContext)
					}
				})

				t.Run("invalid value type", func(t *ldtest.T) {
					// Non-integer numeric values, and any value types other than string and number, are not allowed
					// and cause the bucket value to be zero.
					bucketBy := "attr1"
					flagKey, salt := "hashKey", "saltyA"
					for _, value := range []ldvalue.Value{
						ldvalue.Float64(1.5),
						ldvalue.Bool(true),
						ldvalue.ArrayOf(ldvalue.String("x")),
						ldvalue.ObjectBuild().Set("x", ldvalue.String("y")).Build(),
					} {
						p := bucketingTestParams{
							flagOrSegmentKey:      flagKey,
							salt:                  salt,
							overrideExpectedValue: ldvalue.NewOptionalInt(0),
						}
						makeContext := func(_ bucketingTestParams, shouldMatchRule bool) ldcontext.Context {
							b := ldcontext.NewBuilder("arbitrary-key")
							b.SetValue(bucketBy, value)
							if shouldMatchRule {
								b.SetBool(matchRuleAttr, true)
							}
							return b.Build()
						}
						doTest(t, p, rolloutKind, ldcontext.DefaultKind, ldattr.NewRef(bucketBy), makeContext)
					}
				})

				t.Run("attribute not found", func(t *ldtest.T) {
					bucketBy := "missingAttr"

					makeContext := func(_ bucketingTestParams, shouldMatchRule bool) ldcontext.Context {
						b := ldcontext.NewBuilder("arbitrary-key")
						if shouldMatchRule {
							b.SetBool(matchRuleAttr, true)
						}
						return b.Build()
					}
					params := []bucketingTestParams{}
					for _, p := range makeBucketingTestParams() {
						p1 := p
						p1.overrideExpectedValue = ldvalue.NewOptionalInt(0)
						params = append(params, p1)
					}
					doTests(t, params, rolloutKind, ldcontext.DefaultKind, ldattr.NewRef(bucketBy), makeContext)
				})
			})
		}
	})
}
