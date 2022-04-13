package sdktests

import (
	"errors"
	"fmt"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var bigSegmentsUser = lduser.NewUser("user-key")                                 //nolint:gochecknoglobals
var bigSegmentsExpectedUserHash = "CEXjZY7cHJG/ydFy7q4+YEFwVrG3/pkJwA4FAjrbfx0=" //nolint:gochecknoglobals

func doBigSegmentsEvaluateSegment(t *ldtest.T) {
	otherUser := lduser.NewUser("other-user-key")

	basicSegment := ldbuilders.NewSegmentBuilder("segment1").Version(1).
		Included(otherUser.GetKey()). // for "regular included list is ignored for big segment" test
		Unbounded(true).Generation(100).Build()
	basicSegmentRef := fmt.Sprintf("%s.g%d", basicSegment.Key, basicSegment.Generation.IntValue())

	segmentWithRule := ldbuilders.NewSegmentBuilder("segment2").Version(1).
		Unbounded(true).Generation(100).
		AddRule(ldbuilders.NewSegmentRuleBuilder().Clauses(
			ldbuilders.Clause(lduser.KeyAttribute, ldmodel.OperatorIn, ldvalue.String(bigSegmentsUser.GetKey())),
		)).
		Build()
	segmentWithRuleRef := fmt.Sprintf("%s.g%d", segmentWithRule.Key, segmentWithRule.Generation.IntValue())

	basicFlag := makeFlagToCheckSegmentMatch("flagkey1", basicSegment.Key, ldvalue.Bool(false), ldvalue.Bool(true))
	flagForSegmentWithRule := makeFlagToCheckSegmentMatch(
		"flagkey2", segmentWithRule.Key, ldvalue.Bool(false), ldvalue.Bool(true))
	data := mockld.NewServerSDKDataBuilder().
		Flag(basicFlag, flagForSegmentWithRule).
		Segment(basicSegment, segmentWithRule).
		Build()
	dataSource := NewSDKDataSource(t, data)

	for _, status := range []ldreason.BigSegmentsStatus{ldreason.BigSegmentsHealthy, ldreason.BigSegmentsStale} {
		t.Run(fmt.Sprintf("user not found, status %s", status), func(t *ldtest.T) {
			bigSegmentStore := NewBigSegmentStore(t, status)
			client := NewSDKClient(t, dataSource, bigSegmentStore)

			result := evaluateFlagDetail(t, client, basicFlag.Key, bigSegmentsUser, ldvalue.Null())
			m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, status))

			assert.Equal(t, []string{bigSegmentsExpectedUserHash}, bigSegmentStore.GetMembershipQueries())
		})

		t.Run(fmt.Sprintf("user not included nor excluded (empty membership), status %s", status), func(t *ldtest.T) {
			bigSegmentStore := NewBigSegmentStore(t, status)
			bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{bigSegmentsExpectedUserHash: {}})
			client := NewSDKClient(t, dataSource, bigSegmentStore)

			result := evaluateFlagDetail(t, client, basicFlag.Key, bigSegmentsUser, ldvalue.Null())
			m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, status))

			assert.Equal(t, []string{bigSegmentsExpectedUserHash}, bigSegmentStore.GetMembershipQueries())
		})

		t.Run(fmt.Sprintf("user not included nor excluded (null membership), status %s", status), func(t *ldtest.T) {
			bigSegmentStore := NewBigSegmentStore(t, status)
			bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{bigSegmentsExpectedUserHash: nil})
			client := NewSDKClient(t, dataSource, bigSegmentStore)

			result := evaluateFlagDetail(t, client, basicFlag.Key, bigSegmentsUser, ldvalue.Null())
			m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, status))

			assert.Equal(t, []string{bigSegmentsExpectedUserHash}, bigSegmentStore.GetMembershipQueries())
		})

		t.Run(fmt.Sprintf("user not included nor excluded, matched by segment rule, status %s", status), func(t *ldtest.T) {
			bigSegmentStore := NewBigSegmentStore(t, status)
			bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{bigSegmentsExpectedUserHash: {}})
			client := NewSDKClient(t, dataSource, bigSegmentStore)

			result := evaluateFlagDetail(t, client, flagForSegmentWithRule.Key, bigSegmentsUser, ldvalue.Null())
			m.In(t).Assert(result, expectBigSegmentsResult(true, flagForSegmentWithRule, status))

			assert.Equal(t, []string{bigSegmentsExpectedUserHash}, bigSegmentStore.GetMembershipQueries())
		})

		t.Run(fmt.Sprintf("user included, status is %s", status), func(t *ldtest.T) {
			bigSegmentStore := NewBigSegmentStore(t, status)
			bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
				bigSegmentsExpectedUserHash: {basicSegmentRef: true}})
			client := NewSDKClient(t, dataSource, bigSegmentStore)

			result := evaluateFlagDetail(t, client, basicFlag.Key, bigSegmentsUser, ldvalue.Null())
			m.In(t).Assert(result, expectBigSegmentsResult(true, basicFlag, status))

			assert.Equal(t, []string{bigSegmentsExpectedUserHash}, bigSegmentStore.GetMembershipQueries())
		})

		t.Run(fmt.Sprintf("user excluded, no rules, status is %s", status), func(t *ldtest.T) {
			bigSegmentStore := NewBigSegmentStore(t, status)
			bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
				bigSegmentsExpectedUserHash: {basicSegmentRef: false}})
			client := NewSDKClient(t, dataSource, bigSegmentStore)

			result := evaluateFlagDetail(t, client, basicFlag.Key, bigSegmentsUser, ldvalue.Null())
			m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, status))

			assert.Equal(t, []string{bigSegmentsExpectedUserHash}, bigSegmentStore.GetMembershipQueries())
		})

		t.Run(fmt.Sprintf("user excluded, matched by segment rule, status is %s", status), func(t *ldtest.T) {
			bigSegmentStore := NewBigSegmentStore(t, status)
			bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
				bigSegmentsExpectedUserHash: {segmentWithRuleRef: false}})
			client := NewSDKClient(t, dataSource, bigSegmentStore)

			result := evaluateFlagDetail(t, client, flagForSegmentWithRule.Key, bigSegmentsUser, ldvalue.Null())
			m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, status))

			assert.Equal(t, []string{bigSegmentsExpectedUserHash}, bigSegmentStore.GetMembershipQueries())
		})
	}

	t.Run("regular include list is ignored for big segment", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		result := evaluateFlagDetail(t, client, basicFlag.Key, otherUser, ldvalue.Null())
		m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, ldreason.BigSegmentsHealthy))
	})
}

func doBigSegmentsMembershipCachingTests(t *ldtest.T) {
	user1, user2, user3 := lduser.NewUser("user1"), lduser.NewUser("user2"), lduser.NewUser("user3")
	expectedUserHash1, expectedUserHash2, expectedUserHash3 := "CgQblGLKpKMbrDVn4Lbm/ZEAeH2yq0M9lvbReMq/zpA=",
		"YCXRj+SKvUUWhSjxioLiZd2Y1CGnCEqgn2GzQXA5AaM=", "WGD68CtrxiIrpaylI1YPDjZMzYtnvuSG/ov3wB1JLMs="

	segment1 := ldbuilders.NewSegmentBuilder("segment1").Version(1).
		Unbounded(true).Generation(100).Build()
	segmentRef1 := fmt.Sprintf("%s.g%d", segment1.Key, segment1.Generation.IntValue())
	segment2 := ldbuilders.NewSegmentBuilder("segment2").Version(1).
		Unbounded(true).Generation(101).Build()
	segmentRef2 := fmt.Sprintf("%s.g%d", segment2.Key, segment2.Generation.IntValue())
	flag := ldbuilders.NewFlagBuilder("flag-key").Version(1).
		On(true).FallthroughVariation(1).Variations(ldvalue.Bool(true), ldvalue.Bool(false)).
		AddRule(
			ldbuilders.NewRuleBuilder().ID("rule1").Variation(0).Clauses(
				ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String(segment1.Key)),
			)).
		AddRule(
			ldbuilders.NewRuleBuilder().ID("rule2").Variation(0).Clauses(
				ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String(segment2.Key)),
			),
		).
		Build()
	data := mockld.NewServerSDKDataBuilder().Flag(flag).Segment(segment1, segment2).Build()
	dataSource := NewSDKDataSource(t, data)

	t.Run("membership query is cached for multiple tests in one evaluation", func(t *ldtest.T) {
		// Set up membership so the user is included in segment2, and not included in segment1.
		// Due to the order of the flag rules, the SDK will check segment1 first, find no match,
		// and then check segment2. We should only see one membership query for the user.

		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {segmentRef1: false, segmentRef2: true}})
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		value := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1}, bigSegmentStore.GetMembershipQueries())
	})

	t.Run("membership query is cached across evaluations for same user", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {segmentRef1: true}})

		value := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {segmentRef1: false}}) // the SDK will not query this value

		value = basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1}, bigSegmentStore.GetMembershipQueries())
	})

	t.Run("membership query is cached separately per user", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {segmentRef1: true},
			expectedUserHash2: {segmentRef1: true}})

		// evaluate for user1
		value := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1}, bigSegmentStore.GetMembershipQueries())

		// modify the stored data for user1
		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {segmentRef1: false},
			expectedUserHash2: {segmentRef1: false}})

		// re-evaluate for user1 - should use the cached state, not the value from the store
		value = basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1},
			bigSegmentStore.GetMembershipQueries()) // didn't do a 2nd query

		// now evaluate for user2 - its state is not yet cached, so it does a query
		value = basicEvaluateFlag(t, client, flag.Key, user2, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(false))

		assert.Equal(t, []string{expectedUserHash1, expectedUserHash2},
			bigSegmentStore.GetMembershipQueries())

		// re-evaluate for user1 - should still use the cache
		value = basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1, expectedUserHash2},
			bigSegmentStore.GetMembershipQueries())
	})

	t.Run("user cache expiration (cache time)", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
			BigSegments: o.Some(servicedef.SDKConfigBigSegmentsParams{
				UserCacheTimeMS: o.Some(ldtime.UnixMillisecondTime(10)),
			}),
		}), dataSource, bigSegmentStore)

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {segmentRef1: true}})

		value := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1}, bigSegmentStore.GetMembershipQueries())

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {segmentRef1: false}})

		assert.Eventually(
			t,
			func() bool {
				value := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
				return value.Equal(ldvalue.Bool(false))
			},
			time.Second,
			time.Millisecond*20,
			"timed out waiting for user membership to be re-queried",
		)

		assert.Equal(t, []string{expectedUserHash1, expectedUserHash1},
			bigSegmentStore.GetMembershipQueries())
	})

	t.Run("user cache eviction (cache size)", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
			BigSegments: o.Some(servicedef.SDKConfigBigSegmentsParams{
				UserCacheSize: o.Some(2),
			}),
		}), dataSource, bigSegmentStore)

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {segmentRef1: true},
			expectedUserHash2: {segmentRef2: true},
			expectedUserHash3: nil})

		value1a := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value1a, m.JSONEqual(true))
		value2a := basicEvaluateFlag(t, client, flag.Key, user2, ldvalue.Null())
		m.In(t).Assert(value2a, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1, expectedUserHash2},
			bigSegmentStore.GetMembershipQueries())

		value2b := basicEvaluateFlag(t, client, flag.Key, user2, ldvalue.Null())
		m.In(t).Assert(value2b, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1, expectedUserHash2},
			bigSegmentStore.GetMembershipQueries())

		value3 := basicEvaluateFlag(t, client, flag.Key, user3, ldvalue.Null())
		m.In(t).Assert(value3, m.JSONEqual(false))

		assert.Equal(t, []string{expectedUserHash1, expectedUserHash2, expectedUserHash3},
			bigSegmentStore.GetMembershipQueries())

		value1b := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value1b, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1, expectedUserHash2, expectedUserHash3, expectedUserHash1},
			bigSegmentStore.GetMembershipQueries())
	})
}

func doBigSegmentsStatusPollingTests(t *ldtest.T) {
	dataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())

	t.Run("polling can be set to a short interval", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)

		_ = NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
			BigSegments: o.Some(servicedef.SDKConfigBigSegmentsParams{
				StatusPollIntervalMS: o.Some(ldtime.UnixMillisecondTime(10)),
			}),
		}), dataSource, bigSegmentStore)

		for i := 0; i < 3; i++ {
			// Using a long timeout here just so we're not sensitive to random fluctuations in host speed.
			// We don't really care if it's greater than the configured interval, as long as it's nowhere
			// near the default interval of 5 seconds.
			bigSegmentStore.ExpectMetadataQuery(t, time.Millisecond*500)
		}
	})

	t.Run("polling can be set to a long interval", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)

		client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
			BigSegments: o.Some(servicedef.SDKConfigBigSegmentsParams{
				StatusPollIntervalMS: o.Some(ldtime.UnixMillisecondTime(10000)),
			}),
		}), dataSource, bigSegmentStore)

		initialStatus := client.GetBigSegmentStoreStatus(t)
		assert.Equal(t, servicedef.BigSegmentStoreStatusResponse{Available: true, Stale: false}, initialStatus)

		bigSegmentStore.ExpectMetadataQuery(t, time.Millisecond*500)
		bigSegmentStore.ExpectNoMoreMetadataQueries(t, time.Millisecond*200)
	})

	doStatusChangeTest := func(oldStatus, newStatus ldreason.BigSegmentsStatus) {
		t.Run(fmt.Sprintf("polling detects change from %s to %s", oldStatus, newStatus), func(t *ldtest.T) {
			bigSegmentStore := NewBigSegmentStore(t, oldStatus)

			client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
				BigSegments: o.Some(servicedef.SDKConfigBigSegmentsParams{
					StatusPollIntervalMS: o.Some(ldtime.UnixMillisecondTime(10)),
				}),
			}), dataSource, bigSegmentStore)

			initialStatus := client.GetBigSegmentStoreStatus(t)
			initialExpected := servicedef.BigSegmentStoreStatusResponse{
				Available: oldStatus != ldreason.BigSegmentsStoreError,
				Stale:     oldStatus == ldreason.BigSegmentsStale,
			}
			require.Equal(t, initialExpected, initialStatus)

			bigSegmentStore.SetupMetadataForStatus(newStatus)

			newExpected := servicedef.BigSegmentStoreStatusResponse{
				Available: newStatus != ldreason.BigSegmentsStoreError,
				Stale:     newStatus == ldreason.BigSegmentsStale,
			}
			assert.Eventually(
				t,
				func() bool {
					status := client.GetBigSegmentStoreStatus(t)
					return status == newExpected
				},
				time.Second,
				time.Millisecond*20,
				"timed out waiting for status to change",
			)
		})
	}
	doStatusChangeTest(ldreason.BigSegmentsHealthy, ldreason.BigSegmentsStoreError)
	doStatusChangeTest(ldreason.BigSegmentsStoreError, ldreason.BigSegmentsHealthy)
	doStatusChangeTest(ldreason.BigSegmentsHealthy, ldreason.BigSegmentsStale)
	doStatusChangeTest(ldreason.BigSegmentsStale, ldreason.BigSegmentsHealthy)

	t.Run("multiple evaluations don't cause another status poll before next interval", func(t *ldtest.T) {
		segment := ldbuilders.NewSegmentBuilder("segment-key").Version(1).
			Included(bigSegmentsUser.GetKey()). // regular include list should be ignored if unbounded=true
			Unbounded(true).Generation(100).Build()
		flag := makeFlagToCheckSegmentMatch("flag-key", segment.Key, ldvalue.Bool(false), ldvalue.Bool(true))
		data := mockld.NewServerSDKDataBuilder().Flag(flag).Segment(segment).Build()

		dataSource := NewSDKDataSource(t, data)
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)

		client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
			BigSegments: o.Some(servicedef.SDKConfigBigSegmentsParams{
				StatusPollIntervalMS: o.Some(ldtime.UnixMillisecondTime(10000)),
			}),
		}), dataSource, bigSegmentStore)

		initialStatus := client.GetBigSegmentStoreStatus(t)
		require.Equal(t, servicedef.BigSegmentStoreStatusResponse{Available: true, Stale: false}, initialStatus)
		bigSegmentStore.ExpectMetadataQuery(t, time.Millisecond*500)

		for i := 0; i < 10; i++ {
			basicEvaluateFlag(t, client, flag.Key, lduser.NewUser(fmt.Sprintf("user-%d", i)), ldvalue.Null())
		}

		bigSegmentStore.ExpectNoMoreMetadataQueries(t, time.Millisecond*50)
	})
}

func doBigSegmentsErrorHandlingTests(t *ldtest.T) {
	t.Run("big segment store was not configured", func(t *ldtest.T) {
		segment := ldbuilders.NewSegmentBuilder("segment-key").Version(1).
			Included(bigSegmentsUser.GetKey()). // regular include list should be ignored if unbounded=true
			Unbounded(true).Generation(100).Build()
		flag := makeFlagToCheckSegmentMatch("flag-key", segment.Key, ldvalue.Bool(false), ldvalue.Bool(true))
		data := mockld.NewServerSDKDataBuilder().Flag(flag).Segment(segment).Build()

		dataSource := NewSDKDataSource(t, data)
		client := NewSDKClient(t, dataSource)

		result := evaluateFlagDetail(t, client, flag.Key, bigSegmentsUser, ldvalue.Null())
		m.In(t).Assert(result.Value, m.JSONEqual(false))
		m.In(t).Assert(result.Reason, m.JSONEqual(
			ldreason.NewEvalReasonFromReasonWithBigSegmentsStatus(ldreason.NewEvalReasonFallthrough(),
				ldreason.BigSegmentsNotConfigured)))
	})

	t.Run("big segment with no generation is invalid", func(t *ldtest.T) {
		// This is an unexpected data inconsistency condition, so even though the problem might
		// not be in the configuration or the big segment store itself, we have to assume none
		// of the big segments results are really valid. Therefore the status should be reported
		// as NOT_CONFIGURED.

		segment := ldbuilders.NewSegmentBuilder("segment-key").Version(1).
			Unbounded(true).Build()
		segmentRef0 := fmt.Sprintf("%s.g0", segment.Key)
		flag := makeFlagToCheckSegmentMatch("flag-key", segment.Key, ldvalue.Bool(false), ldvalue.Bool(true))
		data := mockld.NewServerSDKDataBuilder().Flag(flag).Segment(segment).Build()

		dataSource := NewSDKDataSource(t, data)
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			bigSegmentsExpectedUserHash: {segmentRef0: true}})

		result := evaluateFlagDetail(t, client, flag.Key, bigSegmentsUser, ldvalue.Null())
		m.In(t).Assert(result.Value, m.JSONEqual(false))
		m.In(t).Assert(result.Reason, m.JSONEqual(
			ldreason.NewEvalReasonFromReasonWithBigSegmentsStatus(ldreason.NewEvalReasonFallthrough(),
				ldreason.BigSegmentsNotConfigured)))
		assert.Len(t, bigSegmentStore.GetMembershipQueries(), 0)
	})

	t.Run("store error on user membership query", func(t *ldtest.T) {
		segment := ldbuilders.NewSegmentBuilder("segment-key").Version(1).
			Unbounded(true).Generation(100).Build()
		flag := makeFlagToCheckSegmentMatch("flag-key", segment.Key, ldvalue.Bool(false), ldvalue.Bool(true))
		data := mockld.NewServerSDKDataBuilder().Flag(flag).Segment(segment).Build()

		dataSource := NewSDKDataSource(t, data)
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		bigSegmentStore.SetupGetUserMembership(func(string) (map[string]bool, error) {
			return nil, errors.New("THIS IS A DELIBERATE ERROR")
		})

		result := evaluateFlagDetail(t, client, flag.Key, bigSegmentsUser, ldvalue.Null())
		m.In(t).Assert(result.Value, m.JSONEqual(false))
		m.In(t).Assert(result.Reason, m.JSONEqual(
			ldreason.NewEvalReasonFromReasonWithBigSegmentsStatus(ldreason.NewEvalReasonFallthrough(),
				ldreason.BigSegmentsStoreError)))

		assert.Equal(t, []string{bigSegmentsExpectedUserHash}, bigSegmentStore.GetMembershipQueries())
	})
}

func expectBigSegmentsResult(isMatch bool, flag ldmodel.FeatureFlag, status ldreason.BigSegmentsStatus) m.Matcher {
	baseReason := ldreason.NewEvalReasonFallthrough()
	if isMatch {
		baseReason = ldreason.NewEvalReasonRuleMatch(0, flag.Rules[0].ID)
	}
	return m.AllOf(
		EvalResponseValue().Should(m.JSONEqual(ldvalue.Bool(isMatch))),
		EvalResponseReason().Should(m.JSONEqual(
			ldreason.NewEvalReasonFromReasonWithBigSegmentsStatus(baseReason, status))),
	)
}
