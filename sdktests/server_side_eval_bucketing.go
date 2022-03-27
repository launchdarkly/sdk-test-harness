package sdktests

import (
	"fmt"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
)

type bucketingTestParams struct {
	flagOrSegmentKey    string
	salt                string
	seed                ldvalue.OptionalInt
	contextValue        interface{} // i.e. the context key, or whatever other attribute we might be bucketing by
	secondaryKey        string
	expectedBucketValue float32
}

func (p bucketingTestParams) describe() string {
	return fmt.Sprintf("%+v", p)
}

func makeBucketingTestParams() []bucketingTestParams {
	return []bucketingTestParams{
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        "userKeyA",
			expectedBucketValue: 0.42157587,
		},
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        "userKeyB",
			expectedBucketValue: 0.6708485,
		},
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        "userKeyC",
			expectedBucketValue: 0.10343106,
		},
	}
}

func makeBucketingTestParamsWithNonStringValues() []bucketingTestParams {
	return []bucketingTestParams{
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        33333,
			expectedBucketValue: 0.54771423,
		},
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        99999,
			expectedBucketValue: 0.7309658,
		},
		// Non-integer numeric values, and any value types other than string and number, are not allowed
		// and cause the bucket value to be zero.
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        1.5,
			expectedBucketValue: 0,
		},
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        true,
			expectedBucketValue: 0,
		},
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        []interface{}{"x"},
			expectedBucketValue: 0,
		},
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        map[string]interface{}{"x": "y"},
			expectedBucketValue: 0,
		},
	}
}

func makeBucketingTestParamsForExperiments() []bucketingTestParams {
	ret := makeBucketingTestParams()
	ret = append(ret, []bucketingTestParams{
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        "userKeyA",
			expectedBucketValue: 0.42157587,
		},
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        "userKeyB",
			expectedBucketValue: 0.6708485,
		},
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        "userKeyC",
			expectedBucketValue: 0.10343106,
		},
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        "userKeyA",
			seed:                ldvalue.NewOptionalInt(61),
			expectedBucketValue: 0.09801207,
		},
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        "userKeyB",
			seed:                ldvalue.NewOptionalInt(61),
			expectedBucketValue: 0.14483777,
		},
		{
			flagOrSegmentKey:    "hashKey",
			salt:                "saltyA",
			contextValue:        "userKeyC",
			seed:                ldvalue.NewOptionalInt(61),
			expectedBucketValue: 0.9242641,
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

	makeRolloutVariationsToMatch := func(expectedBucketValue float32, desiredVariation int) []ldmodel.WeightedVariation {
		targetBucketSize := 10 // arbitrary small number (0.01% of 100000) to allow for a tiny bit of rounding error
		bucketWeightBefore := int(expectedBucketValue*100000) - (targetBucketSize / 2)
		bucketWeightAfter := 100000 - (bucketWeightBefore + targetBucketSize)
		return []ldmodel.WeightedVariation{
			{Variation: unwantedVar, Weight: bucketWeightBefore},
			{Variation: desiredVariation, Weight: targetBucketSize},
			{Variation: unwantedVar, Weight: bucketWeightAfter},
		}
	}

	// These tests check for consistent computation of bucket values for rollouts/experiments across SDKs.
	// They use hard-coded expected values that we have precomputed using a known good implementation of
	// the bucketing algorithm.
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
	doTests := func(
		t *ldtest.T,
		params []bucketingTestParams,
		rolloutKind ldmodel.RolloutKind,
		bucketBy lduser.UserAttribute,
		makeUser func(p bucketingTestParams, shouldMatchRule bool) lduser.User,
	) {
		flagForSegmentKeyPrefix := "matchsegment-"
		isExperiment := rolloutKind == ldmodel.RolloutKindExperiment

		for _, p := range params {
			dataBuilder := mockld.NewServerSDKDataBuilder()

			flagFallthroughRollout := ldmodel.Rollout{
				Kind:       rolloutKind,
				BucketBy:   bucketBy,
				Seed:       p.seed,
				Variations: makeRolloutVariationsToMatch(p.expectedBucketValue, expectedFallthroughVar),
			}
			flagRuleRollout := ldmodel.Rollout{
				Kind:       rolloutKind,
				BucketBy:   bucketBy,
				Seed:       p.seed,
				Variations: makeRolloutVariationsToMatch(p.expectedBucketValue, expectedRuleVar),
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
						Weight:   int(p.expectedBucketValue*100000) + 5,
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
	}

	t.Run("basic bucketing calculations", func(t *ldtest.T) {
		makeUser := func(p bucketingTestParams, shouldMatchRule bool) lduser.User {
			b := lduser.NewUserBuilder(ldvalue.CopyArbitraryValue(p.contextValue).StringValue())
			if p.secondaryKey != "" {
				b.Secondary(p.secondaryKey)
			}
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

	t.Run("bucket by non-key attribute (rollout only)", func(t *ldtest.T) {
		t.Run("string value", func(t *ldtest.T) {
			bucketBy := "attr1"
			makeUser := func(p bucketingTestParams, shouldMatchRule bool) lduser.User {
				value := ldvalue.CopyArbitraryValue(p.contextValue)
				b := lduser.NewUserBuilder("arbitrary-key")
				b.Custom(bucketBy, value)
				if p.secondaryKey != "" {
					b.Secondary(p.secondaryKey)
				}
				if shouldMatchRule {
					b.Custom(matchRuleAttr, ldvalue.Bool(true))
				}
				return b.Build()
			}
			doTests(t, makeBucketingTestParams(), ldmodel.RolloutKindRollout, lduser.UserAttribute(bucketBy), makeUser)
		})

		t.Run("non-string value", func(t *ldtest.T) {
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
			doTests(t, makeBucketingTestParamsWithNonStringValues(), ldmodel.RolloutKindRollout,
				lduser.UserAttribute(bucketBy), makeUser)
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
				p1.expectedBucketValue = 0
				params = append(params, p1)
			}
			doTests(t, params, ldmodel.RolloutKindRollout, lduser.UserAttribute(bucketBy), makeUser)
		})
	})
}
