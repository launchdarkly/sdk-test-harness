package sdktests

import (
	"fmt"
	"strconv"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

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

	doTest := func(
		t *ldtest.T,
		p bucketingTestParams,
		rolloutKind ldmodel.RolloutKind,
		bucketBy lduser.UserAttribute,
		makeUser func(p bucketingTestParams, shouldMatchRule bool) lduser.User,
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
			Kind:       rolloutKind,
			BucketBy:   bucketBy,
			Seed:       p.seed,
			Variations: makeRolloutVariationsToMatch(expectedBucketValue, expectedFallthroughVar),
		}
		flagRuleRollout := ldmodel.Rollout{
			Kind:       rolloutKind,
			BucketBy:   bucketBy,
			Seed:       p.seed,
			Variations: makeRolloutVariationsToMatch(expectedBucketValue, expectedRuleVar),
		}

		flagWithRollouts := ldbuilders.NewFlagBuilder(p.flagOrSegmentKey).
			On(true).
			Variations(unwantedValue, expectedFallthroughValue, expectedRuleValue, expectedSegmentValue).
			Salt(p.salt).
			Fallthrough(ldmodel.VariationOrRollout{Rollout: flagFallthroughRollout}).
			AddRule(ldbuilders.NewRuleBuilder().
				ID("rule").
				VariationOrRollout(ldmodel.VariationOrRollout{Rollout: flagRuleRollout}).
				Clauses(ldbuilders.Clause(lduser.UserAttribute(matchRuleAttr), ldmodel.OperatorIn, ldvalue.Bool(true)))).
			Build()
		dataBuilder.Flag(flagWithRollouts)

		flagForSegmentKey := flagForSegmentKeyPrefix + p.flagOrSegmentKey
		if !isExperiment {
			segmentWithRollout := ldbuilders.NewSegmentBuilder(p.flagOrSegmentKey).Salt(p.salt).Build()
			segmentWithRollout.Rules = []ldmodel.SegmentRule{
				{
					BucketBy: bucketBy,
					Weight:   expectedBucketValue + bucketValueMarginOfError,
					Clauses:  []ldmodel.Clause{makeClauseThatAlwaysMatches()},
				},
			}
			flagForSegment := ldbuilders.NewFlagBuilder(flagForSegmentKey).
				On(true).
				Variations(unwantedValue, expectedFallthroughValue, expectedRuleValue, expectedSegmentValue).
				FallthroughVariation(unwantedVar).
				AddRule(ldbuilders.NewRuleBuilder().
					ID("rule").
					Variation(expectedSegmentVar).
					Clauses(ldbuilders.SegmentMatchClause(p.flagOrSegmentKey))).
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
		fallthroughResult := evaluateFlagDetail(t, client, flagWithRollouts.Key, makeUser(p, false), defaultValue)
		m.In(t).For(p.describe()+" (in flag fallthrough)").Assert(fallthroughResult, expectedFallthroughResult)

		ruleResult := evaluateFlagDetail(t, client, flagWithRollouts.Key, makeUser(p, true), defaultValue)
		m.In(t).For(p.describe()+" (in flag rule)").Assert(ruleResult, expectedRuleResult)

		if rolloutKind != ldmodel.RolloutKindExperiment {
			segmentResult := evaluateFlagDetail(t, client, flagForSegmentKey, makeUser(p, false), defaultValue)
			m.In(t).For(p.describe()+" (in segment)").Assert(segmentResult, expectedSegmentResult)
		}
	}

	doTests := func(
		t *ldtest.T,
		params []bucketingTestParams,
		rolloutKind ldmodel.RolloutKind,
		bucketBy lduser.UserAttribute,
		makeUser func(p bucketingTestParams, shouldMatchRule bool) lduser.User,
	) {
		for _, p := range params {
			doTest(t, p, rolloutKind, bucketBy, makeUser)
		}
	}

	t.Run("basic bucketing calculations", func(t *ldtest.T) {
		makeUser := func(p bucketingTestParams, shouldMatchRule bool) lduser.User {
			b := lduser.NewUserBuilder(ldvalue.CopyArbitraryValue(p.contextValue).StringValue())
			if shouldMatchRule {
				b.Custom(matchRuleAttr, ldvalue.Bool(true))
			}
			return b.Build()
		}

		t.Run("rollouts", func(t *ldtest.T) {
			doTests(t, makeBucketingTestParams(), ldmodel.RolloutKindRollout, "", makeUser)
		})

		t.Run("experiments", func(t *ldtest.T) {
			doTests(t, makeBucketingTestParamsForExperiments(), ldmodel.RolloutKindExperiment,
				"", makeUser)
		})
	})

	t.Run("secondary key", func(t *ldtest.T) {
		for _, secondary := range []string{"abcdef", ""} {
			makeUser := func(p bucketingTestParams, shouldMatchRule bool) lduser.User {
				b := lduser.NewUserBuilder(ldvalue.CopyArbitraryValue(p.contextValue).StringValue())
				b.Secondary(secondary)
				if shouldMatchRule {
					b.Custom(matchRuleAttr, ldvalue.Bool(true))
				}
				return b.Build()
			}

			desc := selectString(secondary == "", "empty-but-not-undefined secondary key", "non-empty secondary key")
			t.Run(desc, func(t *ldtest.T) {
				for _, isExperiment := range []bool{false, true} {
					// Note: in the SDK versions that this version of sdk-test-harness is for, the defined behavior
					// was that the secondary key could be used for either a rollout or an experiment. In later
					// versions, the secondary key is ignored in experiments and this test logic is changed.
					desc := fmt.Sprintf("affects bucketing calculation in %s", selectString(isExperiment, "experiments", "rollouts"))
					t.Run(desc, func(t *ldtest.T) {
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
							doTest(t, p1, rolloutKind, "", makeUser)
						}
					})
				}
			})
		}
	})

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
					makeUser := func(p bucketingTestParams, shouldMatchRule bool) lduser.User {
						value := ldvalue.CopyArbitraryValue(p.contextValue)
						b := lduser.NewUserBuilder("arbitrary-key")
						b.Custom(bucketBy, value)
						if shouldMatchRule {
							b.Custom(matchRuleAttr, ldvalue.Bool(true))
						}
						return b.Build()
					}
					doTests(t, makeBucketingTestParams(), rolloutKind, lduser.UserAttribute(bucketBy), makeUser)
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
						makeUser := func(_ bucketingTestParams, shouldMatchRule bool) lduser.User {
							b := lduser.NewUserBuilder("arbitrary-key")
							b.Custom(bucketBy, ldvalue.Int(n))
							if shouldMatchRule {
								b.Custom(matchRuleAttr, ldvalue.Bool(true))
							}
							return b.Build()
						}
						doTest(t, p, rolloutKind, lduser.UserAttribute(bucketBy), makeUser)
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
						makeUser := func(p bucketingTestParams, shouldMatchRule bool) lduser.User {
							b := lduser.NewUserBuilder("arbitrary-key")
							b.Custom(bucketBy, value)
							if shouldMatchRule {
								b.Custom(matchRuleAttr, ldvalue.Bool(true))
							}
							return b.Build()
						}
						doTest(t, p, rolloutKind, lduser.UserAttribute(bucketBy), makeUser)
					}
				})

				t.Run("attribute not found", func(t *ldtest.T) {
					bucketBy := "missingAttr"

					makeUser := func(p bucketingTestParams, shouldMatchRule bool) lduser.User {
						b := lduser.NewUserBuilder("arbitrary-key")
						if shouldMatchRule {
							b.Custom(matchRuleAttr, ldvalue.Bool(true))
						}
						return b.Build()
					}
					params := []bucketingTestParams{}
					for _, p := range makeBucketingTestParams() {
						p1 := p
						p1.overrideExpectedValue = ldvalue.NewOptionalInt(0)
						params = append(params, p1)
					}
					doTests(t, params, rolloutKind, lduser.UserAttribute(bucketBy), makeUser)
				})
			})
		}
	})
}
