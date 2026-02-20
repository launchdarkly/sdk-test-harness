# "Previous / Stale Flag Version" and Listeners — Research Findings

**Date**: 2026-02-19  
**Context**: Researching whether a flag change listener should remain silent when the SDK receives a flag update whose version number is *lower* than the version the SDK already knows about.

---

## The Original Hypothesis

The user requested:
> "A test to verify that no notification is sent if the flag's version is changed to a number that is less than the version that the SDK currently knows about."

The intuition is reasonable: if an SDK has flag version 5 and receives an update claiming version 3, it should treat the update as stale/out-of-order and discard it, which means no listener notification.

This is a well-known pattern in **FDv1** streaming, where each flag carries its own `version` field and the SDK uses it to detect out-of-order or replayed messages.

---

## What We Tried

Two test functions were added:

- `flagChangeListenerNoNotificationForStaleVersion` — registers a general flag change listener, pushes a flag update with a *lower* version number, and expects **no** notification.
- `flagValueChangeListenerNoNotificationForStaleVersion` — same but for a value change listener.

Both tests **failed**: the SDK notified listeners even when the pushed flag version was lower than the current version.

---

## Root Cause: FDv2 Ignores Per-Flag Model Versions

The test harness operates in **FDv2 streaming mode**. FDv2 has a fundamentally different approach to versioning from FDv1:

### FDv1 (per-flag versioning)
Each flag update message contains a `"version"` field. The SDK compares this to its stored version; if the incoming version is ≤ stored version, the update is discarded as stale.

### FDv2 (payload-level versioning)
FDv2 replaces per-flag version checks with a **payload sequence number** carried in the `payload-transferred` event. The SDK uses this sequence number to decide whether a full payload is current, but individual flag object `version` fields have **no role in staleness detection**.

From the Go SDK's FDv2 tests (`sdktests/common_tests_stream_fdv2.go`), the test `IgnoresModelVersion` explicitly verifies that the SDK **does** process and apply flag updates regardless of their individual `version` field — proving that per-flag version checking is intentionally absent in FDv2.

### How `pushFlagUpdate` works in the test harness

```go
func (c CommonListenerTests) pushFlagUpdate(dataSystem *SDKDataSystem, key string, version int, value ldvalue.Value) {
    flag := c.makeListenerFlag(key, version, value)
    streaming := dataSystem.Synchronizers[0].streaming
    streaming.PushUpdate("flag", key, version, jsonhelpers.ToJSON(flag))
    streaming.PushPayloadTransferred("updated", version)  // ← the payload sequence number
}
```

The second argument to `PushUpdate` (the `version int`) is used as the `payload-transferred` sequence number in `PushPayloadTransferred`. In FDv2, **this** number is what matters for ordering. The `version` field embedded inside the flag JSON (`flag.Version`) is effectively decorative as far as FDv2 staleness logic is concerned.

---

## FDv2 Adoption Across SDKs

The test harness is currently on the `feat/fdv2` branch and uses the FDv2 streaming protocol exclusively. Understanding which SDKs have adopted FDv2 is critical to knowing where the stale-version test is valid.

### Server-Side SDKs — All use FDv2

| SDK | FDv2? | Evidence |
|---|---|---|
| **Go** | ✅ Yes | `internal/datasystem/` uses FDv2 protocol; `IgnoresModelVersion` test confirms per-flag versions are decorative |
| **Java** | ✅ Yes | `datasources/FDv2SourceResult.java` — dedicated FDv2 result type throughout data source layer |
| **Python** | ✅ Yes | `ldclient/impl/datasystem/fdv2.py` — named `FDv2` class; uses `payload-transferred` and `xfer-full` event names |
| **.NET Server** | ✅ Yes | `LdClient.cs` calls `FDv2DataSystem.Create(...)` |

All four server-side SDKs have adopted FDv2, which means per-flag version checking is absent in all of them. The stale-version test cannot be applied to any server-side SDK via the current test harness.

### Client-Side SDKs — No FDv2 (network protocol)

| SDK | FDv2? | Per-flag version check? | Notes |
|---|---|---|---|
| **Android** | ❌ No | ✅ Yes | `ContextDataManager.upsertFlag()` explicitly rejects updates where `oldFlag.getVersion() >= flag.getVersion()` |
| **JS / React Native** | ⚠️ Internal only | Unknown | Uses FDv1 streaming protocol (PUT/PATCH with evaluated results); `shared/common` contains `FDv1PayloadAdaptor` that converts FDv1 responses into FDv2-style events internally. Per-flag version checking at the storage layer is unclear. |
| **.NET Client** | Unknown | Unknown | Not yet researched |
| **iOS** | Unknown | Unknown | Not yet researched |
| **Flutter** | Unknown | Unknown | Not yet researched |

The Android SDK is the clearest case: it uses FDv1 per-flag version checking. The `ContextDataManager` source (line 239) explicitly implements:

```java
// In ContextDataManager.upsertFlag():
if (oldFlag != null && oldFlag.getVersion() >= flag.getVersion()) {
    return false;  // reject the stale update; no listener fires
}
```

---

## Conclusion

**The stale-version test is not viable for the current test harness when targeting FDv2 SDKs.**

- All current server-side SDKs use FDv2, which does not check per-flag version numbers. Pushing a flag with a lower `version` in FDv2 is processed normally, and listeners will fire.
- The test harness currently only supports FDv2 streaming, so the stale-version scenario cannot be exercised against any of the already-integrated server-side SDKs.

**However, the test IS viable for client-side SDKs that use FDv1 (most notably Android).** When Phase 3B (client-side listener tests) is implemented:

- The test harness pushes a streaming update with a lower `version` than what the SDK already has
- The client-side SDK (e.g. Android) checks per-flag versions and rejects the update
- No listener notification fires
- The test asserts silence using `ExpectNoNotification`

This would be a client-side-only test, gated with `t.RequireCapability(servicedef.CapabilityClientSide)`.

Both original server-side test functions (`flagChangeListenerNoNotificationForStaleVersion` and `flagValueChangeListenerNoNotificationForStaleVersion`) were removed from `common_tests_listeners.go` for this reason.

---

## Implications for Future Work

1. **Client-side stale-version test**: Can be implemented in Phase 3B for FDv1 client-side SDKs (Android confirmed). JS/React Native behavior at the storage layer should be verified before adding them to the test target list.
2. **FDv2 staleness (payload-transferred version)**: A lower `payload-transferred` state version in FDv2 *should* be ignored by FDv2 SDKs. This could be a testable scenario but is separate from per-flag version checking and requires dedicated test harness infrastructure.
3. **FDv1 server-side SDKs**: If any older server-side SDKs are ever integrated that don't support FDv2, a stale-version test would be valid for them. Such SDKs would not declare the `flag-change-listeners` capability (or would suppress the test), so this is not a concern for the current implementation.

---

## References

- `sdktests/common_tests_stream_fdv2.go` — `IgnoresModelVersion` test; explicitly verifies Go SDK processes flag updates regardless of per-flag `version` field in FDv2
- `mockld/streaming_service.go` — `PushUpdate` and `PushPayloadTransferred` implementation
- Java SDK — `lib/sdk/server/src/main/java/com/launchdarkly/sdk/server/datasources/FDv2SourceResult.java`
- Python SDK — `ldclient/impl/datasystem/fdv2.py`, `ldclient/interfaces.py` (defines `PAYLOAD_TRANSFERRED = "payload-transferred"` and `TRANSFER_FULL = "xfer-full"`)
- .NET Server SDK — `LdClient.cs` line ~168: `_dataSystem = FDv2DataSystem.Create(...)`
- Android SDK — `ContextDataManager.java` line ~239: per-flag version gate (`oldFlag.getVersion() >= flag.getVersion()`)
- JS/React Native — `packages/shared/common/src/internal/fdv2/FDv1PayloadAdaptor.ts` — FDv1 streaming responses adapted into FDv2 payload processing internally; `payloadProcessor.ts` — no per-object version checking at this layer
- `sdktests/common_tests_listeners.go` (git history) — `flagChangeListenerNoNotificationForStaleVersion` and `flagValueChangeListenerNoNotificationForStaleVersion` tests were added and then removed
