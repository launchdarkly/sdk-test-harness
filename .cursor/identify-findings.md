# Identify and Flag Change Listeners — Research Findings

**Date**: 2026-02-19  
**Context**: Researching whether a test case can verify that flag change listeners fire when a client-side SDK's `Identify` is called.

---

## What is `Identify`?

`Identify` exists in **both server-side and client-side SDKs**, but the two meanings are entirely different. This distinction is critical for flag change listeners.

### Server-Side SDKs: `Identify` = analytics event only

All server-side SDKs expose an `identify()` method, but it only sends an **analytics (telemetry) event** to LaunchDarkly so LD knows about that context. It has no effect on flag evaluation or on flag change listeners.

| SDK | Method | Effect |
|---|---|---|
| **Go** | `client.Identify(context)` | Sends an identify event to LD's event processor; no flag evaluation, no listener trigger |
| **Java** | `client.identify(context)` | Calls `eventProcessor.recordIdentifyEvent(context)`; no flag evaluation |
| **.NET Server** | `client.Identify(context)` | Calls `_eventProcessor.RecordIdentifyEvent(...)`; no flag evaluation |
| **Python** | `client.identify(context)` | Calls `self._send_event(self._event_factory_default.new_identify_event(context))`; no flag evaluation |

None of these server-side `identify()` calls change the SDK's evaluation state, trigger a flag re-evaluation, or fire flag change listeners. Server-side SDKs evaluate flags for whatever context is passed to each individual `variation()` call and maintain no single "current context."

### Client-Side SDKs: `Identify` = context switch + fresh flag fetch

In client-side SDKs, `identify()` is a fundamentally different operation. It **switches the SDK's current evaluation context** (e.g. from an anonymous user to a known user after login), fetches fresh flag evaluations for the new context from LaunchDarkly, and can therefore trigger flag change listeners if any flag values differ for the new context.

This is well-documented in Confluence: ["Start/Identify and Waiting for Flag Data / Error (Audited Aug 2023)"](https://launchdarkly.atlassian.net/wiki/spaces/PD/pages/2532606366) covers the behavior of `identify` across iOS, Android, Flutter, React Native, and .NET client SDKs, specifically noting that `identify` causes the SDK to fetch new flag values for the new context.

---

## How Listeners Interact with `Identify`

For **client-side SDKs**: when `Identify` fetches fresh flag evaluations for the new context, a flag value change listener **should fire** if the flag's evaluated value is different for the new context than it was for the old one.

For **server-side SDKs**: `Identify` only sends an analytics event and has no effect on listeners whatsoever.

This distinction matters for the test harness: any test of "listener fires on Identify" must target client-side SDKs only.

### Client-Side SDK-Specific Behavior

| SDK | Fires on Identify? | Notes |
|---|---|---|
| **JavaScript / React Native** | ✅ Yes | `client.on('change')` and `client.on('change:key')` fire whenever the flag manager detects value changes, including after `Identify` |
| **.NET Client SDK** | ✅ Yes (with caveats) | `IFlagTracker.FlagValueChanged` fires on `Identify` when flag values differ. However, it does **not** fire when offline + `Identify` + stored data is loaded — it only fires for fresh data from LD or `TestData` |
| **Android SDK** | ✅ Yes | `FeatureFlagChangeListener` fires when flag values change for the new context after `Identify` |

### Server-Side SDKs

| SDK | Fires on Identify? | Notes |
|---|---|---|
| **Go Server SDK** | ❌ No | `identify()` sends an analytics event only; no listener trigger |
| **Java Server SDK** | ❌ No | `identify()` sends an analytics event only; no listener trigger |
| **.NET Server SDK** | ❌ No | `identify()` sends an analytics event only; no listener trigger |
| **Python Server SDK** | ❌ No | `identify()` sends an analytics event only; no listener trigger |

---

## The Test Case Proposal

The request was to verify:
> "A notification is received when the SDK client's configuration is changed. For example, when the 'Identify' action is performed. This is different than when a flag's config is updated."

### Analysis

- **Server-side SDKs**: `identify()` sends an analytics event only — it has no effect on flag evaluation or listeners. There is nothing to test for listeners here.
- **Client-side SDKs**: After `identify()` fetches fresh flag values for the new context, a value change listener **should** fire if any flag value differs.

### Conclusion

The test is **client-side only**. Since at the time of this discussion the test harness did not yet support client-side listener tests, the test was **deferred** to Phase 3B (client-side listener implementation).

Confirmed: "OK, let's implement test 2 now, and defer test 1 for when we implement client-side SDK tests."

---

## Proposed Test Design (Deferred)

When Phase 3B is implemented, the test would look roughly like:

```go
func (c CommonListenerTests) listenerFiresOnIdentify(t *ldtest.T) {
    t.RequireCapability(servicedef.CapabilityClientSide)

    // Set up flag that returns different values for two different contexts.
    // context1 sees "value-for-user-1", context2 sees "value-for-user-2".
    // ... (client-side data setup using ClientSDKData) ...

    // Initialize client with context1.
    client := NewSDKClient(t, WithClientSideInitialContext(context1), ...)

    // Register a value change listener for "flag1".
    callback := NewListenerCallback(requireContext(t).harness, t.DebugLogger())
    defer callback.Close()
    client.RegisterFlagValueChangeListener(t, servicedef.RegisterFlagValueChangeListenerParams{
        ListenerID:   "listener-1",
        FlagKey:      "flag1",
        CallbackURI:  callback.GetURL(),
        // Note: no Context field needed for client-side (client uses its current context)
    })

    // Identify as context2 (a different user).
    client.SendIdentifyEvent(t, servicedef.IdentifyEventParams{Context: context2})

    // The listener should fire because "flag1" has a different value for context2.
    callback.ExpectValueChangeNotification(t, "flag1", valueForUser1, valueForUser2)
}
```

### Key Implementation Notes

1. **Context in `RegisterFlagValueChangeListenerParams`**: Client-side SDKs ignore the `Context` field — they always use the client's own current context. Test code should omit it.
2. **Data setup**: Client-side tests use `ClientSDKData` (pre-evaluated flag results), not full `FeatureFlag` model JSON. The two contexts must be represented as separate data sets or by using LD targeting — but since client-side data is pre-evaluated, the test service needs to serve different values depending on which context the client requests.
3. **`Identify` command**: The test harness needs a command to call `Identify` on the SDK client (this likely already exists as part of client-side event tests).
4. **.NET caveat**: The .NET Client SDK documents that `FlagValueChanged` does **not** fire when the SDK loads stored/cached flag data offline. Tests should ensure the SDK is online and fetching fresh data to reliably trigger the listener.

---

## Confluence Search Results

Searched Confluence for pages mentioning "identify" + "SDK" and "identify" + "flag change". No pages were found that specifically cover the intersection of flag change listeners and identify. The most relevant pages found:

- ["Start/Identify and Waiting for Flag Data / Error (Audited Aug 2023)"](https://launchdarkly.atlassian.net/wiki/spaces/PD/pages/2532606366) — Documents client-side `identify` behavior across iOS, Android, Flutter, React Native, and .NET client; confirms `identify` triggers a network fetch for fresh flag values
- ["One pager: don't publish index or identify events for anonymous contexts in server-side SDKs"](https://launchdarkly.atlassian.net/wiki/spaces/ENG/pages/2838462539) — Discusses the analytics-only nature of `identify` in server-side SDKs; confirms `identify` in server-side is about index/identify events sent to LD's event pipeline, not about flag evaluation

## References

- `.cursor/change-listener-tests-research.md` — Section "Client-Side SDK Analysis" → "Tests That Need Client-Side-Specific Variants"
- Go Server SDK — `ldclient.go` line ~459: `func (client *LDClient) Identify(context ldcontext.Context) error` — calls `eventProcessor.RecordIdentifyEvent()` only
- Java Server SDK — `LDClient.java` line ~317: `public void identify(LDContext context)` — calls `eventProcessor.recordIdentifyEvent(context)` only
- .NET Server SDK — `LdClient.cs` line ~605: `public void Identify(Context context)` — calls `_eventProcessor.RecordIdentifyEvent(...)` only
- Python Server SDK — `ldclient/client.py` line ~443: `def identify(self, context)` — calls `self._send_event(self._event_factory_default.new_identify_event(context))` only
- `.NET Client SDK` — `pkgs/sdk/client/src/Interfaces/IFlagTracker.cs` (docs on `FlagValueChanged`, including the identify caveat)
- `js-core` — `packages/shared/sdk-client/src/LDClientImpl.ts` (lines ~147–153: `_flagManager.on(...)` emits `change` events after any flag update including after Identify)
- `android-client-sdk` — `LDClientInterface.java` (`registerFeatureFlagListener`)
