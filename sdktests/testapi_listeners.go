package sdktests

import (
	"time"

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
	helpers.RequireNoMoreValuesWithMessage(t, lc.service.CallChannel, listenerNoNotificationTimeout,
		"received unexpected listener notification for flag %q", flagKey)
}
