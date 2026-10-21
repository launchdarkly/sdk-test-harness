package sdktests

import (
	"sort"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/sdk-test-harness/v3/framework"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/stretchr/testify/assert"
)

const listenerReceiveTimeout = time.Second * 5
const listenerNoNotificationTimeout = time.Millisecond * 500

// ListenerCallback is used in flag change listener tests to receive and assert on listener
// notifications from the SDK test service. Each instance manages a dedicated mock HTTP
// endpoint that the SDK test service POSTs to when the registered listener fires.
//
// The general usage pattern is:
//
//	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
//	defer callback.Close()
//	client.RegisterFlagChangeListener(t, listenerID, flagKey, callback.GetURL())
//	// ... trigger a flag change via the streaming service ...
//	callback.ExpectFlagChangeNotification(t, flagKey)
type ListenerCallback struct {
	service *mockld.ListenerCallbackService
}

// NewListenerCallback creates a ListenerCallback with a dedicated mock HTTP endpoint ready to
// receive notifications. Call Close() when done to release the endpoint.
func NewListenerCallback(
	testHarness *harness.TestHarness,
	logger framework.Logger,
) *ListenerCallback {
	return &ListenerCallback{
		service: mockld.NewListenerCallbackService(testHarness, logger),
	}
}

// GetURL returns the callback URI to pass as the callbackUri field in a
// registerFlagChangeListener or registerFlagValueChangeListener command.
func (lc *ListenerCallback) GetURL() string {
	return lc.service.GetURL()
}

// Close shuts down the mock HTTP endpoint.
func (lc *ListenerCallback) Close() {
	lc.service.Close()
}

// registerFlagChangeListener registers a general flag change listener with the SDK client and
// returns the callback that receives its notifications. The callback is closed when the test
// scope exits.
func registerFlagChangeListener(t *ldtest.T, client *SDKClient) *ListenerCallback {
	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	t.Defer(callback.Close)
	client.RegisterFlagChangeListener(t, servicedef.RegisterFlagChangeListenerParams{
		ListenerID:  "listener-1",
		CallbackURI: callback.GetURL(),
	})
	return callback
}

// registerFlagValueChangeListener registers a flag value change listener for one flag and context
// with the SDK client and returns the callback that receives its notifications. The callback is
// closed when the test scope exits.
func registerFlagValueChangeListener(
	t *ldtest.T,
	client *SDKClient,
	flagKey string,
	context ldcontext.Context,
	defaultValue ldvalue.Value,
) *ListenerCallback {
	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	t.Defer(callback.Close)
	client.RegisterFlagValueChangeListener(t, servicedef.RegisterFlagValueChangeListenerParams{
		ListenerID:   "listener-1",
		FlagKey:      flagKey,
		Context:      context,
		DefaultValue: defaultValue,
		CallbackURI:  callback.GetURL(),
	})
	return callback
}

// ExpectFlagChangeNotification waits up to listenerReceiveTimeout for a general flag change
// notification for the given flag key. It fails the test if no notification arrives within the
// timeout, or if the notification's flag key does not match. Returns the notification for
// further inspection.
func (lc *ListenerCallback) ExpectFlagChangeNotification(
	t *ldtest.T,
	flagKey string,
) servicedef.ListenerNotification {
	notification := helpers.RequireValueWithMessage(
		t, lc.service.CallChannel, listenerReceiveTimeout,
		"timed out waiting for flag change notification for flag %q", flagKey,
	)

	assert.Equal(t, flagKey, notification.FlagKey,
		"flag change notification had unexpected flag key")

	return notification
}

// ExpectFlagChangeNotifications waits for one general flag change notification per expected flag
// key and asserts that, taken together and in any order, they are for exactly those keys. It then
// asserts that no further notification arrives within listenerNoNotificationTimeout, so a flag
// that should not have been notified fails the test as well.
func (lc *ListenerCallback) ExpectFlagChangeNotifications(t *ldtest.T, flagKeys ...string) {
	received := make([]string, 0, len(flagKeys))
	for range flagKeys {
		notification := helpers.RequireValueWithMessage(
			t, lc.service.CallChannel, listenerReceiveTimeout,
			"timed out waiting for flag change notifications; expected %v, received %v so far", flagKeys, received,
		)
		received = append(received, notification.FlagKey)
	}
	expected := append([]string(nil), flagKeys...)
	sort.Strings(expected)
	sort.Strings(received)
	assert.Equal(t, expected, received, "flag change notifications were for unexpected flags")

	helpers.RequireNoMoreValuesWithMessage(t, lc.service.CallChannel, listenerNoNotificationTimeout,
		"received a flag change notification beyond the expected ones for %v", flagKeys)
}

// ExpectValueChangeNotification waits up to listenerReceiveTimeout for a value change
// notification for the given flag key, and asserts that the old and new values match.
// It fails the test if no notification arrives within the timeout, or if any assertion fails.
// Returns the notification for further inspection.
func (lc *ListenerCallback) ExpectValueChangeNotification(
	t *ldtest.T,
	flagKey string,
	oldValue ldvalue.Value,
	newValue ldvalue.Value,
) servicedef.ListenerNotification {
	notification := helpers.RequireValueWithMessage(
		t, lc.service.CallChannel, listenerReceiveTimeout,
		"timed out waiting for value change notification for flag %q", flagKey,
	)

	assert.Equal(t, flagKey, notification.FlagKey,
		"value change notification had unexpected flag key")
	assert.Equal(t, o.Some(oldValue), notification.OldValue,
		"value change notification had unexpected old value for flag %q", flagKey)
	assert.Equal(t, o.Some(newValue), notification.NewValue,
		"value change notification had unexpected new value for flag %q", flagKey)

	return notification
}

// ExpectNoNotification asserts that no notification arrives within listenerNoNotificationTimeout.
// Use this to verify that a listener did NOT fire (e.g., because the flag value did not change).
// The flagKey parameter is used only in the failure message if a notification unexpectedly arrives.
func (lc *ListenerCallback) ExpectNoNotification(t *ldtest.T, flagKey string) {
	lc.ExpectNoNotificationFor(t, flagKey, listenerNoNotificationTimeout)
}

// ExpectNoNotificationFor is ExpectNoNotification with a caller-chosen wait, for cases where the
// SDK could legitimately take longer than listenerNoNotificationTimeout to react to the change
// that must not produce a notification.
func (lc *ListenerCallback) ExpectNoNotificationFor(t *ldtest.T, flagKey string, timeout time.Duration) {
	helpers.RequireNoMoreValuesWithMessage(t, lc.service.CallChannel, timeout,
		"received unexpected listener notification for flag %q", flagKey)
}
