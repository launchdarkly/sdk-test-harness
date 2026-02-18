package sdktests

import (
	"github.com/launchdarkly/go-test-helpers/v2/jsonhelpers"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"
)

func doCommonListenerTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityFlagChangeListeners)
	t.Run("flag change listener", doFlagChangeListenerTests)
	t.Run("flag value change listener", doFlagValueChangeListenerTests)
}

func doFlagChangeListenerTests(t *ldtest.T) {
	t.Run("receives notification when flag changes", flagChangeListenerReceivesNotification)
	t.Run("fires on config change even when value unchanged", flagChangeListenerFiresOnConfigChange)
	t.Run("filters by flag key", flagChangeListenerFiltersByFlagKey)
	t.Run("with empty flag key receives all flag changes", flagChangeListenerEmptyKeyReceivesAllFlags)
}

func doFlagValueChangeListenerTests(t *ldtest.T) {
	t.Run("receives notification when value changes", flagValueChangeListenerReceivesNotification)
	t.Run("does not notify when value is unchanged", flagValueChangeListenerNoNotificationWhenUnchanged)
	t.Run("multiple listeners both receive notification", multipleValueListenersBothNotified)
	t.Run("is context specific", valueListenerIsContextSpecific)
}

// makeListenerFlag builds a server-side feature flag for listener tests. The flag evaluates to
// value as its off-variation, so any context will receive that value.
func makeListenerFlag(key string, version int, value ldvalue.Value) ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder(key).Version(version).
		On(false).OffVariation(0).Variations(value, ldvalue.String("other")).Build()
}

// createClientForListeners sets up a client with two flags (flag1 and flag2) pre-loaded via
// streaming, both initially evaluating to "value1". Use dataSystem.Synchronizers[0].streaming
// to push flag updates and trigger listener notifications.
func createClientForListeners(t *ldtest.T) (*SDKClient, *SDKDataSystem) {
	flag1 := makeListenerFlag("flag1", 1, ldvalue.String("value1"))
	flag2 := makeListenerFlag("flag2", 1, ldvalue.String("value1"))
	data := mockld.NewServerSDKDataBuilder().Flag(flag1, flag2).Build()

	dataSystem := NewSDKDataSystem(t, data)
	client := NewSDKClient(t, dataSystem)

	return client, dataSystem
}

// pushFlagUpdate pushes a flag update through the streaming service and signals that the payload
// is complete. version must increase with each call; it is used as both the flag version and the
// payload-transferred sequence number.
func pushFlagUpdate(dataSystem *SDKDataSystem, key string, version int, value ldvalue.Value) {
	flag := makeListenerFlag(key, version, value)

	streaming := dataSystem.Synchronizers[0].streaming
	streaming.PushUpdate("flag", key, version, jsonhelpers.ToJSON(flag))
	streaming.PushPayloadTransferred("updated", version)
}

// --- Flag change listener tests ---

func flagChangeListenerReceivesNotification(t *ldtest.T) {
	client, dataSystem := createClientForListeners(t)

	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback.Close()

	client.RegisterFlagChangeListener(t, servicedef.RegisterFlagChangeListenerParams{
		ListenerID:  "listener-1",
		FlagKey:     "flag1",
		CallbackURI: callback.GetURL(),
	})

	pushFlagUpdate(dataSystem, "flag1", 2, ldvalue.String("new-value"))

	callback.ExpectFlagChangeNotification(t, "flag1")
}

func flagChangeListenerFiresOnConfigChange(t *ldtest.T) {
	client, dataSystem := createClientForListeners(t)

	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback.Close()

	client.RegisterFlagChangeListener(t, servicedef.RegisterFlagChangeListenerParams{
		ListenerID:  "listener-1",
		FlagKey:     "flag1",
		CallbackURI: callback.GetURL(),
	})

	// Push an update that changes the flag's version but not its evaluated value.
	// The general flag change listener must fire regardless of value changes, because
	// it tracks configuration changes (e.g. targeting rule edits), not just value changes.
	pushFlagUpdate(dataSystem, "flag1", 2, ldvalue.String("value1"))

	callback.ExpectFlagChangeNotification(t, "flag1")
}

func flagChangeListenerEmptyKeyReceivesAllFlags(t *ldtest.T) {
	client, dataSystem := createClientForListeners(t)

	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback.Close()

	// An empty FlagKey means the listener should receive changes for any flag.
	client.RegisterFlagChangeListener(t, servicedef.RegisterFlagChangeListenerParams{
		ListenerID:  "listener-1",
		FlagKey:     "",
		CallbackURI: callback.GetURL(),
	})

	// Update flag1 — listener should fire.
	pushFlagUpdate(dataSystem, "flag1", 2, ldvalue.String("new-value"))
	callback.ExpectFlagChangeNotification(t, "flag1")

	// Update flag2 — listener should fire again.
	// Use version 3 so the payload-transferred sequence number also increments.
	pushFlagUpdate(dataSystem, "flag2", 3, ldvalue.String("new-value"))
	callback.ExpectFlagChangeNotification(t, "flag2")
}

func flagChangeListenerFiltersByFlagKey(t *ldtest.T) {
	client, dataSystem := createClientForListeners(t)

	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback.Close()

	// Register listener only for flag1.
	client.RegisterFlagChangeListener(t, servicedef.RegisterFlagChangeListenerParams{
		ListenerID:  "listener-1",
		FlagKey:     "flag1",
		CallbackURI: callback.GetURL(),
	})

	// Update flag2 — should NOT trigger the listener.
	pushFlagUpdate(dataSystem, "flag2", 2, ldvalue.String("new-value"))
	callback.ExpectNoNotification(t, "flag1")

	// Update flag1 — SHOULD trigger the listener.
	pushFlagUpdate(dataSystem, "flag1", 2, ldvalue.String("another-value"))
	callback.ExpectFlagChangeNotification(t, "flag1")
}

// --- Flag value change listener tests ---

func flagValueChangeListenerReceivesNotification(t *ldtest.T) {
	client, dataSystem := createClientForListeners(t)

	context := ldcontext.New("user-key")
	oldValue := ldvalue.String("value1")
	newValue := ldvalue.String("new-value")
	defaultValue := ldvalue.String("default")

	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback.Close()

	client.RegisterFlagValueChangeListener(t, servicedef.RegisterFlagValueChangeListenerParams{
		ListenerID:   "listener-1",
		FlagKey:      "flag1",
		Context:      context,
		DefaultValue: defaultValue,
		CallbackURI:  callback.GetURL(),
	})

	pushFlagUpdate(dataSystem, "flag1", 2, newValue)

	callback.ExpectValueChangeNotification(t, "flag1", oldValue, newValue)
}

func flagValueChangeListenerNoNotificationWhenUnchanged(t *ldtest.T) {
	client, dataSystem := createClientForListeners(t)

	context := ldcontext.New("user-key")

	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback.Close()

	client.RegisterFlagValueChangeListener(t, servicedef.RegisterFlagValueChangeListenerParams{
		ListenerID:   "listener-1",
		FlagKey:      "flag1",
		Context:      context,
		DefaultValue: ldvalue.String("default"),
		CallbackURI:  callback.GetURL(),
	})

	// Update flag1 with a new version but the same evaluated value — should NOT trigger notification.
	pushFlagUpdate(dataSystem, "flag1", 2, ldvalue.String("value1"))
	callback.ExpectNoNotification(t, "flag1")
}

func multipleValueListenersBothNotified(t *ldtest.T) {
	client, dataSystem := createClientForListeners(t)

	context := ldcontext.New("user-key")
	oldValue := ldvalue.String("value1")
	newValue := ldvalue.String("new-value")
	defaultValue := ldvalue.String("default")

	callback1 := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback1.Close()
	callback2 := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback2.Close()

	// Register two independent listeners for the same flag and context.
	client.RegisterFlagValueChangeListener(t, servicedef.RegisterFlagValueChangeListenerParams{
		ListenerID:   "listener-1",
		FlagKey:      "flag1",
		Context:      context,
		DefaultValue: defaultValue,
		CallbackURI:  callback1.GetURL(),
	})
	client.RegisterFlagValueChangeListener(t, servicedef.RegisterFlagValueChangeListenerParams{
		ListenerID:   "listener-2",
		FlagKey:      "flag1",
		Context:      context,
		DefaultValue: defaultValue,
		CallbackURI:  callback2.GetURL(),
	})

	pushFlagUpdate(dataSystem, "flag1", 2, newValue)

	// Both listeners must receive the notification independently.
	callback1.ExpectValueChangeNotification(t, "flag1", oldValue, newValue)
	callback2.ExpectValueChangeNotification(t, "flag1", oldValue, newValue)
}

func valueListenerIsContextSpecific(t *ldtest.T) {
	context1 := ldcontext.New("user-1")
	context2 := ldcontext.New("user-2")
	defaultValue := ldvalue.String("default")

	// Initially both contexts see "value1" (flag is off, returns the same off-variation for all).
	flag1 := makeListenerFlag("flag1", 1, ldvalue.String("value1"))
	data := mockld.NewServerSDKDataBuilder().Flag(flag1).Build()
	dataSystem := NewSDKDataSystem(t, data)
	client := NewSDKClient(t, dataSystem)

	callback1 := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback1.Close()
	callback2 := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback2.Close()

	client.RegisterFlagValueChangeListener(t, servicedef.RegisterFlagValueChangeListenerParams{
		ListenerID:   "listener-1",
		FlagKey:      "flag1",
		Context:      context1,
		DefaultValue: defaultValue,
		CallbackURI:  callback1.GetURL(),
	})
	client.RegisterFlagValueChangeListener(t, servicedef.RegisterFlagValueChangeListenerParams{
		ListenerID:   "listener-2",
		FlagKey:      "flag1",
		Context:      context2,
		DefaultValue: defaultValue,
		CallbackURI:  callback2.GetURL(),
	})

	// Push an updated flag that returns "updated-value" for user-1 via a targeting rule,
	// and "value1" (unchanged) for everyone else via the fallthrough.
	updatedFlag := ldbuilders.NewFlagBuilder("flag1").Version(2).
		On(true).
		FallthroughVariation(0).
		Variations(ldvalue.String("value1"), ldvalue.String("updated-value")).
		AddRule(ldbuilders.NewRuleBuilder().ID("target-rule").Variation(1).Clauses(
			ldbuilders.Clause(ldattr.KeyAttr, ldmodel.OperatorIn, ldvalue.String("user-1")),
		)).
		Build()

	streaming := dataSystem.Synchronizers[0].streaming
	streaming.PushUpdate("flag", "flag1", 2, jsonhelpers.ToJSON(updatedFlag))
	streaming.PushPayloadTransferred("updated", 2)

	// context1 (user-1): value changed from "value1" to "updated-value" → notification expected.
	callback1.ExpectValueChangeNotification(t, "flag1", ldvalue.String("value1"), ldvalue.String("updated-value"))

	// context2 (user-2): value unchanged ("value1" → "value1") → no notification expected.
	callback2.ExpectNoNotification(t, "flag1")
}
