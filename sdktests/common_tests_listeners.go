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

// CommonListenerTests groups together flag change listener tests that are shared between
// server-side and client-side SDKs. Embed commonTestsBase to gain SDK-kind awareness so
// that later commits can branch on isClientSide for data format and context handling.
type CommonListenerTests struct {
	commonTestsBase
}

// NewCommonListenerTests constructs a CommonListenerTests for the current test scope.
func NewCommonListenerTests(t *ldtest.T) CommonListenerTests {
	return CommonListenerTests{newCommonTestsBase(t, "CommonListenerTests")}
}

func doCommonListenerTests(t *ldtest.T) {
	t.RequireCapability(servicedef.CapabilityFlagChangeListeners)
	c := NewCommonListenerTests(t)
	t.Run("flag change listener", c.doFlagChangeListenerTests)
	t.Run("flag value change listener", c.doFlagValueChangeListenerTests)
}

func (c CommonListenerTests) doFlagChangeListenerTests(t *ldtest.T) {
	t.Run("receives notification when flag changes", c.flagChangeListenerReceivesNotification)
	t.Run("fires on config change even when value unchanged", c.flagChangeListenerFiresOnConfigChange)
	t.Run("filters by flag key", c.flagChangeListenerFiltersByFlagKey)
	t.Run("with empty flag key receives all flag changes", c.flagChangeListenerEmptyKeyReceivesAllFlags)
}

func (c CommonListenerTests) doFlagValueChangeListenerTests(t *ldtest.T) {
	t.Run("receives notification when value changes", c.flagValueChangeListenerReceivesNotification)
	t.Run("does not notify when value is unchanged", c.flagValueChangeListenerNoNotificationWhenUnchanged)
	t.Run("multiple listeners both receive notification", c.multipleValueListenersBothNotified)
	t.Run("is context specific", c.valueListenerIsContextSpecific)
}

// makeListenerFlag builds a server-side feature flag for listener tests. The flag evaluates to
// value as its off-variation, so any context will receive that value.
func (c CommonListenerTests) makeListenerFlag(key string, version int, value ldvalue.Value) ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder(key).Version(version).
		On(false).OffVariation(0).Variations(value, ldvalue.String("other")).Build()
}

// makeClientSideListenerFlag builds a client-side flag entry for listener tests.
func (c CommonListenerTests) makeClientSideListenerFlag(key string, version int, value ldvalue.Value) mockld.ClientSDKFlagWithKey { //nolint:lll
	return mockld.ClientSDKFlagWithKey{
		Key:           key,
		ClientSDKFlag: mockld.ClientSDKFlag{Version: version, Value: value},
	}
}

// setupListenerDataSystems creates a streaming-capable data system for listener tests and returns
// the configurers needed to wire it into the SDK client. Always uses streaming as the main
// synchronizer so that tests can push flag updates at will via pushFlagUpdate.
//
// Configurer ordering is critical: SDKDataSystem.Configure overwrites DataSystem.Synchronizers
// entirely, so only the last SDKDataSystem in the chain takes effect. For SDK kinds that need
// both a polling endpoint AND a streaming endpoint (JS), we apply the streaming SDKDataSystem
// first and then append the polling URL with WithPollingSynchronizer, which appends rather than
// overwrites.
func (c CommonListenerTests) setupListenerDataSystems(
	t *ldtest.T, initialData mockld.SDKData,
) (*SDKDataSystem, []SDKConfigurer) {
	// The streaming data system is the one we push updates through and return as the primary handle.
	dataSystem := NewSDKDataSystem(t, initialData, DataSystemOptionStreaming())

	switch c.sdkKind {
	case mockld.ServerSideSDK:
		return dataSystem, []SDKConfigurer{dataSystem}

	case mockld.RokuSDK:
		fallthrough
	case mockld.MobileSDK:
		// Mobile SDKs require a polling endpoint to exist even when streaming is primary.
		// The polling configurer runs before dataSystem so the streaming URL wins.
		emptyPollingDataSource := NewSDKDataSystem(t, nil, DataSystemOptionPolling())
		return dataSystem, []SDKConfigurer{emptyPollingDataSource, dataSystem}

	case mockld.JSClientSDK:
		// JS-based SDKs always poll for initial data, then open an SSE connection for updates.
		// Both URLs must reach the browser entity. We cannot use two SDKDataSystem configurers
		// because the second overwrites the first's synchronizer list. Instead we apply
		// dataSystem (streaming) first to set Synchronizers = [{Streaming: url}], then append
		// the polling URL with WithPollingSynchronizer, which appends rather than overwrites,
		// giving Synchronizers = [{Streaming: url}, {Polling: url}].
		pollingDataSource := NewSDKDataSystem(t, initialData, DataSystemOptionPolling())
		pollingURL := pollingDataSource.Synchronizers[0].Endpoint().BaseURL()
		return dataSystem, []SDKConfigurer{
			dataSystem,
			WithPollingSynchronizer(servicedef.SDKConfigPollingParams{BaseURI: pollingURL}),
		}

	default:
		panic("unknown SDK kind for listener tests")
	}
}

// createClient sets up a client with two flags (flag1 and flag2) pre-loaded, both initially
// evaluating to "value1". Call pushFlagUpdate to push changes and trigger listener notifications.
func (c CommonListenerTests) createClient(t *ldtest.T) (*SDKClient, *SDKDataSystem) {
	var initialData mockld.SDKData
	if c.isClientSide {
		initialData = mockld.NewClientSDKDataBuilder().
			FlagWithValue("flag1", 1, ldvalue.String("value1"), 0).
			FlagWithValue("flag2", 1, ldvalue.String("value1"), 0).
			Build()
	} else {
		flag1 := c.makeListenerFlag("flag1", 1, ldvalue.String("value1"))
		flag2 := c.makeListenerFlag("flag2", 1, ldvalue.String("value1"))
		initialData = mockld.NewServerSDKDataBuilder().Flag(flag1, flag2).Build()
	}

	dataSystem, configurers := c.setupListenerDataSystems(t, initialData)
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
	return client, dataSystem
}

// pushFlagUpdate pushes a flag update through the streaming service.
//
// Server-side SDKs use FDv2 streaming (put-object + payload-transferred).
// Client-side SDKs (browser, mobile) use FDv1 streaming (patch), because the
// client-side StreamingProcessor only listens for "put", "patch", and "delete"
// events and silently ignores FDv2 event names.
func (c CommonListenerTests) pushFlagUpdate(dataSystem *SDKDataSystem, key string, version int, value ldvalue.Value) {
	streaming := dataSystem.Synchronizers[0].streaming
	if c.isClientSide {
		clientFlag := c.makeClientSideListenerFlag(key, version, value)
		streaming.PushEvent("patch", clientFlag)
	} else {
		flag := c.makeListenerFlag(key, version, value)
		streaming.PushUpdate("flag", key, version, jsonhelpers.ToJSON(flag))
		streaming.PushPayloadTransferred("updated", version)
	}
}

// --- Flag change listener tests ---

func (c CommonListenerTests) flagChangeListenerReceivesNotification(t *ldtest.T) {
	client, dataSystem := c.createClient(t)

	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback.Close()

	client.RegisterFlagChangeListener(t, servicedef.RegisterFlagChangeListenerParams{
		ListenerID:  "listener-1",
		FlagKey:     "flag1",
		CallbackURI: callback.GetURL(),
	})

	c.pushFlagUpdate(dataSystem, "flag1", 2, ldvalue.String("new-value"))

	callback.ExpectFlagChangeNotification(t, "flag1")
}

func (c CommonListenerTests) flagChangeListenerFiresOnConfigChange(t *ldtest.T) {
	// General flag change listeners track configuration changes, not just value changes.
	// Client-side SDKs only receive pre-evaluated values and have no concept of "config change
	// without value change", so this test applies to server-side SDKs only.
	t.RequireCapability(servicedef.CapabilityServerSide)

	client, dataSystem := c.createClient(t)

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
	c.pushFlagUpdate(dataSystem, "flag1", 2, ldvalue.String("value1"))

	callback.ExpectFlagChangeNotification(t, "flag1")
}

func (c CommonListenerTests) flagChangeListenerEmptyKeyReceivesAllFlags(t *ldtest.T) {
	client, dataSystem := c.createClient(t)

	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback.Close()

	// An empty FlagKey means the listener should receive changes for any flag.
	client.RegisterFlagChangeListener(t, servicedef.RegisterFlagChangeListenerParams{
		ListenerID:  "listener-1",
		FlagKey:     "",
		CallbackURI: callback.GetURL(),
	})

	// Update flag1 — listener should fire.
	c.pushFlagUpdate(dataSystem, "flag1", 2, ldvalue.String("new-value"))
	callback.ExpectFlagChangeNotification(t, "flag1")

	// Update flag2 — listener should fire again.
	// Use version 3 so the payload-transferred sequence number also increments.
	c.pushFlagUpdate(dataSystem, "flag2", 3, ldvalue.String("new-value"))
	callback.ExpectFlagChangeNotification(t, "flag2")
}

func (c CommonListenerTests) flagChangeListenerFiltersByFlagKey(t *ldtest.T) {
	client, dataSystem := c.createClient(t)

	callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
	defer callback.Close()

	// Register listener only for flag1.
	client.RegisterFlagChangeListener(t, servicedef.RegisterFlagChangeListenerParams{
		ListenerID:  "listener-1",
		FlagKey:     "flag1",
		CallbackURI: callback.GetURL(),
	})

	// Update flag2 — should NOT trigger the listener.
	c.pushFlagUpdate(dataSystem, "flag2", 2, ldvalue.String("new-value"))
	callback.ExpectNoNotification(t, "flag1")

	// Update flag1 — SHOULD trigger the listener.
	c.pushFlagUpdate(dataSystem, "flag1", 2, ldvalue.String("another-value"))
	callback.ExpectFlagChangeNotification(t, "flag1")
}

// --- Flag value change listener tests ---

func (c CommonListenerTests) flagValueChangeListenerReceivesNotification(t *ldtest.T) {
	client, dataSystem := c.createClient(t)

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

	c.pushFlagUpdate(dataSystem, "flag1", 2, newValue)

	callback.ExpectValueChangeNotification(t, "flag1", oldValue, newValue)
}

func (c CommonListenerTests) flagValueChangeListenerNoNotificationWhenUnchanged(t *ldtest.T) {
	client, dataSystem := c.createClient(t)

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
	c.pushFlagUpdate(dataSystem, "flag1", 2, ldvalue.String("value1"))
	callback.ExpectNoNotification(t, "flag1")
}

func (c CommonListenerTests) multipleValueListenersBothNotified(t *ldtest.T) {
	client, dataSystem := c.createClient(t)

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

	c.pushFlagUpdate(dataSystem, "flag1", 2, newValue)

	// Both listeners must receive the notification independently.
	callback1.ExpectValueChangeNotification(t, "flag1", oldValue, newValue)
	callback2.ExpectValueChangeNotification(t, "flag1", oldValue, newValue)
}

func (c CommonListenerTests) valueListenerIsContextSpecific(t *ldtest.T) {
	// Client-side SDKs evaluate flags for a single, fixed context set at initialization.
	// Registering listeners for multiple independent contexts is a server-side-only concept.
	t.RequireCapability(servicedef.CapabilityServerSide)

	context1 := ldcontext.New("user-1")
	context2 := ldcontext.New("user-2")
	defaultValue := ldvalue.String("default")

	// Initially both contexts see "value1" (flag is off, returns the same off-variation for all).
	flag1 := c.makeListenerFlag("flag1", 1, ldvalue.String("value1"))
	data := mockld.NewServerSDKDataBuilder().Flag(flag1).Build()
	dataSystem := NewSDKDataSystem(t, data)
	client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSystem)...)

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
