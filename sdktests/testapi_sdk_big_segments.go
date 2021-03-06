package sdktests

import (
	"errors"
	"sync"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"

	"github.com/stretchr/testify/assert"
)

// BigSegmentStore is a test fixture that provides callback endpoints for SDK clients to connect to,
// behaving like a Big Segment store for a simulated database.
type BigSegmentStore struct {
	service           *mockld.MockBigSegmentStoreService
	endpoint          *harness.MockEndpoint
	getMetadata       func() (ldtime.UnixMillisecondTime, error)
	getUserMembership func(string) (map[string]bool, error)
	metadataQueries   chan struct{}
	membershipQueries []string
	lock              sync.Mutex
}

// NewBigSegmentStore creates a new BigSegmentStore with the specified initial status.
//
// The object's lifecycle is tied to the test scope that created it; it will be automatically closed
// when this test scope exits. It can be reused by subtests until then. Debug output related to the
// data source will be attached to this test scope.
func NewBigSegmentStore(t *ldtest.T, initialStatus ldreason.BigSegmentsStatus) *BigSegmentStore {
	b := &BigSegmentStore{}
	b.service = mockld.NewMockBigSegmentStoreService(
		b.doGetMetadata,
		b.doGetUserMembership,
		t.DebugLogger(),
	)
	b.endpoint = requireContext(t).harness.NewMockEndpoint(b.service, t.DebugLogger(),
		harness.MockEndpointDescription("big segment store fixture"))
	t.Defer(b.endpoint.Close)

	b.metadataQueries = make(chan struct{}, 20) // arbitrary capacity that's more than our tests care about

	b.SetupMetadataForStatus(initialStatus)

	return b
}

// Configure updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the big segments test fixture.
func (b *BigSegmentStore) Configure(config *servicedef.SDKConfigParams) error {
	newState := config.BigSegments.Value()
	newState.CallbackURI = b.endpoint.BaseURL()
	config.BigSegments = o.Some(newState)
	return nil
}

// SetupGetMetadata causes the specified function to be called whenever the SDK calls the "get
// metadata" method on the Big Segment store.
func (b *BigSegmentStore) SetupGetMetadata(fn func() (ldtime.UnixMillisecondTime, error)) {
	b.lock.Lock()
	b.getMetadata = fn
	b.lock.Unlock()
}

// SetupMetadataForStatus is a shortcut to call SetupGetMetadata with appropriate logic for
// making the Big Segment store return a current time ("healthy" status), an old time ("stale"
// status), or an error ("store error" status).
func (b *BigSegmentStore) SetupMetadataForStatus(status ldreason.BigSegmentsStatus) {
	b.SetupGetMetadata(func() (ldtime.UnixMillisecondTime, error) {
		switch status {
		case ldreason.BigSegmentsStoreError, ldreason.BigSegmentsNotConfigured:
			return 0, errors.New("THIS IS A DELIBERATE ERROR")
		case ldreason.BigSegmentsStale:
			return ldtime.UnixMillisecondTime(1), nil
		default:
			return ldtime.UnixMillisNow(), nil
		}
	})
}

// SetupGetUserMembership causes the specified function to be called whenever the SDK calls the
// "get user membership" method on the Big Segment store.
func (b *BigSegmentStore) SetupGetUserMembership(fn func(userHash string) (map[string]bool, error)) {
	b.lock.Lock()
	b.getUserMembership = fn
	b.lock.Unlock()
}

// SetupMemberships is a shortcut to call SetupGetUserMembership with appropriate logic for
// providing preconfigured results for each possible user hash. Any user hash whose key does not
// appear in the map will cause the test to fail.
func (b *BigSegmentStore) SetupMemberships(t *ldtest.T, memberships map[string]map[string]bool) {
	b.SetupGetUserMembership(func(userHash string) (map[string]bool, error) {
		if membership, ok := memberships[userHash]; ok {
			return membership, nil
		}
		expectedKeys := make([]string, len(memberships))
		for k := range memberships {
			expectedKeys = append(expectedKeys, k)
		}
		assert.Fail(t, "got membership query with unexpected user hash value",
			"actual: %s, expected: %v", userHash, sortedStrings(expectedKeys))
		return nil, nil
	})
}

// ExpectMetadataQuery blocks until the Big Segment store has received a metadata query.
func (b *BigSegmentStore) ExpectMetadataQuery(t *ldtest.T, timeout time.Duration) {
	_ = helpers.RequireValueWithMessage(t, b.metadataQueries, timeout,
		"timed out waiting for big segments metadata query")
}

// ExpectNoMoreMetadataQueries causes a test failure if the Big Segment store receives a
// metadata query.
func (b *BigSegmentStore) ExpectNoMoreMetadataQueries(t *ldtest.T, timeout time.Duration) {
	helpers.RequireNoMoreValues(t, b.metadataQueries, timeout)
}

// GetMembershipQueries returns the user hashes of all membership queries that have been
// received so far.
func (b *BigSegmentStore) GetMembershipQueries() []string {
	b.lock.Lock()
	defer b.lock.Unlock()
	return append([]string(nil), b.membershipQueries...)
}

func (b *BigSegmentStore) doGetMetadata() (ldtime.UnixMillisecondTime, error) {
	b.lock.Lock()
	defer b.lock.Unlock()
	select {
	case b.metadataQueries <- struct{}{}: // non-blocking send
		break
	default:
	}
	if b.getMetadata != nil {
		return b.getMetadata()
	}
	return 0, nil
}

func (b *BigSegmentStore) doGetUserMembership(userHash string) (map[string]bool, error) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.membershipQueries = append(b.membershipQueries, userHash)
	if b.getUserMembership != nil {
		return b.getUserMembership(userHash)
	}
	return nil, nil
}
