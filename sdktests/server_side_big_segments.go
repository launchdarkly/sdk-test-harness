package sdktests

import (
	"errors"
	"fmt"
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
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldmodel"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var bigSegmentsContext = ldcontext.New("user-key")                           //nolint:gochecknoglobals
var bigSegmentsExpectedHash = "CEXjZY7cHJG/ydFy7q4+YEFwVrG3/pkJwA4FAjrbfx0=" //nolint:gochecknoglobals

func doServerSideBigSegmentsTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityBigSegments)

	t.Run("evaluation", doBigSegmentsEvaluateSegment)
	t.Run("membership caching", doBigSegmentsMembershipCachingTests)
	t.Run("status polling", doBigSegmentsStatusPollingTests)
	t.Run("error handling", doBigSegmentsErrorHandlingTests)
}

func doBigSegmentsEvaluateSegment(t *ldtest.T) {
	otherContext := ldcontext.New("other-user-key")
	otherKind := ldcontext.Kind("other")

	basicSegment := ldbuilders.NewSegmentBuilder("segment1").Version(1).
		Included(otherContext.Key()). // for "regular included list is ignored for big segment" test
		Unbounded(true).Generation(100).Build()

	segmentWithRule := ldbuilders.NewSegmentBuilder("segment2").Version(1).
		Unbounded(true).Generation(100).
		AddRule(ldbuilders.NewSegmentRuleBuilder().Clauses(
			ldbuilders.Clause(ldattr.KeyAttr, ldmodel.OperatorIn, ldvalue.String(bigSegmentsContext.Key())),
		)).
		Build()

	segmentWithOtherKind := ldbuilders.NewSegmentBuilder("segment3").Version(1).
		Unbounded(true).UnboundedContextKind(otherKind).Generation(100).Build()

	basicFlag := makeFlagToCheckSegmentMatch("flagkey1", basicSegment.Key, ldvalue.Bool(false), ldvalue.Bool(true))
	flagForSegmentWithRule := makeFlagToCheckSegmentMatch(
		"flagkey2", segmentWithRule.Key, ldvalue.Bool(false), ldvalue.Bool(true))
	flagForSegmentWithOtherKind := makeFlagToCheckSegmentMatch(
		"flagkey3", segmentWithOtherKind.Key, ldvalue.Bool(false), ldvalue.Bool(true))
	data := mockld.NewServerSDKDataBuilder().
		Flag(basicFlag, flagForSegmentWithRule, flagForSegmentWithOtherKind).
		Segment(basicSegment, segmentWithRule, segmentWithOtherKind).
		Build()
	dataSource := NewSDKDataSource(t, data)

	for _, status := range []ldreason.BigSegmentsStatus{ldreason.BigSegmentsHealthy, ldreason.BigSegmentsStale} {
		t.Run(fmt.Sprintf("status %s", status), func(t *ldtest.T) {
			t.Run("context not found", func(t *ldtest.T) {
				bigSegmentStore := NewBigSegmentStore(t, status)
				client := NewSDKClient(t, dataSource, bigSegmentStore)

				result := evaluateFlagDetail(t, client, basicFlag.Key, bigSegmentsContext, ldvalue.Null())
				m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, status))

				assert.Equal(t, []string{bigSegmentsExpectedHash}, bigSegmentStore.GetMembershipQueries())
			})

			t.Run("context not included nor excluded (empty membership)", func(t *ldtest.T) {
				bigSegmentStore := NewBigSegmentStore(t, status)
				bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{bigSegmentsExpectedHash: {}})
				client := NewSDKClient(t, dataSource, bigSegmentStore)

				result := evaluateFlagDetail(t, client, basicFlag.Key, bigSegmentsContext, ldvalue.Null())
				m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, status))

				assert.Equal(t, []string{bigSegmentsExpectedHash}, bigSegmentStore.GetMembershipQueries())
			})

			t.Run("context not included nor excluded (null membership)", func(t *ldtest.T) {
				bigSegmentStore := NewBigSegmentStore(t, status)
				bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{bigSegmentsExpectedHash: nil})
				client := NewSDKClient(t, dataSource, bigSegmentStore)

				result := evaluateFlagDetail(t, client, basicFlag.Key, bigSegmentsContext, ldvalue.Null())
				m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, status))

				assert.Equal(t, []string{bigSegmentsExpectedHash}, bigSegmentStore.GetMembershipQueries())
			})

			t.Run("context not included nor excluded, matched by segment rule", func(
				t *ldtest.T) {
				bigSegmentStore := NewBigSegmentStore(t, status)
				bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{bigSegmentsExpectedHash: {}})
				client := NewSDKClient(t, dataSource, bigSegmentStore)

				result := evaluateFlagDetail(t, client, flagForSegmentWithRule.Key, bigSegmentsContext, ldvalue.Null())
				m.In(t).Assert(result, expectBigSegmentsResult(true, flagForSegmentWithRule, status))

				assert.Equal(t, []string{bigSegmentsExpectedHash}, bigSegmentStore.GetMembershipQueries())
			})

			t.Run("context included", func(t *ldtest.T) {
				bigSegmentStore := NewBigSegmentStore(t, status)
				bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
					bigSegmentsExpectedHash: {bigSegmentRef(basicSegment): true}})
				client := NewSDKClient(t, dataSource, bigSegmentStore)

				result := evaluateFlagDetail(t, client, basicFlag.Key, bigSegmentsContext, ldvalue.Null())
				m.In(t).Assert(result, expectBigSegmentsResult(true, basicFlag, status))

				assert.Equal(t, []string{bigSegmentsExpectedHash}, bigSegmentStore.GetMembershipQueries())
			})

			t.Run("context excluded", func(t *ldtest.T) {
				bigSegmentStore := NewBigSegmentStore(t, status)
				bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
					bigSegmentsExpectedHash: {bigSegmentRef(basicSegment): false}})
				client := NewSDKClient(t, dataSource, bigSegmentStore)

				result := evaluateFlagDetail(t, client, basicFlag.Key, bigSegmentsContext, ldvalue.Null())
				m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, status))

				assert.Equal(t, []string{bigSegmentsExpectedHash}, bigSegmentStore.GetMembershipQueries())
			})

			t.Run("context excluded, matched by segment rule", func(t *ldtest.T) {
				bigSegmentStore := NewBigSegmentStore(t, status)
				bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
					bigSegmentsExpectedHash: {bigSegmentRef(segmentWithRule): false}})
				client := NewSDKClient(t, dataSource, bigSegmentStore)

				result := evaluateFlagDetail(t, client, flagForSegmentWithRule.Key, bigSegmentsContext, ldvalue.Null())
				m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, status))

				assert.Equal(t, []string{bigSegmentsExpectedHash}, bigSegmentStore.GetMembershipQueries())
			})
		})
	}

	t.Run("regular include list is ignored for big segment", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		result := evaluateFlagDetail(t, client, basicFlag.Key, otherContext, ldvalue.Null())
		m.In(t).Assert(result, expectBigSegmentsResult(false, basicFlag, ldreason.BigSegmentsHealthy))
	})

	t.Run("no query is done if context kind does not match", func(t *ldtest.T) {
		// We deliberately configured flagForSegmentWithOtherKind so that the clause referencing the segment
		// has the default user kind, and so does the context we're evaluating-- so it will check the segment,
		// but then it will see that the segment's UnboundedContextKind is wrong, so it should not match
		// even though the key is in the big segment data.

		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			bigSegmentsExpectedHash: {bigSegmentRef(segmentWithOtherKind): true}})
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		result := evaluateFlagDetail(t, client, flagForSegmentWithOtherKind.Key, bigSegmentsContext, ldvalue.Null())
		m.In(t).Assert(result, m.AllOf(
			EvalResponseValue().Should(m.JSONEqual(false)),
			EvalResponseReason().Should(EqualReason(ldreason.NewEvalReasonFallthrough())),
		))

		assert.Len(t, bigSegmentStore.GetMembershipQueries(), 0)
	})
}

func doBigSegmentsMembershipCachingTests(t *ldtest.T) {
	user1, user2, user3 := ldcontext.New("user1"), ldcontext.New("user2"), ldcontext.New("user3")
	otherKind := ldcontext.Kind("other")
	expectedUserHash1, expectedUserHash2, expectedUserHash3 := "CgQblGLKpKMbrDVn4Lbm/ZEAeH2yq0M9lvbReMq/zpA=",
		"YCXRj+SKvUUWhSjxioLiZd2Y1CGnCEqgn2GzQXA5AaM=", "WGD68CtrxiIrpaylI1YPDjZMzYtnvuSG/ov3wB1JLMs="

	segment1 := ldbuilders.NewSegmentBuilder("segment1").Version(1).
		Unbounded(true).Generation(100).Build()
	segment2 := ldbuilders.NewSegmentBuilder("segment2").Version(1).
		Unbounded(true).Generation(101).Build()
	segment3 := ldbuilders.NewSegmentBuilder("segment3").Version(1).
		Unbounded(true).UnboundedContextKind(otherKind).Generation(102).Build()
	flag := ldbuilders.NewFlagBuilder("flag-key").Version(1).
		On(true).FallthroughVariation(1).Variations(ldvalue.Bool(true), ldvalue.Bool(false)).
		AddRule(
			ldbuilders.NewRuleBuilder().ID("rule1").Variation(0).Clauses(
				ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String(segment1.Key)),
			)).
		AddRule(
			ldbuilders.NewRuleBuilder().ID("rule2").Variation(0).Clauses(
				ldbuilders.Clause("", ldmodel.OperatorSegmentMatch, ldvalue.String(segment2.Key)),
			)).
		AddRule(
			ldbuilders.NewRuleBuilder().ID("rule3").Variation(0).Clauses(
				ldbuilders.ClauseWithKind(otherKind, "", ldmodel.OperatorSegmentMatch, ldvalue.String(segment3.Key)),
			)).
		Build()
	data := mockld.NewServerSDKDataBuilder().Flag(flag).Segment(segment1, segment2, segment3).Build()
	dataSource := NewSDKDataSource(t, data)

	t.Run("membership query is cached for multiple tests in one evaluation", func(t *ldtest.T) {
		// Set up membership so the context is included in segment2, and not included in segment1.
		// Due to the order of the flag rules, the SDK will check segment1 first, find no match,
		// and then check segment2. We should only see one membership query for the context.

		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {bigSegmentRef(segment1): false, bigSegmentRef(segment2): true}})
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		value := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1}, bigSegmentStore.GetMembershipQueries())
	})

	t.Run("queries may be done for multiple context kinds", func(t *ldtest.T) {
		// Here we provide a multi-kind context where there is at least one rule checking each kind,
		// with a different key for each kind.

		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {bigSegmentRef(segment1): false, bigSegmentRef(segment2): false},
			expectedUserHash2: {bigSegmentRef(segment3): true}})
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		context := ldcontext.NewMulti(
			user1,
			ldcontext.NewWithKind(otherKind, user2.Key()),
		)
		value := basicEvaluateFlag(t, client, flag.Key, context, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1, expectedUserHash2}, bigSegmentStore.GetMembershipQueries())
	})

	t.Run("query is by key regardless of context kind", func(t *ldtest.T) {
		// Here we provide a multi-kind context where the key in each kind is the same.

		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {bigSegmentRef(segment1): false, bigSegmentRef(segment2): false, bigSegmentRef(segment3): true}})
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		context := ldcontext.NewMulti(
			user1,
			ldcontext.NewWithKind(otherKind, user1.Key()),
		)
		value := basicEvaluateFlag(t, client, flag.Key, context, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1}, bigSegmentStore.GetMembershipQueries())
	})

	t.Run("membership query is cached across evaluations for same context", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {bigSegmentRef(segment1): true}})

		value := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {bigSegmentRef(segment1): false}}) // the SDK will not query this value

		value = basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1}, bigSegmentStore.GetMembershipQueries())
	})

	t.Run("membership query is cached separately per context", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {bigSegmentRef(segment1): true},
			expectedUserHash2: {bigSegmentRef(segment1): true}})

		// evaluate for user1
		value := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1}, bigSegmentStore.GetMembershipQueries())

		// modify the stored data for user1
		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {bigSegmentRef(segment1): false},
			expectedUserHash2: {bigSegmentRef(segment1): false}})

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

	t.Run("context cache expiration (cache time)", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
			BigSegments: o.Some(servicedef.SDKConfigBigSegmentsParams{
				UserCacheTimeMS: o.Some(ldtime.UnixMillisecondTime(10)),
			}),
		}), dataSource, bigSegmentStore)

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {bigSegmentRef(segment1): true}})

		value := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
		m.In(t).Assert(value, m.JSONEqual(true))

		assert.Equal(t, []string{expectedUserHash1}, bigSegmentStore.GetMembershipQueries())

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {bigSegmentRef(segment1): false}})

		h.AssertEventually(
			t,
			func() bool {
				value := basicEvaluateFlag(t, client, flag.Key, user1, ldvalue.Null())
				return value.Equal(ldvalue.Bool(false))
			},
			time.Second,
			time.Millisecond*20,
			"timed out waiting for context membership to be re-queried",
		)

		assert.Equal(t, []string{expectedUserHash1, expectedUserHash1},
			bigSegmentStore.GetMembershipQueries())
	})

	t.Run("context cache eviction (cache size)", func(t *ldtest.T) {
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, WithConfig(servicedef.SDKConfigParams{
			BigSegments: o.Some(servicedef.SDKConfigBigSegmentsParams{
				UserCacheSize: o.Some(2),
			}),
		}), dataSource, bigSegmentStore)

		bigSegmentStore.SetupMemberships(t, map[string]map[string]bool{
			expectedUserHash1: {bigSegmentRef(segment1): true},
			expectedUserHash2: {bigSegmentRef(segment2): true},
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
			h.AssertEventually(
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
			Included(bigSegmentsContext.Key()). // regular include list should be ignored if unbounded=true
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
			basicEvaluateFlag(t, client, flag.Key, ldcontext.New(fmt.Sprintf("user-%d", i)), ldvalue.Null())
		}

		bigSegmentStore.ExpectNoMoreMetadataQueries(t, time.Millisecond*50)
	})
}

func doBigSegmentsErrorHandlingTests(t *ldtest.T) {
	t.Run("big segment store was not configured", func(t *ldtest.T) {
		segment := ldbuilders.NewSegmentBuilder("segment-key").Version(1).
			Included(bigSegmentsContext.Key()). // regular include list should be ignored if unbounded=true
			Unbounded(true).Generation(100).Build()
		flag := makeFlagToCheckSegmentMatch("flag-key", segment.Key, ldvalue.Bool(false), ldvalue.Bool(true))
		data := mockld.NewServerSDKDataBuilder().Flag(flag).Segment(segment).Build()

		dataSource := NewSDKDataSource(t, data)
		client := NewSDKClient(t, dataSource)

		result := evaluateFlagDetail(t, client, flag.Key, bigSegmentsContext, ldvalue.Null())
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
			bigSegmentsExpectedHash: {segmentRef0: true}})

		result := evaluateFlagDetail(t, client, flag.Key, bigSegmentsContext, ldvalue.Null())
		m.In(t).Assert(result.Value, m.JSONEqual(false))
		m.In(t).Assert(result.Reason, m.JSONEqual(
			ldreason.NewEvalReasonFromReasonWithBigSegmentsStatus(ldreason.NewEvalReasonFallthrough(),
				ldreason.BigSegmentsNotConfigured)))
		assert.Len(t, bigSegmentStore.GetMembershipQueries(), 0)
	})

	t.Run("store error on context membership query", func(t *ldtest.T) {
		segment := ldbuilders.NewSegmentBuilder("segment-key").Version(1).
			Unbounded(true).Generation(100).Build()
		flag := makeFlagToCheckSegmentMatch("flag-key", segment.Key, ldvalue.Bool(false), ldvalue.Bool(true))
		data := mockld.NewServerSDKDataBuilder().Flag(flag).Segment(segment).Build()

		dataSource := NewSDKDataSource(t, data)
		bigSegmentStore := NewBigSegmentStore(t, ldreason.BigSegmentsHealthy)
		client := NewSDKClient(t, dataSource, bigSegmentStore)

		bigSegmentStore.SetupGetMembership(func(string) (map[string]bool, error) {
			return nil, errors.New("THIS IS A DELIBERATE ERROR")
		})

		result := evaluateFlagDetail(t, client, flag.Key, bigSegmentsContext, ldvalue.Null())
		m.In(t).Assert(result.Value, m.JSONEqual(false))
		m.In(t).Assert(result.Reason, m.JSONEqual(
			ldreason.NewEvalReasonFromReasonWithBigSegmentsStatus(ldreason.NewEvalReasonFallthrough(),
				ldreason.BigSegmentsStoreError)))

		assert.Equal(t, []string{bigSegmentsExpectedHash}, bigSegmentStore.GetMembershipQueries())
	})
}

func expectBigSegmentsResult(isMatch bool, flag ldmodel.FeatureFlag, status ldreason.BigSegmentsStatus) m.Matcher {
	baseReason := ldreason.NewEvalReasonFallthrough()
	if isMatch {
		baseReason = ldreason.NewEvalReasonRuleMatch(0, flag.Rules[0].ID)
	}
	return m.AllOf(
		EvalResponseValue().Should(m.JSONEqual(isMatch)),
		EvalResponseReason().Should(EqualReason(
			ldreason.NewEvalReasonFromReasonWithBigSegmentsStatus(baseReason, status))),
	)
}

func bigSegmentRef(segment ldmodel.Segment) string {
	return fmt.Sprintf("%s.g%d", segment.Key, segment.Generation.IntValue())
}
