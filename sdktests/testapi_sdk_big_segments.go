package sdktests

import (
	"errors"
	"sync"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BigSegmentStore is a test fixture that provides callback endpoints for SDK clients to connect to,
// behaving like a Big Segment store for a simulated database.
type BigSegmentStore struct {
	service           *mockld.MockBigSegmentStoreService
	endpoint          *harness.MockEndpoint
	getMetadata       func() (ldtime.UnixMillisecondTime, error)
	getMembership     func(string) (map[string]bool, error)
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
		b.doGetMembership,
		t.DebugLogger(),
	)
	b.endpoint = requireContext(t).harness.NewMockEndpoint(b.service, nil, t.DebugLogger())
	t.Defer(b.endpoint.Close)

	b.metadataQueries = make(chan struct{}, 20) // arbitrary capacity that's more than our tests care about

	b.SetupMetadataForStatus(initialStatus)

	return b
}

// ApplyConfiguration updates the SDK client configuration for NewSDKClient, causing the SDK
// to connect to the appropriate base URI for the big segments test fixture.
func (b *BigSegmentStore) ApplyConfiguration(config *servicedef.SDKConfigParams) {
	if config.BigSegments == nil {
		config.BigSegments = &servicedef.SDKConfigBigSegmentsParams{}
	} else {
		bc := *config.BigSegments
		config.BigSegments = &bc // copy to avoid side effects
	}
	config.BigSegments.CallbackURI = b.endpoint.BaseURL()
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

// SetupGetMembership causes the specified function to be called whenever the SDK calls the
// "get membership" method on the Big Segment store.
func (b *BigSegmentStore) SetupGetMembership(fn func(contextHash string) (map[string]bool, error)) {
	b.lock.Lock()
	b.getMembership = fn
	b.lock.Unlock()
}

// SetupMemberships is a shortcut to call SetupGetMembership with appropriate logic for
// providing preconfigured results for each possible context hash. Any context hash whose key does not
// appear in the map will cause the test to fail.
func (b *BigSegmentStore) SetupMemberships(t *ldtest.T, memberships map[string]map[string]bool) {
	b.SetupGetMembership(func(contextHash string) (map[string]bool, error) {
		if membership, ok := memberships[contextHash]; ok {
			return membership, nil
		}
		expectedKeys := make([]string, len(memberships))
		for k := range memberships {
			expectedKeys = append(expectedKeys, k)
		}
		assert.Fail(t, "got membership query with unexpected context hash value",
			"actual: %s, expected: %v", contextHash, sortedStrings(expectedKeys))
		return nil, nil
	})
}

// ExpectMetadataQuery blocks until the Big Segment store has received a metadata query.
func (b *BigSegmentStore) ExpectMetadataQuery(t *ldtest.T, timeout time.Duration) {
	select {
	case <-b.metadataQueries:
		return
	case <-time.After(timeout):
		require.Fail(t, "timed out waiting for big segments metadata query")
	}
}

// ExpectNoMoreMetadataQueries causes a test failure if the Big Segment store receives a
// metadata query.
func (b *BigSegmentStore) ExpectNoMoreMetadataQueries(t *ldtest.T, timeout time.Duration) {
	select {
	case <-b.metadataQueries:
		require.Fail(t, "got an unexpected big segments metadata query")
	case <-time.After(timeout):
	}
}

// GetMembershipQueries returns the context hashes of all membership queries that have been
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

func (b *BigSegmentStore) doGetMembership(contextHash string) (map[string]bool, error) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.membershipQueries = append(b.membershipQueries, contextHash)
	if b.getMembership != nil {
		return b.getMembership(contextHash)
	}
	return nil, nil
}
