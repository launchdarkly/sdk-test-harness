# Flag Change Listener Tests - Research Document

**Status**: Planning  
**Last Updated**: 2026-02-12  
**Related Documentation**: [LaunchDarkly Flag Changes Documentation](https://launchdarkly.com/docs/sdk/features/flag-changes)

## Table of Contents
- [Overview](#overview)
- [Current State](#current-state)
- [Go SDK Implementation Analysis](#go-sdk-implementation-analysis)
- [Multi-SDK Implementation Analysis](#multi-sdk-implementation-analysis)
  - [Cross-SDK Comparison](#cross-sdk-comparison)
  - [Universal Design Patterns](#universal-design-patterns)
  - [Implications for Test Harness](#implications-for-test-harness)
- [Client-Side SDK Analysis](#client-side-sdk-analysis)
  - [Client-Side Listener APIs](#client-side-listener-apis)
  - [Key Differences: Client-Side vs Server-Side](#key-differences-client-side-vs-server-side)
  - [Can the Callback Architecture Work for Client-Side?](#can-the-callback-architecture-work-for-client-side)
  - [Streaming Data Format for Client-Side](#streaming-data-format-for-client-side)
  - [Plan: Sharing Tests Between Server and Client Side](#plan-sharing-tests-between-server-and-client-side)
- [Proposed Test Harness Design](#proposed-test-harness-design)
- [Implementation Plan](#implementation-plan)
- [Open Questions](#open-questions)
- [References](#references)

---

## Overview

This document describes the design and implementation plan for adding flag change listener tests to the SDK test harness. Flag change listeners allow applications to react to feature flag changes immediately through push notifications rather than polling.

### What are Flag Change Listeners?

Flag change listeners are SDK features that notify application code when feature flag configurations or values change. They provide a push-based alternative to polling for flag changes.

**Use Cases:**
- Update UI immediately when a flag changes
- React to promotional flag changes
- Enable/disable feature code paths efficiently
- Show/hide outage bulletins
- Grant entitlements dynamically

### SDK Support

Flag change listeners are supported across:
- **Server-side SDKs**: .NET, Go, Java, Node.js, Python, Ruby
- **Client-side SDKs**: .NET (client), Android, C++, Electron, Flutter, iOS, JavaScript, Node.js (client), React Native, Roku

---

## Current State

### Test Harness Status

After reviewing the test harness codebase, **flag change listeners are NOT currently tested**.

**Evidence:**
- No dedicated test files for listeners (no `*listener*.go` or `*change*.go` files)
- No commands in `servicedef/command_params.go` for listener lifecycle
- No capability flags in the service spec for listener support
- Existing stream update tests (`common_tests_stream_updates.go`) verify flag updates by **polling** the evaluation API, not via push notifications

**What Currently Exists:**
- Tests that push flag updates via streaming (`mockld.StreamingService.PushUpdate()`)
- Helper functions that poll the client to check if values changed:
  - `checkForUpdatedValue()` - polls once
  - `pollUntilFlagValueUpdated()` - polls repeatedly with timeout
- These verify the SDK receives updates correctly but don't test listener APIs

---

## Go SDK Implementation Analysis

We analyzed the [Go server SDK](https://github.com/launchdarkly/go-server-sdk) implementation as a reference.

### Architecture

**Interface Location**: `interfaces/flag_tracker.go`  
**Implementation**: `internal/flag_tracker_impl.go`  
**Tests**: `ldclient_listeners_test.go`, `ldclient_listeners_fdv2_test.go`

### Two Listener Types

#### 1. General Flag Change Listener

```go
AddFlagChangeListener() <-chan FlagChangeEvent
```

**Characteristics:**
- Returns a Go channel receiving `FlagChangeEvent`
- Fires when **any** flag configuration changes
- Event contains only `Key` (flag key) - no values
- Also fires for prerequisite or segment changes
- **Does not guarantee value changed** - just that configuration changed

**Event Structure:**
```go
type FlagChangeEvent struct {
    Key string  // The flag key that changed
}
```

#### 2. Flag Value Change Listener

```go
AddFlagValueChangeListener(
    flagKey string,
    context ldcontext.Context,
    defaultValue ldvalue.Value,
) <-chan FlagValueChangeEvent
```

**Characteristics:**
- Tracks a **specific flag and evaluation context**
- Immediately evaluates flag upon registration (gets initial value)
- Re-evaluates whenever underlying flag changes
- Only sends event if **value actually changed** (not just config)
- Requires evaluation context even for global flags

**Event Structure:**
```go
type FlagValueChangeEvent struct {
    Key      string        // Flag key
    OldValue ldvalue.Value // Previous value
    NewValue ldvalue.Value // New value
}
```

### Implementation Details

**Channel-Based Architecture:**
- Go uses channels (goroutines) rather than callbacks
- Caller is responsible for consuming from channels
- Important: Unconsumed events can block SDK goroutines

**Value Change Implementation:**
```go
func runValueChangeListener(
    flagCh <-chan interfaces.FlagChangeEvent,
    valueCh chan<- interfaces.FlagValueChangeEvent,
    evaluateFn func(flagKey string, context ldcontext.Context, defaultValue ldvalue.Value) ldvalue.Value,
    flagKey string,
    context ldcontext.Context,
    defaultValue ldvalue.Value,
)
```

**How it works:**
1. Evaluates flag immediately to get initial value
2. Subscribes to general flag change events
3. Filters for the specific flag key
4. Re-evaluates on each change
5. Compares old vs new value
6. Only sends event if value changed

**Broadcaster Pattern:**
- Uses internal `Broadcaster[FlagChangeEvent]`
- All listeners subscribe to this broadcaster
- Broadcaster pushes events to all registered listeners

### Client Access

```go
client := MakeClient(sdkKey, waitTime)
tracker := client.GetFlagTracker()

// General listener
ch1 := tracker.AddFlagChangeListener()

// Value listener
ch2 := tracker.AddFlagValueChangeListener(flagKey, context, defaultValue)
```

### Test Examples

From `ldclient_listeners_test.go`:

```go
// Test general flag change events
ch1 := client.GetFlagTracker().AddFlagChangeListener()
ch2 := client.GetFlagTracker().AddFlagChangeListener()

testData.Update(testData.Flag(flagKey))

event1 := <-ch1  // Receive event
assert.Equal(t, flagKey, event1.Key)

// Test value change events
user := lduser.NewUser("important-user")
ch := client.GetFlagTracker().AddFlagValueChangeListener(flagKey, user, ldvalue.Null())

testData.Update(testData.Flag(flagKey).VariationForUser(user.Key(), true))

event := <-ch
assert.Equal(t, flagKey, event.Key)
assert.Equal(t, ldvalue.Bool(false), event.OldValue)
assert.Equal(t, ldvalue.Bool(true), event.NewValue)
```

### Key Insights

1. **Channel vs Callback**: Go uses channels, but most languages use callbacks/events - test harness needs language-agnostic approach
2. **Two Distinct APIs**: Config changes vs value changes - value changes are more application-relevant
3. **Context Required**: Even "global" flags need a context for value listeners
4. **Immediate Evaluation**: Value listeners evaluate on registration, not just changes
5. **Filtering in SDK**: SDK handles filtering general changes to specific flags internally

---

## Multi-SDK Implementation Analysis

To inform a comprehensive test harness design, we analyzed flag change listener implementations across three additional SDKs representing different language paradigms and architectural patterns.

### .NET SDK Implementation

**Repository**: [launchdarkly/dotnet-core](https://github.com/launchdarkly/dotnet-core)  
**Interface Location**: `pkgs/sdk/server/src/Interfaces/IFlagTracker.cs`  
**Implementation**: `pkgs/sdk/server/src/Internal/FlagTrackerImpl.cs`

#### Architecture Pattern: .NET Events

The .NET SDK uses C# events (delegate-based observers) for listener notifications:

```csharp
public interface IFlagTracker
{
    // General flag change event
    event EventHandler<FlagChangeEvent> FlagChanged;
    
    // Factory method for value change handler
    EventHandler<FlagChangeEvent> FlagValueChangeHandler(
        string flagKey,
        Context context,
        EventHandler<FlagValueChangeEvent> handler
    );
}
```

#### Two-Tier Event System

**1. General Flag Changes:**
```csharp
client.FlagTracker.FlagChanged += (sender, eventArgs) => {
    Console.WriteLine("Flag changed: " + eventArgs.Key);
};
```

**2. Value-Specific Changes (Factory Pattern):**
```csharp
var listenForNewValue = client.FlagTracker.FlagValueChangeHandler(
    flagKey,
    contextForFlagEvaluation,
    (sender, changeArgs) => {
        Console.WriteLine($"Flag '{changeArgs.Key}' changed from " +
            $"{changeArgs.OldValue} to {changeArgs.NewValue}");
    }
);
client.FlagTracker.FlagChanged += listenForNewValue;
```

#### Event Structures

```csharp
public struct FlagChangeEvent {
    public string Key { get; }
}

public struct FlagValueChangeEvent {
    public string Key { get; }
    public LdValue OldValue { get; }
    public LdValue NewValue { get; }
}
```

#### Implementation Pattern: Monitor Class

The .NET SDK uses an internal `FlagValueChangeMonitor` class that:
1. Evaluates the flag immediately on creation
2. Stores the current value with thread-safe locking
3. Subscribes to general flag changes
4. Re-evaluates on each change and compares
5. Only invokes handler if value changed

```csharp
private sealed class FlagValueChangeMonitor {
    private readonly object _valueLock = new object();
    private LdValue _value;
    
    internal void OnFlagChanged(object sender, FlagChangeEvent eventArgs) {
        if (eventArgs.Key != _flagKey) return;
        
        var newValue = _evaluateFn(_flagKey, _context);
        LdValue oldValue = LdValue.Null;
        bool changed = false;
        
        lock (_valueLock) {
            if (!_value.Equals(newValue)) {
                changed = true;
                oldValue = _value;
                _value = newValue;
            }
        }
        
        if (changed) {
            _handler(sender, new FlagValueChangeEvent(_flagKey, oldValue, newValue));
        }
    }
}
```

#### Key Insights for .NET
- **Event-based pattern**: Standard C# events (not channels or promises)
- **Factory method**: Creates and returns a handler to attach to main event
- **Thread safety**: Explicit locking for value storage
- **Synchronous invocation**: Handlers called directly on event thread

---

### JavaScript SDK Implementation

**Repository**: [launchdarkly/js-core](https://github.com/launchdarkly/js-core)  
**Client Location**: `packages/shared/sdk-client/src/LDClientImpl.ts`  
**Emitter**: `packages/shared/sdk-client/src/LDEmitter.ts`

#### Architecture Pattern: EventEmitter

The JavaScript SDK uses an event emitter pattern similar to Node.js:

```typescript
type EventName = 
  | 'change'                    // All flag changes
  | `change:${string}`          // Specific flag changes (e.g., 'change:my-flag')
  | 'dataSourceStatus'
  | 'error'
  | 'initialized'
  | 'ready';

class LDEmitter {
  on(name: EventName, listener: Function): void
  off(name: EventName, listener?: Function): void
  emit(name: EventName, ...detail: any[]): void
}
```

#### Usage Patterns

**1. Listen to All Flag Changes:**
```javascript
client.on('change', (changes) => {
  console.log('Flags changed:', Object.keys(changes));
});
```

**2. Listen to Specific Flag:**
```javascript
client.on('change:my-flag-key', (current, previous) => {
  console.log(`Flag changed from ${previous} to ${current}`);
});
```

#### Event Payload

Unlike Go and .NET which provide structured event objects, JavaScript passes current and previous values directly:

```javascript
// General change event - receives all changed flags
client.on('change', (changedFlags) => {
  // changedFlags = { 'flag-1': { current: true, previous: false }, ... }
});

// Specific flag change - receives values only
client.on('change:flag-key', (currentValue, previousValue) => {
  // currentValue = true, previousValue = false
});
```

#### Implementation Details

**LDEmitter Implementation:**
```typescript
class LDEmitter {
  private _listeners: Map<EventName, Function[]> = new Map();

  on(name: EventName, listener: Function) {
    if (!this._listeners.has(name)) {
      this._listeners.set(name, [listener]);
    } else {
      this._listeners.get(name)?.push(listener);
    }
  }

  off(name: EventName, listener?: Function) {
    if (!listener) {
      // Remove all listeners for this event
      this._listeners.delete(name);
    } else {
      // Remove specific listener
      const updated = existingListeners.filter(fn => fn !== listener);
      this._listeners.set(name, updated);
    }
  }

  emit(name: EventName, ...detail: any[]) {
    this._listeners.get(name)?.forEach(listener => {
      try {
        listener(...detail);
      } catch (err) {
        this._logger?.error(`Error invoking handler: ${err}`);
      }
    });
  }
}
```

#### Key Insights for JavaScript
- **String-based event names**: Uses convention `'change:flag-key'` for specificity
- **Multiple signatures**: General vs specific flag listeners have different payloads
- **Error isolation**: Exceptions in one listener don't affect others
- **No structured events**: Values passed directly, not wrapped in event objects
- **Client-side oriented**: Designed for browser/React Native environments

---

### Python SDK Implementation

**Repository**: [launchdarkly/python-server-sdk](https://github.com/launchdarkly/python-server-sdk)  
**Interface Location**: `ldclient/interfaces.py`  
**Implementation**: `ldclient/impl/flag_tracker.py`  
**Tests**: `ldclient/testing/impl/test_flag_tracker.py`

#### Architecture Pattern: Callback Functions

The Python SDK uses simple callable functions as listeners:

```python
class FlagTracker:
    @abstractmethod
    def add_listener(self, listener: Callable[[FlagChange], None]):
        """Register a listener for all flag changes"""
        pass
    
    @abstractmethod
    def remove_listener(self, listener: Callable[[FlagChange], None]):
        """Unregister a listener"""
        pass
    
    @abstractmethod
    def add_flag_value_change_listener(
        self, 
        key: str, 
        context: Context, 
        listener: Callable[[FlagValueChange], None]
    ):
        """Register a listener for a specific flag's value changes"""
        pass
```

#### Event Structures

```python
class FlagChange:
    def __init__(self, key: str):
        self.__key = key
    
    @property
    def key(self) -> str:
        return self.__key

class FlagValueChange:
    def __init__(self, key, old_value, new_value):
        self.__key = key
        self.__old_value = old_value
        self.__new_value = new_value
    
    @property
    def key(self):
        return self.__key
    
    @property
    def old_value(self):
        return self.__old_value
    
    @property
    def new_value(self):
        return self.__new_value
```

#### Usage Pattern

```python
# General flag changes
def on_flag_changed(change):
    print(f"Flag changed: {change.key}")

client.flag_tracker.add_listener(on_flag_changed)

# Specific flag value changes
def on_value_changed(change):
    print(f"Flag {change.key} changed from {change.old_value} to {change.new_value}")

context = Context.create("user-key")
client.flag_tracker.add_flag_value_change_listener(
    "my-flag", 
    context, 
    on_value_changed
)
```

#### Test Example

From `test_flag_tracker.py`:

```python
def test_flag_change_listener_notified_when_value_changes():
    responses = ['initial', 'second', 'second', 'final']
    
    def eval_fn(key, context):
        return responses.pop(0)
    
    listeners = Listeners()
    tracker = FlagTrackerImpl(listeners, eval_fn)
    
    spy = SpyListener()
    tracker.add_flag_value_change_listener('flag-key', None, spy)
    
    listeners.notify(FlagChange('flag-key'))  # First change
    assert len(spy.statuses) == 1
    
    listeners.notify(FlagChange('flag-key'))  # No change (second -> second)
    assert len(spy.statuses) == 1  # Still 1
    
    listeners.notify(FlagChange('flag-key'))  # Third change
    assert len(spy.statuses) == 2
    
    assert spy.statuses[0].old_value == 'initial'
    assert spy.statuses[0].new_value == 'second'
    assert spy.statuses[1].old_value == 'second'
    assert spy.statuses[1].new_value == 'final'
```

#### Implementation Pattern: Listeners Broadcaster

The Python SDK uses an internal `Listeners` class that maintains a registry and broadcasts to all:

```python
class FlagTrackerImpl:
    def __init__(self, listeners: Listeners, eval_fn):
        self._listeners = listeners
        self._eval_fn = eval_fn
    
    def add_flag_value_change_listener(self, key, context, listener):
        # Create a wrapper that filters and re-evaluates
        current_value = self._eval_fn(key, context)
        
        def value_change_handler(change):
            nonlocal current_value
            if change.key != key:
                return
            new_value = self._eval_fn(key, context)
            if new_value != current_value:
                listener(FlagValueChange(key, current_value, new_value))
                current_value = new_value
        
        self._listeners.add(value_change_handler)
        return value_change_handler  # Return for later removal
```

#### Key Insights for Python
- **Simple callables**: Any callable accepting the event object
- **Property-based access**: Event objects use `@property` decorators
- **Closure-based filtering**: Value change listeners use closures to track state
- **Returns listener handle**: Can use returned value to unregister
- **No explicit threading**: Synchronous notification (thread safety handled elsewhere)

---

### Cross-SDK Comparison

| Aspect | Go | .NET | JavaScript | Python |
|--------|----|----|------------|--------|
| **Notification Mechanism** | Channels | Events | EventEmitter | Callbacks |
| **Registration Pattern** | Returns channel | Event subscription | `on(name, fn)` | `add_listener(fn)` |
| **Unregistration** | `Remove(channel)` | Event unsubscription | `off(name, fn)` | `remove_listener(fn)` |
| **Value Change API** | Separate method | Factory method | Same API, different name | Separate method |
| **Event Structure** | Struct | Struct | Function params | Class |
| **Threading Model** | Goroutines | Delegates (thread-safe) | Single-threaded | Thread-agnostic |
| **Error Handling** | Channels (non-blocking) | Sync (can throw) | Try-catch per listener | Sync (can throw) |
| **Type System** | Strongly typed | Strongly typed | TypeScript optional | Duck-typed |

### Universal Design Patterns

Despite different implementation approaches, all SDKs share:

1. **Two-tier listening**: General flag changes + specific value changes
2. **Immediate evaluation**: Value listeners evaluate on registration
3. **Change detection**: Only notify if value actually changed
4. **Context binding**: Value listeners bind to a specific evaluation context
5. **Filtering in SDK**: SDK internally filters general changes to specific flags
6. **Cleanup support**: All provide unregistration mechanisms

### Implications for Test Harness

The test harness design must accommodate:

1. **Language-agnostic callback mechanism**: Use HTTP callbacks (POST to URL)
2. **Flexible event payload**: JSON structure that works for all SDKs
3. **Async handling**: Some SDKs are async (Go channels, JS promises), others sync
4. **Context requirement**: All SDKs require context for value listeners (even global flags)
5. **Unregistration semantics**: Support listener cleanup testing
6. **Error isolation**: Test services should handle SDK exceptions gracefully

---

## Client-Side SDK Analysis

This section covers client-side SDK listener APIs and the plan for extending the test harness to cover them. SDKs surveyed: `js-core` (browser/React Native), `.NET` client SDK (`dotnet-core/pkgs/sdk/client`), and Android (`android-client-sdk`).

### Client-Side Listener APIs

#### JavaScript / React Native (`js-core`)

**Key files**: `packages/shared/sdk-client/src/LDClientImpl.ts`, `packages/shared/sdk-client/src/LDEmitter.ts`

API: event emitter pattern

```typescript
// All flags: fires with (context, flagKeys[]) whenever any flag's value changes
client.on('change', (context, flagKeys) => { ... });

// Specific flag: fires with (context) when that flag's value changes
client.on('change:flag-key', (context) => { ... });
```

**Characteristics:**
- Backed by `LDEmitter`, which is typed: event names are `'change'` or the template literal `` `change:${string}` ``
- The `_flagManager.on(...)` callback (in `LDClientImpl`) emits both events whenever flags change, including after `Identify` when new flag values arrive
- No explicit "unregister" method—listeners are removed with `client.off(eventName, listener)`
- No `FlagValueChangeEvent` with old/new values; the listener must call `variation()` to get the current value

#### .NET Client SDK (`dotnet-core`, `pkgs/sdk/client`)

**Key file**: `pkgs/sdk/client/src/Interfaces/IFlagTracker.cs`

API: C# event

```csharp
client.FlagTracker.FlagValueChanged += (sender, eventArgs) =>
{
    Console.WriteLine($"Flag '{eventArgs.Key}' changed from {eventArgs.OldValue} to {eventArgs.NewValue}");
};
```

**Characteristics:**
- A single `FlagValueChanged` event fires for **all** flags (no per-flag subscription at the SDK level)
- `FlagValueChangeEvent` includes `Key`, `OldValue`, `NewValue`, and `Deleted`
- Fires when `Identify` is called and any flag value changes for the new context
- Does **not** fire on initial flag load—only on subsequent changes
- Explicitly documented: does not fire when offline + Identify + stored data loaded (only fires for fresh data from LD)

#### Android SDK (`android-client-sdk`)

**Key files**:
- `FeatureFlagChangeListener.java` — per-flag callback (`onFeatureFlagChange(String flagKey)`)
- `LDAllFlagsListener.java` — all-flags callback (`onChange(List<String> flagKeys)`)
- `LDClientInterface.java` — `registerFeatureFlagListener(key, listener)` / `unregisterFeatureFlagListener(key, listener)`

Two distinct APIs:

```java
// Specific flag — listener receives only the key; must call variation() for new value
client.registerFeatureFlagListener("flag-key", flagKey -> { ... });

// All flags — listener receives the list of changed keys
client.registerListener(allFlagsListener); // LDAllFlagsListener
```

**Characteristics:**
- Per-flag listener: receives the flag key only (no old/new value in callback)
- All-flags listener: receives `List<String>` of changed flag keys
- Explicit register/unregister lifecycle (unlike JS `on`/`off`)
- Fires when the flag value changes for the current evaluation context

---

### Key Differences: Client-Side vs Server-Side

| Aspect | Server-Side | Client-Side |
|---|---|---|
| **Context** | Specified per listener call | Implicit: the client's current context |
| **Data format (streaming)** | Full `FeatureFlag` model JSON | Evaluated results only (`ClientSDKFlagWithKey`) |
| **Config vs value change** | Two listener types: general (config) + value | Value change only (client evaluates for one context) |
| **All-flags listener** | Single listener registered with empty `FlagKey` | SDK-specific API: `on('change')`, `LDAllFlagsListener`, etc. |
| **Old/new values in callback** | Yes (value listeners include `OldValue`/`NewValue`) | SDK-dependent: .NET yes; JS/Android no |
| **Identify** | N/A | May fire listeners when `Identify` switches context and flag values differ |
| **Targeting rules** | Evaluated server-side based on provided context | Pre-evaluated by LD; client receives the result |

---

### Can the Callback Architecture Work for Client-Side?

**Yes.** The HTTP callback architecture is test-harness-level infrastructure—it sits between the test harness and the SDK test service. The test service (not the test harness) adapts to each SDK's native listener API. For client-side SDKs, the test service would:

1. Register a `client.on('change:flag-key', ...)` (or platform-equivalent) listener internally
2. When the listener fires, POST a `ListenerNotification` JSON body to the callback URL supplied in the `registerFlagChangeListener` or `registerFlagValueChangeListener` command
3. The test harness receives the notification via `ListenerCallback.ExpectFlagChangeNotification()` or `ExpectValueChangeNotification()`

The command wire protocol (`registerFlagChangeListener`, `registerFlagValueChangeListener`, `unregisterListener`) and the callback payload (`ListenerNotification`) remain **identical** between server-side and client-side. Only the test service implementation changes.

**One important difference**: for client-side SDKs, the `Context` field in `RegisterFlagValueChangeListenerParams` is meaningless (the client has its own context). Test service implementers for client-side SDKs should ignore it. The test harness will either omit it or the test code should not send it for client-side tests.

---

### Streaming Data Format for Client-Side

Client-side SDKs receive a different streaming payload from server-side SDKs. Instead of a full `FeatureFlag` model, they receive evaluated results:

```json
// Server-side patch: full FeatureFlag JSON
{ "key": "flag-key", "version": 2, "on": false, "offVariation": 0, "variations": ["new-value", "other"], ... }

// Client-side patch: evaluated result only (ClientSDKFlagWithKey)
{ "key": "flag-key", "value": "new-value", "version": 2, "variation": 0 }
```

The existing `pushFlagUpdate` helper in listener tests uses `jsonhelpers.ToJSON(flag)` where `flag` is an `ldmodel.FeatureFlag`. For client-side tests, this must be replaced with `mockld.ClientSDKFlagWithKey` data.

Additionally, JS-based client-side SDKs connect to the **polling** endpoint for initial data, and then SSE for streaming updates. The `CommonStreamingTests.setupDataSystems` method already handles this pattern with its switch on `c.sdkKind`.

---

### Plan: Sharing Tests Between Server and Client Side

#### Approach: Refactor to `CommonListenerTests` Struct

To support both SDK kinds, we should convert the standalone functions in `common_tests_listeners.go` to use the `commonTestsBase` struct pattern (the same approach as `CommonStreamingTests`, `CommonEventTests`, etc.). This enables SDK-kind-aware data setup, context handling, and streaming updates within the same test functions.

The refactored type would look like:

```go
type CommonListenerTests struct {
    commonTestsBase
}

func NewCommonListenerTests(t *ldtest.T) CommonListenerTests {
    return CommonListenerTests{newCommonTestsBase(t, "CommonListenerTests")}
}
```

Key helpers that need SDK-kind-aware variants:

| Helper | Server-Side | Client-Side |
|---|---|---|
| `makeListenerFlag` | `ldmodel.FeatureFlag` | `mockld.ClientSDKFlagWithKey` |
| `createClientForListeners` | `NewServerSDKDataBuilder()` | `NewClientSDKDataBuilder()` + polling endpoint for JS SDKs |
| `pushFlagUpdate` | `jsonhelpers.ToJSON(featureFlag)` | `jsonhelpers.ToJSON(clientSDKFlagWithKey)` |
| Context in register params | Provided explicitly | Omitted or use client's initial context |

#### Tests That Can Be Shared (with SDK-kind-aware setup)

| Test | Shareable? | Notes |
|---|---|---|
| `flagValueChangeListenerReceivesNotification` | ✅ Yes | Adjust data format and omit context for client-side |
| `flagValueChangeListenerNoNotificationWhenUnchanged` | ✅ Yes | Same adjustments |
| `multipleValueListenersBothNotified` | ✅ Yes | Same adjustments |
| `flagChangeListenerReceivesNotification` | ✅ Yes | Works if SDKs support `FlagKey`-filtered general listeners |
| `flagChangeListenerFiltersByFlagKey` | ✅ Yes | Android natively filters per key; JS uses `change:key` |
| `flagChangeListenerEmptyKeyReceivesAllFlags` | ⚠️ Partial | API differs (JS: `on('change')`, Android: `LDAllFlagsListener`, .NET: single event always); may need SDK-kind-specific commands |
| `flagChangeListenerFiresOnConfigChange` | ❌ Server-side only | Client-side only receives evaluated results; "config change without value change" is invisible to client |
| `valueListenerIsContextSpecific` | ❌ Server-side only | Client-side has a single current context; cannot test per-context isolation this way |

#### Tests That Need Client-Side-Specific Variants

1. **Identify test** (already deferred from Phase 3): A test that calls `Identify` with a new context and verifies the listener fires when flag values differ for the new context. This is a client-side-only concern because server-side SDKs don't have an `Identify` operation.

#### Implementation Steps for Client-Side Support

The steps below are captured in the Implementation Plan under Phase 3B.

1. **Refactor `common_tests_listeners.go`** to use a `CommonListenerTests` struct embedding `commonTestsBase`
2. **Add SDK-kind-aware helpers**: `makeListenerFlagForSDK`, `createClientForListenerTests`, `pushListenerFlagUpdate` that branch on `c.isClientSide`
3. **Update `testsuite_entry_point.go`** to call `doCommonListenerTests` from `doAllClientSideTests` once the refactoring is complete
4. **Implement Identify test** for client-side (deferred)

---

## Proposed Test Harness Design

### Design Decision: Command Structure

#### Question: Separate Commands vs Unified Command?

Should the test harness use:
- **Option A**: Two separate commands (`registerFlagChangeListener`, `registerFlagValueChangeListener`)
- **Option B**: One unified command with a type parameter

#### Decision: ✅ Separate Commands

**Rationale:**

| **Separate Commands (Chosen)** | **Unified Command** |
|-------------------------------|---------------------|
| ✅ Type safety - required fields enforced at compile time | ❌ Runtime validation required |
| ✅ Matches SDK APIs directly | ❌ Doesn't match SDK method signatures |
| ✅ Simple implementation - direct command→SDK mapping | ❌ Requires branching/switching logic |
| ✅ Clear intent from command name | ❌ Must inspect type parameter to understand |
| ✅ Better validation errors at compile time | ❌ Runtime errors for missing Context |
| ✅ Follows existing pattern (hooks, migrations) | ❌ No precedent in test harness |
| ✅ Easier to test in isolation | ❌ Tests must handle multiple payload formats |
| ✅ Self-documenting code | ❌ Requires conditional documentation |
| ❌ More code (2 commands, 2 params) | ✅ Less code overall |
| ❌ Some duplication (ListenerID, CallbackURI) | ✅ No duplication |

**Key insight**: The two listener types have fundamentally different parameters (value-change requires Context and DefaultValue), which makes them poor candidates for unification. Separate commands provide type safety and clarity.

**Existing Pattern**: The hooks implementation (`testapi_hooks.go`, `mockld/hook_callback_service.go`) demonstrates the callback pattern we're following.

### 1. New Capability Flag

Add to `docs/service_spec.md`:

```markdown
#### Capability `"flag-change-listeners"`

This means that the SDK supports subscribing to flag change notifications. The SDK should 
provide a mechanism to register listeners/callbacks that are invoked when flag changes occur.

For SDKs that support both general flag change events and value change events, the test 
harness will test both types:
- **General flag change listeners**: Test that the SDK notifies when any flag configuration changes
- **Value change listeners**: Test that the SDK notifies when a flag's evaluated value changes 
  for a specific context

Both types of listeners are important for complete SDK coverage and ensuring the SDK correctly 
implements the flag change notification contract.
```

### 2. New Service Commands

Add to `servicedef/command_params.go`:

#### Command: Register Flag Change Listener

For general flag configuration changes (doesn't track evaluated values):

```go
const CommandRegisterFlagChangeListener = "registerFlagChangeListener"

type RegisterFlagChangeListenerParams struct {
    ListenerID  string `json:"listenerId"`  // Unique identifier
    FlagKey     string `json:"flagKey"`     // Flag to listen to (empty = all flags)
    CallbackURI string `json:"callbackUri"` // Where to POST events
}
```

**Behavior:**
- Test service registers a flag-change listener using SDK's native API (e.g., `AddFlagChangeListener`)
- Stores mapping of `listenerId` → listener handle + callbackUri
- When SDK detects flag configuration change, test service POSTs to callbackUri
- If `flagKey` is empty, listens to all flag changes; if specified, filters to that flag

#### Command: Register Flag Value Change Listener

For value changes of a specific flag for a specific context:

```go
const CommandRegisterFlagValueChangeListener = "registerFlagValueChangeListener"

type RegisterFlagValueChangeListenerParams struct {
    ListenerID   string            `json:"listenerId"`   // Unique identifier
    FlagKey      string            `json:"flagKey"`      // Flag to listen to (required)
    Context      ldcontext.Context `json:"context"`      // Evaluation context (required)
    DefaultValue ldvalue.Value     `json:"defaultValue"` // Default if flag missing (required)
    CallbackURI  string            `json:"callbackUri"`  // Where to POST events
}
```

**Behavior:**
- Test service registers a value-change listener using SDK's native API (e.g., `AddFlagValueChangeListener`)
- SDK immediately evaluates flag for the given context to capture initial value
- Stores mapping of `listenerId` → listener handle + callbackUri + metadata
- When SDK detects flag value changed for this context, test service POSTs to callbackUri
- Only notifies if the evaluated value actually changed (not just configuration)

#### Command: Unregister Listener

Single unregister command works for both listener types:

```go
const CommandUnregisterListener = "unregisterListener"

type UnregisterListenerParams struct {
    ListenerID string `json:"listenerId"` // ID from registration
}
```

**Behavior:**
- Look up listener by ID (works for either type)
- Unregister using SDK's native API
- Remove from internal mapping

### 3. Callback Payload Structures

#### Payload: Flag Change Notification

When a flag configuration changes, test service POSTs to the callbackUri:

```json
{
  "listenerId": "listener-1",
  "flagKey": "example-flag-key",
  "timestamp": 1707753600000
}
```

**Fields:**
- `listenerId` - matches the ID from registration
- `flagKey` - the flag whose configuration changed
- `timestamp` - milliseconds since epoch

**Note**: This notification indicates configuration changed, not that any specific value changed.

#### Payload: Flag Value Change Notification

When a flag's evaluated value changes for a specific context, test service POSTs to the callbackUri:

```json
{
  "listenerId": "listener-2",
  "flagKey": "example-flag-key",
  "oldValue": {
    "type": "boolean",
    "value": false
  },
  "newValue": {
    "type": "boolean", 
    "value": true
  },
  "timestamp": 1707753600000
}
```

**Fields:**
- `listenerId` - matches the ID from registration
- `flagKey` - the flag that changed
- `oldValue` - previous evaluated value (typed JSON, or null if unknown)
- `newValue` - new evaluated value (typed JSON)
- `timestamp` - milliseconds since epoch

**Note**: This notification only occurs when the evaluated value actually changed for the registered context.

### 4. Test Service Implementation Requirements

Each SDK's test service must:

1. **Maintain Listener Registry**
   - Map: `listenerId` → `{listenerType, sdkListenerHandle, callbackUri, metadata}`
   - `listenerType`: "flag-change" or "value-change"
   - `metadata`: Varies by type (flagKey, context, defaultValue as applicable)

2. **On RegisterFlagChangeListener**:
   - Use SDK's native flag-change listener API
   - Store callbackUri and listener metadata
   - When SDK fires event, POST flag-change notification to callbackUri

3. **On RegisterFlagValueChangeListener**:
   - Use SDK's native value-change listener API
   - Store callbackUri, context, and listener metadata  
   - SDK evaluates immediately (some SDKs provide initial value)
   - When SDK fires event with old/new values, POST value-change notification to callbackUri

4. **On Listener Invocation**:
   - **Flag-change listener**: Construct notification with listenerId, flagKey, timestamp
   - **Value-change listener**: Construct notification with listenerId, flagKey, oldValue, newValue, timestamp
   - POST to callbackUri (non-blocking, ideally in goroutine/async)
   - Handle HTTP errors gracefully (log, don't crash)

5. **On UnregisterListener**:
   - Look up by listenerId (type doesn't matter)
   - Call SDK's unregister API with stored handle
   - Remove from registry

### 5. Test Harness Components

**Reference Implementation**: See `sdktests/testapi_hooks.go` and `mockld/hook_callback_service.go` for the canonical callback pattern.

#### New Test API: `sdktests/testapi_listeners.go`

```go
// Unified notification type that can represent either listener type
type ListenerNotification struct {
    ListenerID string
    FlagKey    string
    OldValue   o.Maybe[ldvalue.Value] // Present only for value-change notifications
    NewValue   o.Maybe[ldvalue.Value] // Present only for value-change notifications
    Timestamp  int64
}

type ListenerCallback struct {
    endpoint      *harness.MockEndpoint
    notifications chan ListenerNotification
}

// NewListenerCallback creates a callback endpoint and starts collecting notifications
// Pattern follows mockld.HookCallbackService
func NewListenerCallback(t *ldtest.T) *ListenerCallback

// AwaitNotification waits for any notification with timeout
func (l *ListenerCallback) AwaitNotification(timeout time.Duration) (ListenerNotification, bool)

// ExpectFlagChangeNotification asserts a flag-change notification is received
func (l *ListenerCallback) ExpectFlagChangeNotification(
    t *ldtest.T,
    expectedKey string,
    timeout time.Duration,
)

// ExpectValueChangeNotification asserts a value-change notification is received
func (l *ListenerCallback) ExpectValueChangeNotification(
    t *ldtest.T,
    expectedKey string,
    expectedNewValue ldvalue.Value,
    timeout time.Duration,
)

// ExpectNoNotification asserts no notification is received within timeout
func (l *ListenerCallback) ExpectNoNotification(t *ldtest.T, timeout time.Duration)
```

#### SDK Client Methods: `sdktests/testapi_sdk_client.go`

```go
// RegisterFlagChangeListener tells the SDK to register a general flag-change listener
func (c *SDKClient) RegisterFlagChangeListener(
    t *ldtest.T,
    listenerId string,
    flagKey string,  // empty string = listen to all flags
    callback *ListenerCallback,
) error

// RegisterFlagValueChangeListener tells the SDK to register a value-change listener
func (c *SDKClient) RegisterFlagValueChangeListener(
    t *ldtest.T,
    listenerId string,
    flagKey string,
    context ldcontext.Context,
    defaultValue ldvalue.Value,
    callback *ListenerCallback,
) error

// UnregisterListener tells the SDK to remove a listener (works for both types)
func (c *SDKClient) UnregisterListener(t *ldtest.T, listenerId string) error
```

### 6. Test Scenarios

Create new test file: `sdktests/common_tests_listeners.go`

#### Server-Side Tests

```go
type CommonListenerTests struct {
    commonTestsBase
}

func NewCommonListenerTests(t *ldtest.T, testName string, baseSDKConfigurers ...SDKConfigurer) CommonListenerTests

func (c CommonListenerTests) Run(t *ldtest.T) {
    // General flag-change listener tests
    t.Run("flag change listener receives config change", c.flagChangeListenerReceivesConfigChange)
    t.Run("flag change listener receives all flag changes", c.flagChangeListenerReceivesAllFlags)
    t.Run("flag change listener with specific key", c.flagChangeListenerWithSpecificKey)
    
    // Value-change listener tests
    t.Run("value listener receives flag value change", c.valueListenerReceivesValueChange)
    t.Run("value listener receives correct old and new values", c.valueListenerReceivesCorrectValues)
    t.Run("value listener not called when value unchanged", c.valueListenerNotCalledWhenUnchanged)
    t.Run("multiple value listeners all notified", c.multipleValueListenersNotified)
    t.Run("unregistered listener stops receiving events", c.unregisteredListenerStops)
    t.Run("value listener for different context not notified", c.valueListenerForDifferentContext)
    t.Run("value listener receives changes from prerequisites", c.valueListenerReceivesPrerequisiteChanges)
    t.Run("value listener works with flag deletion", c.valueListenerWorksWithDeletion)
}
```

**Test: Flag Change Listener - Config Change**
```go
func (c CommonListenerTests) flagChangeListenerReceivesConfigChange(t *ldtest.T) {
    // Setup: Create flag with initial config
    dataSystem, configurers := c.setupDataSystems(t, 
        c.makeSDKDataWithFlag(1, ldvalue.String("value")))
    
    client := NewSDKClient(t, configurers...)
    callback := NewListenerCallback(t)
    
    // Register general flag-change listener
    client.RegisterFlagChangeListener(t, "listener-1", "flag-key", callback)
    
    // Act: Push flag config update (even if value stays same)
    dataSystem.PushUpdate("flag", "flag-key", 2, 
        c.makeFlagData("flag-key", 2, ldvalue.String("value"))) // same value
    
    // Assert: Flag-change notification received
    callback.ExpectFlagChangeNotification(t, "flag-key", time.Second)
}
```

**Test: Flag Change Listener - All Flags**
```go
func (c CommonListenerTests) flagChangeListenerReceivesAllFlags(t *ldtest.T) {
    dataSystem, configurers := c.setupDataSystems(t, 
        c.makeSDKDataWithFlags(map[string]ldvalue.Value{
            "flag-1": ldvalue.Bool(true),
            "flag-2": ldvalue.String("a"),
        }))
    
    client := NewSDKClient(t, configurers...)
    callback := NewListenerCallback(t)
    
    // Register listener for all flags (empty flagKey)
    client.RegisterFlagChangeListener(t, "listener-1", "", callback)
    
    // Update flag-1
    dataSystem.PushUpdate("flag", "flag-1", 2, 
        c.makeFlagData("flag-1", 2, ldvalue.Bool(false)))
    callback.ExpectFlagChangeNotification(t, "flag-1", time.Second)
    
    // Update flag-2
    dataSystem.PushUpdate("flag", "flag-2", 2,
        c.makeFlagData("flag-2", 2, ldvalue.String("b")))
    callback.ExpectFlagChangeNotification(t, "flag-2", time.Second)
}
```

**Test: Value Change Listener - Basic**
```go
func (c CommonListenerTests) valueListenerReceivesValueChange(t *ldtest.T) {
    // Setup: Create flag with initial value
    dataSystem, configurers := c.setupDataSystems(t, 
        c.makeSDKDataWithFlag(1, ldvalue.String("initial")))
    
    client := NewSDKClient(t, configurers...)
    callback := NewListenerCallback(t)
    
    // Register value-change listener
    client.RegisterFlagValueChangeListener(t, "listener-1", "flag-key", 
        testContext, ldvalue.String("default"), callback)
    
    // Act: Push flag update
    dataSystem.PushUpdate("flag", "flag-key", 2, 
        c.makeFlagData("flag-key", 2, ldvalue.String("updated")))
    
    // Assert: Value-change notification received with old/new values
    callback.ExpectValueChangeNotification(t, "flag-key", 
        ldvalue.String("updated"), time.Second)
}
```

**Test: Value Listener - No Notification When Value Unchanged**
```go
func (c CommonListenerTests) valueListenerNotCalledWhenUnchanged(t *ldtest.T) {
    // Setup with flag value "stable"
    dataSystem, configurers := c.setupDataSystems(t, 
        c.makeSDKDataWithFlag(1, ldvalue.String("stable")))
    
    client := NewSDKClient(t, configurers...)
    callback := NewListenerCallback(t)
    client.RegisterFlagValueChangeListener(t, "listener-1", "flag-key",
        testContext, ldvalue.String("default"), callback)
    
    // Update flag config but keep value same
    dataSystem.PushUpdate("flag", "flag-key", 2,
        c.makeFlagData("flag-key", 2, ldvalue.String("stable")))
    
    // Assert: No notification received (value didn't change)
    callback.ExpectNoNotification(t, time.Millisecond * 100)
}
```

**Test: Multiple Value Listeners**
```go
func (c CommonListenerTests) multipleValueListenersNotified(t *ldtest.T) {
    dataSystem, configurers := c.setupDataSystems(t,
        c.makeSDKDataWithFlag(1, ldvalue.Bool(false)))
    
    client := NewSDKClient(t, configurers...)
    callback1 := NewListenerCallback(t)
    callback2 := NewListenerCallback(t)
    
    client.RegisterFlagValueChangeListener(t, "listener-1", "flag-key",
        testContext, ldvalue.Bool(false), callback1)
    client.RegisterFlagValueChangeListener(t, "listener-2", "flag-key",
        testContext, ldvalue.Bool(false), callback2)
    
    dataSystem.PushUpdate("flag", "flag-key", 2,
        c.makeFlagData("flag-key", 2, ldvalue.Bool(true)))
    
    // Both listeners should receive notification
    callback1.ExpectValueChangeNotification(t, "flag-key", ldvalue.Bool(true), time.Second)
    callback2.ExpectValueChangeNotification(t, "flag-key", ldvalue.Bool(true), time.Second)
}
}
```

**Test: Unregister Stops Notifications**
```go
func (c CommonListenerTests) unregisteredListenerStops(t *ldtest.T) {
    dataSystem, configurers := c.setupDataSystems(t,
        c.makeSDKDataWithFlag(1, ldvalue.String("v1")))
    
    client := NewSDKClient(t, configurers...)
    callback := NewListenerCallback(t)
    
    client.RegisterFlagValueChangeListener(t, "listener-1", "flag-key",
        testContext, ldvalue.String("default"), callback)
    
    // First change - should receive
    dataSystem.PushUpdate("flag", "flag-key", 2,
        c.makeFlagData("flag-key", 2, ldvalue.String("v2")))
    callback.AwaitNotification(time.Second)
    
    // Unregister
    client.UnregisterListener(t, "listener-1")
    
    // Second change - should NOT receive
    dataSystem.PushUpdate("flag", "flag-key", 3,
        c.makeFlagData("flag-key", 3, ldvalue.String("v3")))
    callback.ExpectNoNotification(t, time.Millisecond * 100)
}
```

**Test: Context-Specific Value Listeners**
```go
func (c CommonListenerTests) valueListenerForDifferentContext(t *ldtest.T) {
    context1 := ldcontext.New("user-1")
    context2 := ldcontext.New("user-2")
    
    // Setup flag that targets user-1 specifically
    dataSystem, configurers := c.setupDataSystems(t,
        makeFlagTargetingUser("flag-key", "user-1", ldvalue.Bool(true), ldvalue.Bool(false)))
    
    client := NewSDKClient(t, configurers...)
    callback1 := NewListenerCallback(t)
    callback2 := NewListenerCallback(t)
    
    // Register listeners for different contexts
    client.RegisterFlagValueChangeListener(t, "listener-1", "flag-key",
        context1, ldvalue.Bool(false), callback1)
    client.RegisterFlagValueChangeListener(t, "listener-2", "flag-key",
        context2, ldvalue.Bool(false), callback2)
    
    // Change flag to target user-2 instead
    dataSystem.PushUpdate("flag", "flag-key", 2,
        makeFlagTargetingUser("flag-key", "user-2", ldvalue.Bool(true), ldvalue.Bool(false)))
    
    // Only listener-2 should be notified (value changed for user-2)
    callback2.ExpectValueChangeNotification(t, "flag-key", ldvalue.Bool(true), time.Second)
    callback1.ExpectNoNotification(t, time.Millisecond * 100)
}
```

#### Client-Side Tests

For client-side SDKs, additional scenarios:

```go
func (c CommonListenerTests) listenerNotifiedOnIdentify(t *ldtest.T) {
    // Register listener, then call identify() with new context
    // Verify listener notified if flag value changed for new context
}

func (c CommonListenerTests) listenerNotifiedOnForeground(t *ldtest.T) {
    // Mobile SDK: verify listener notified after app foregrounds
}
```

#### Edge Cases / Error Conditions

```go
func (c CommonListenerTests) listenerWorksWithDeletion(t *ldtest.T) {
    // Verify listener receives default value when flag deleted
}

func (c CommonListenerTests) listenerWorksAfterReconnect(t *ldtest.T) {
    // Simulate disconnect/reconnect, verify listener still works
}

func (c CommonListenerTests) rapidFlagChanges(t *ldtest.T) {
    // Multiple rapid changes, verify all are delivered
}
```

---

## Implementation Plan

### Phase 1: Foundation

**Tasks:**
1. Add `"flag-change-listeners"` capability to service spec:
   - Add `CapabilityFlagChangeListeners = "flag-change-listeners"` constant to `servicedef/service_params.go` in the test harness
   - Update `docs/service_spec.md` with capability description
2. Define command structures in `servicedef/command_params.go`:
   - `RegisterFlagChangeListenerParams`
   - `RegisterFlagValueChangeListenerParams`
   - `UnregisterListenerParams`
3. Create callback payload JSON schema documentation
4. Define `ListenerNotification` structure for callback responses

**Deliverables:**
- Updated service spec document with `flag-change-listeners` capability
- Go type definitions for commands and callback payloads
- Callback endpoint specification

### Phase 2: Test Harness Infrastructure

**Tasks:**
1. Implement `ListenerCallback` in `sdktests/testapi_listeners.go`
   - Mock HTTP endpoint for receiving callbacks
   - Channel-based notification collection
   - Assertion helpers
2. Add client methods to `SDKClient`
   - `RegisterListener()`
   - `UnregisterListener()`
3. Create test helpers in `helpers.go`

**Deliverables:**
- Working listener callback infrastructure
- Client API for listener management
- Helper functions for common patterns

### Phase 3: Test Implementation (Server-Side) ✅ Complete

**Tasks:**
1. Create `common_tests_listeners.go` with standalone test functions ✅
2. Implement core test scenarios: ✅
   - `flagChangeListenerReceivesNotification`
   - `flagChangeListenerFiresOnConfigChange`
   - `flagChangeListenerFiltersByFlagKey`
   - `flagChangeListenerEmptyKeyReceivesAllFlags`
   - `flagValueChangeListenerReceivesNotification`
   - `flagValueChangeListenerNoNotificationWhenUnchanged`
   - `multipleValueListenersBothNotified`
   - `valueListenerIsContextSpecific`
3. Register `doCommonListenerTests` in `doAllServerSideTests` ✅

**Status**: Complete and verified against Go Server SDK.

### Phase 3B: Test Implementation (Client-Side)

**Goal**: Extend existing listener tests to also run against client-side SDKs, where the test logic is similar but data setup, streaming format, and context handling differ.

**Tasks:**
1. **Refactor `common_tests_listeners.go`** to use a `CommonListenerTests` struct embedding `commonTestsBase` (same pattern as `CommonStreamingTests`)
2. **Add SDK-kind-aware data helpers**:
   - `makeListenerFlagForSDK(key, version, value)` — returns `ServerSDKData` or `ClientSDKData`
   - `createClientForListenerTests(t)` — handles polling endpoint for JS SDKs
   - `pushListenerFlagUpdate(dataSystem, key, version, value)` — sends `ClientSDKFlagWithKey` for client-side, `FeatureFlag` for server-side
3. **Mark server-side-only tests** with `t.RequireCapability(servicedef.CapabilityServerSide)` or move to a separate `doServerSideListenerTests` group
4. **Add Identify test** for client-side: verifies that a listener fires when `Identify` changes the context and flag values differ for the new context
5. **Register `doCommonListenerTests` in `doAllClientSideTests`** once refactoring is complete

**Tests that require refactoring to work for client-side:**

| Test | Action needed |
|---|---|
| `flagValueChangeListenerReceivesNotification` | Omit context from register params for client-side; use client-side data format |
| `flagValueChangeListenerNoNotificationWhenUnchanged` | Same |
| `multipleValueListenersBothNotified` | Same |
| `flagChangeListenerReceivesNotification` | Same; test service uses `on('change:key')` or equivalent |
| `flagChangeListenerFiltersByFlagKey` | Same |
| `flagChangeListenerEmptyKeyReceivesAllFlags` | Same; test service uses `on('change')` or `LDAllFlagsListener` |
| `flagChangeListenerFiresOnConfigChange` | Server-side only (client-side sees evaluated results, not config) |
| `valueListenerIsContextSpecific` | Server-side only (client-side has one context) |

**New tests for client-side only:**
- `listenerFiresOnIdentify`: call `Identify` with a new context; verify value change listener fires when the flag value differs for the new context

### Phase 4: SDK Implementation (Ongoing)

**Per SDK:**
1. Add capability flag to test service:
   - Update the SDK's test service (e.g., `go-server-sdk/testservice/service.go`) to include `servicedef.CapabilityFlagChangeListeners` in its capabilities array
   - This tells the test harness that this SDK supports flag change listener tests
2. Implement command handlers in the test service:
   - Handle `registerFlagChangeListener` command
   - Handle `registerFlagValueChangeListener` command
   - Handle `unregisterListener` command
3. Wire up SDK listener APIs:
   - Use SDK's native listener API (e.g., `FlagTracker.AddFlagChangeListener()` for Go)
   - Create HTTP client to POST callbacks to the provided `callbackUri`
   - Manage listener lifecycle (registration, notification, cleanup)
4. Test against harness:
   - Run test harness against the SDK test service
   - Verify all listener tests pass
5. Document any SDK-specific quirks in the test service implementation

**Using Test Suppressions During Development:**

Each SDK repository contains a `testharness-suppressions.txt` file that lists test cases to skip. This can be used during listener implementation for:

1. **Feature Not Supported**: If an SDK doesn't support flag change listeners at all, or doesn't support specific listener types (general vs value change)
2. **Partial Implementation**: During incremental development, suppress tests for features not yet implemented in the test service
3. **SDK-Specific Limitations**: Document and suppress tests for edge cases that don't apply to a particular SDK

**Example workflow:**
- Start Phase 4 with all listener tests in `testharness-suppressions.txt`
- Implement basic listener support
- Remove suppressions incrementally as features are completed
- Final state: Only permanent suppressions (if any) remain for unsupported features

**Priority Order:**

Priority should be based on SDK usage metrics to maximize impact. Suggested approach:
1. Gather usage data from LaunchDarkly's internal metrics or customer surveys
2. Prioritize the most commonly used server-side and client-side SDKs
3. Consider implementation diversity (ensure we test different language patterns)

**Potential candidates for early implementation:**
- Go (reference implementation - already analyzed in this document)
- Node.js (popular, async model)
- Java (enterprise usage, different listener pattern)
- Python (popular, simple callback model)
- .NET (enterprise usage, event-based pattern)
- JavaScript/Browser (client-side, event emitter pattern)

**Note**: Actual priority order should be determined based on customer usage data before beginning Phase 4.

### Phase 5: Documentation & Refinement

**Tasks:**
1. Update test harness README
2. Create implementation guide for SDK test services
3. Document common pitfalls
4. Add troubleshooting guide
5. Review and incorporate feedback

---

## Open Questions

### Design Decisions Made

1. **General vs Value Listeners**
   - ✅ **DECIDED** - Test harness will support testing both general flag change listeners and value change listeners

2. **Command Structure**
   - ✅ **DECIDED** - Use separate commands (`registerFlagChangeListener`, `registerFlagValueChangeListener`) rather than unified command with type parameter
   - **Rationale**: Type safety, matches SDK APIs, simpler implementation
   - See "Design Decision: Command Structure" section above for full analysis

3. **Initial Value Handling**
   - ✅ **DECIDED** - Tests will NOT expect an initial value notification upon listener registration
   - **Rationale**: Go, Python, and .NET SDKs do not send initial notifications for value-change listeners. Only JavaScript sends initial change events, but during initialization (`identify()`), not listener registration.
   - **Impact**: No new capability needed, no conditional test logic required
   - See detailed analysis in "Design Decisions Needed" section below

4. **Async Callback Handling**
   - ✅ **DECIDED** - Use 5-second timeout for listener callbacks (matching existing hook/event timeout pattern)
   - **Rationale**: Consistent with test harness standards. SDKs don't batch listener notifications.
   - **Impact**: Use `time.Second * 5` for positive assertions, shorter timeouts (100ms-1s) for negative assertions
   - See detailed analysis in "Design Decisions Needed" section below

5. **Error Cases**
   - ✅ **DECIDED** - Follow the hooks pattern: test service supports optional error parameter to simulate callback failures
   - **Rationale**: Consistent with how hooks tests verify error handling. SDKs must handle callback errors gracefully without crashing.
   - **Impact**: Add optional error parameter to register commands, test that SDK continues working when callbacks fail
   - See detailed analysis in "Design Decisions Needed" section below

6. **Thread Safety / Concurrent Access**
   - ✅ **DECIDED** - Test multiple listeners receiving notifications concurrently using `sync.WaitGroup` pattern
   - **Rationale**: Follows established patterns from hooks and client independence tests. Focus on correctness, not implementation.
   - **Impact**: Use goroutines + WaitGroup in tests with multiple listeners
   - See detailed analysis in "Design Decisions Needed" section below

### Technical Questions - All Decided

7. **Callback HTTP Semantics**
   - ✅ **DECIDED** - Use POST method, 200/400/500 response codes, no retry logic (same as hooks)
   - **Rationale**: "For simplicity, and to make it clear that these calls should never be cached, the method is always POST"
   - **Impact**: SDK test service POSTs to callback URI once, logs failures, no retries
   - See detailed analysis in "Design Decisions Needed" section below

8. **Listener ID Format**
   - ✅ **DECIDED** - Arbitrary string chosen by test, no enforced format (same as hook names)
   - **Rationale**: Consistent with hook naming. Test author chooses descriptive names.
   - **Impact**: Use descriptive IDs like "listener-1", append numeric suffix for multiples
   - See detailed analysis in "Design Decisions Needed" section below

9. **Cleanup / Lifecycle**
   - ✅ **DECIDED** - Automatic cleanup on client close + explicit unregister command (same as hooks)
   - **Rationale**: "The object's lifecycle is tied to the test scope that created it; it will be automatically closed"
   - **Impact**: Callback service has `Close()` method called via `defer`, SDK cleans up listeners on close
   - See detailed analysis in "Design Decisions Needed" section below

### Design Decisions Needed

1. **Initial Value Handling**
   
   **Question**: Should listeners receive an initial notification immediately upon registration, or only when the flag subsequently changes?
   
   **SDK Behavior Analysis**:
   
   | SDK | Sends Initial Value? | Evidence |
   |-----|---------------------|----------|
   | **Go** | ❌ No | `flag_tracker_impl.go`: Evaluates current value on registration but only sends events on subsequent changes. Test confirms: `th.AssertNoMoreValues(t, ch1, timeout)` after registration. |
   | **Python** | ❌ No | `test_flag_tracker.py`: After `add_flag_value_change_listener()`, test asserts `len(spy.statuses) == 0` |
   | **JavaScript** | ⚠️ Partial | Emits all flags as changed during `identify()` (initialization), but this is a general change event, not specific to individual value change listeners |
   | **.NET** | ❌ No (inferred) | Similar pattern to Go/Python - monitors for changes, not initial state |
   
   **Test Impact Analysis**:
   
   - **No new capability needed**: This is not a distinct SDK feature, just different implementation behavior
   - **Test approach**: Tests should NOT expect or require an initial value notification
   - **Rationale**: 
     - Most SDKs (Go, Python, .NET) do not send initial values for value-change listeners
     - The initial value can be obtained by simply evaluating the flag
     - Value-change listeners are intended to react to *changes*, not initial state
     - JavaScript's behavior is different (emits on `identify()`), but this is about initialization events, not listener registration
   
   **Decision**: ✅ **DECIDED**
   - Tests will NOT expect an initial value notification upon listener registration
   - Tests will only verify notifications occur when flags subsequently change
   - No conditional logic or capability flags needed
   - This simplifies test implementation and aligns with majority SDK behavior

2. **Async Callback Handling**
   
   **Question 1**: How long should tests wait for callbacks?
   
   **Existing Test Harness Patterns**:
   
   | Callback Type | Timeout Value | Location |
   |--------------|---------------|----------|
   | **Hook callbacks** | 5 seconds | `sdktests/testapi_hooks.go`: `const hookReceiveTimeout = time.Second * 5` |
   | **Event payloads** | 5 seconds | `sdktests/common_tests_events_base.go`: `const defaultEventTimeout = time.Second * 5` |
   | **Stream connections** | 2 seconds (positive case) / 100ms (negative case) | `sdktests/server_side_stream_retry.go` |
   
   **Pattern**: The test harness consistently uses **5 seconds** as the standard timeout for async operations like callbacks and events. Shorter timeouts (100ms-2s) are used only for negative assertions ("expect no more X").
   
   **Question 2**: Do SDKs batch notifications?
   
   **SDK Behavior Analysis**:
   
   - **Go SDK**: Uses buffered channels with buffer size of 10 (`internal/broadcasters.go`: `const subscriberChannelBufferLength = 10`)
     - Events are sent individually: `ch.sendCh <- value` in `Broadcast()`
     - Buffer prevents blocking but doesn't batch - just queues up to 10 individual events
     - Each flag change generates a separate event
   
   - **Other SDKs**: No evidence of batching listener notifications
     - Event emitters (JavaScript), delegates (.NET), and callbacks (Python) all fire immediately per change
     - Batching would complicate the listener API and isn't needed for flag changes (unlike analytics events)
   
   **Conclusion**: SDKs do NOT batch flag change notifications. Each flag change generates an individual, immediate notification.
   
   **Decision**: ✅ **DECIDED**
   - Use **5 seconds** as the standard timeout for listener callbacks (consistent with hooks and events)
   - No special handling for "batched" notifications needed - expect one callback per flag change
   - Use shorter timeout (e.g., 100ms-1s) for negative assertions like "expect no notification"
   - Pattern already exists in test harness: `helpers.TryReceive(channel, hookReceiveTimeout)`

3. **Error Cases**
   
   **Question**: Should we test listener exceptions/errors? What if callback URL is unreachable?
   
   **Existing Test Harness Pattern (from Hooks)**:
   
   The test harness already has a well-established pattern for testing error handling in callbacks, used in the hooks tests (`common_tests_hooks.go`):
   
   **Error Configuration Pattern**:
   ```go
   // From servicedef/sdk_config.go
   type SDKConfigHookInstance struct {
       Name        string
       CallbackURI string
       Data        map[HookStage]SDKConfigEvaluationHookData
       Errors      map[HookStage]o.Maybe[string]  // <-- Error configuration
   }
   ```
   
   **Test Example** (`errorInBeforeStageDoesNotAffectAfterStage`):
   ```go
   // Configure hook to throw error at specific stage
   createClientForHooksWithErrors(t, names, hookData, 
       map[servicedef.HookStage]o.Maybe[string]{
           servicedef.BeforeEvaluation: o.Some("something is rotten in the state of Denmark!"),
       }, configurers...)
   
   // Test verifies:
   // 1. Error doesn't crash the SDK
   // 2. Subsequent stages still execute
   // 3. Error data is not propagated (data from failed stage is empty)
   ```
   
   **Key Principle** (from test comment):
   > "The client MUST handle exceptions which are thrown (or errors returned, if idiomatic for the language) during the execution of a stage or handler allowing operations to complete unaffected."
   
   **Decision**: ✅ **DECIDED** - Follow the hooks pattern
   - **Test Approach**: 
     - Test service should support an optional `error` parameter in register listener commands
     - When set, the test service makes the listener callback fail (throws exception or returns error)
     - Tests verify: SDK doesn't crash, other listeners still receive notifications, listener can be unregistered
   - **Callback Unreachable**: Test this by providing an invalid callback URI (e.g., `http://localhost:1/invalid`)
     - SDK test service should log the error but not crash
     - Other listeners should continue working
   - **No retry logic**: SDKs should not retry failed callback POSTs (same as hooks)

4. **Thread Safety / Concurrent Access**
   
   **Question**: Should we test concurrent listener registrations? Thread-safety of listener invocation?
   
   **Existing Test Harness Pattern (from Client Independence)**:
   
   The test harness uses `sync.WaitGroup` for coordinating multiple concurrent operations (`client_side_client_independence.go`):
   
   ```go
   // Example: Two clients operating concurrently
   w := sync.WaitGroup{}
   w.Add(2)
   go func() {
       defer w.Done()
       _ = eventsA.ExpectAnalyticsEvents(t, defaultEventTimeout)
   }()
   go func() {
       defer w.Done()
       _ = eventsB.ExpectAnalyticsEvents(t, defaultEventTimeout)
   }()
   w.Wait()
   ```
   
   **Pattern for Multiple Listeners** (from Hooks):
   ```go
   // ExpectAtLeastOneCallForEachHook waits for a single call from N hooks
   func (h *Hooks) ExpectAtLeastOneCallForEachHook(t *ldtest.T, hookNames []string) {
       out := make(chan o.Maybe[servicedef.HookExecutionPayload])
       
       for _, hookName := range hookNames {
           go func(name string) {
               out <- helpers.TryReceive(h.instances[name].hookService.CallChannel, hookReceiveTimeout)
           }(hookName)
       }
       
       // Collect all results
       var payloads []servicedef.HookExecutionPayload
       for i := 0; i < totalCalls; i++ {
           if val := <-out; val.IsDefined() {
               payloads = append(payloads, val.Value())
           }
       }
   }
   ```
   
   **Decision**: ✅ **DECIDED** - Follow established concurrency patterns
   - **Multiple Listeners**: Register multiple listeners on same flag (already in proposed tests: `multipleValueListenersNotified`)
     - Use goroutines + WaitGroup to collect notifications concurrently
     - Verify all listeners receive notifications
   - **Concurrent Registration**: Not explicitly tested (similar to how hooks doesn't test concurrent hook registration)
     - SDKs internally handle thread-safety of listener management
     - Focus tests on correctness of notifications, not thread-safety implementation details
   - **Pattern**: Use `sync.WaitGroup` when coordinating multiple concurrent operations in tests

### Technical Questions

1. **Callback HTTP Semantics**
   
   **Question**: What HTTP method, response codes, and retry behavior should we use?
   
   **Existing Test Harness Pattern (from Hook Callbacks)**:
   
   From `mockld/hook_callback_service.go` and `mockld/callback_helpers.go`:
   
   ```go
   // Hook callback endpoint handler
   endpointHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
       bytes, err := io.ReadAll(req.Body)
       logger.Printf("Received from hook: %s", string(bytes))
       if err != nil {
           logger.Printf("Could not read body from hook.")
           w.WriteHeader(http.StatusBadRequest)  // 400 for read errors
           return
       }
       var response servicedef.HookExecutionPayload
       err = json.Unmarshal(bytes, &response)
       if err != nil {
           logger.Printf("Could not unmarshal hook payload.")
           w.WriteHeader(http.StatusBadRequest)  // 400 for unmarshaling errors
           return
       }
       
       go func() {
           h.CallChannel <- response
       }()
       
       w.WriteHeader(http.StatusOK)  // 200 for success
   })
   ```
   
   **From `mockld/callback_helpers.go` documentation**:
   > "For simplicity, and to make it clear that these calls should never be cached, the method is always POST"
   
   **Response codes used**:
   - `http.StatusOK` (200) - Success
   - `http.StatusBadRequest` (400) - Invalid request body or JSON
   - `http.StatusInternalServerError` (500) - Handler error
   - `http.StatusNoContent` (204) - DELETE to close endpoint
   
   **Decision**: ✅ **DECIDED** - Follow the callback service pattern
   - **HTTP Method**: POST (never cached, clear semantics)
   - **Response Codes**:
     - 200 (StatusOK) for successful receipt
     - 400 (StatusBadRequest) for malformed JSON
     - 500 (StatusInternalServerError) for test harness errors
   - **Retry Logic**: None - SDK test service sends once, logs failures
   - **Pattern**: Same as hooks - simple POST, no retry, log errors

2. **Listener ID Format**
   
   **Question**: What format/constraints should listener IDs have?
   
   **Existing Test Harness Pattern (from Hooks)**:
   
   Hook names in tests are simple descriptive strings:
   ```go
   hookName := "executesBeforeEvaluationStage"
   hookName := "beforeEvaluationDataPropagatesToAfter"
   ```
   
   For multiple instances, append index:
   ```go
   const numHooks = 3
   var names []string
   for i := 0; i < numHooks; i++ {
       names = append(names, "fallibleHook-"+strconv.Itoa(i))
   }
   ```
   
   **Pattern**: Arbitrary string chosen by test, no enforced format or constraints
   
   **Decision**: ✅ **DECIDED** - Follow the hook naming pattern
   - **Format**: Arbitrary string, test author chooses
   - **Constraints**: None enforced by harness or SDK
   - **Convention**: Use descriptive names in tests (e.g., "listener-1", "value-listener-for-flag-key")
   - **Multiple listeners**: Append numeric suffix if needed (e.g., "listener-0", "listener-1", "listener-2")
   - **Uniqueness**: Test author's responsibility (same as hook names)

3. **Cleanup**
   
   **Question**: How should listener lifecycle be managed? Auto-cleanup or explicit?
   
   **Existing Test Harness Pattern (from Hooks, SDKClient, Events)**:
   
   Test objects are automatically cleaned up using `defer`:
   ```go
   // From common_tests_hooks.go
   client, hooks := createClientForHooks(t, []string{hookName}, nil, configurers...)
   defer hooks.Close()  // Auto-cleanup at end of test
   
   // Close() implementations
   func (h *Hooks) Close() {
       for _, instance := range h.instances {
           instance.hookService.Close()  // Close each callback endpoint
       }
   }
   ```
   
   **Pattern**: Objects have `Close()` method, called via `defer` after creation
   
   **From `testapi_sdk_client.go` documentation**:
   > "The object's lifecycle is tied to the test scope that created it; it will be automatically closed"
   
   **Decision**: ✅ **DECIDED** - Follow the automatic cleanup pattern
   - **Listener Lifecycle**: 
     - Test service should clean up all listeners when SDK client closes
     - Listeners can also be explicitly unregistered via `unregisterListener` command
   - **Test Pattern**:
     - Callback service has `Close()` method
     - Called via `defer` after creation
     - Tests can also explicitly test unregister functionality
   - **Verification**: Tests can verify cleanup by:
     1. Closing client, changing flag, expecting no notification
     2. Explicitly unregistering listener, changing flag, expecting no notification

### SDK-Specific Considerations

See [Client-Side SDK Analysis](#client-side-sdk-analysis) for a full breakdown of client-side listener APIs and the test sharing plan.

1. **Go Server SDK** ✅ Implemented
   - Uses channels, not callbacks; test service spins a goroutine per listener
   - `FlagTracker.AddFlagChangeListener()` returns a channel; test service drains it and POSTs to callback URL
   - Integrated and passing tests; see `go-server-sdk` test service implementation

2. **JavaScript / React Native (js-core)**
   - Event emitter: `client.on('change:key', cb)` for specific flags, `client.on('change', cb)` for all flags
   - No old/new values in callback; test service must use `variation()` to get current value for `ListenerNotification`
   - For value change listeners: the test service re-evaluates the flag after receiving the event
   - JS client-side SDKs need a polling endpoint alongside the streaming endpoint (initial data via polling)

3. **.NET Client SDK (`dotnet-core`)**
   - Single `FlagTracker.FlagValueChanged` event for all flags; event args include `Key`, `OldValue`, `NewValue`
   - For `registerFlagChangeListener` (general): subscribe to the event, filter by `Key` if `FlagKey` is set
   - For `registerFlagValueChangeListener`: subscribe and use the provided old/new values directly
   - Fires on `Identify` when flag values change for the new context

4. **Android SDK**
   - `registerFeatureFlagListener(key, listener)` for per-flag (no values in callback; must call `variation()`)
   - `LDAllFlagsListener` for all-flags changes
   - For `registerFlagChangeListener` with empty `FlagKey`: use `LDAllFlagsListener`
   - For `registerFlagValueChangeListener`: use per-flag listener + call `variation()` for old/new values

5. **Mobile SDKs (general)**
   - May have background/foreground behavior affecting streaming
   - Streaming data format is `ClientSDKFlagWithKey` (evaluated result), not the full flag model

6. **Roku**
   - Uses `observeField` — a different event pattern
   - May need special handling or suppression of listener tests

---

## JS Test Service Implementation Guide

This section is a self-contained reference for implementing `flag-change-listeners` support in the **`js-core`** test service. It is intended to be read in a fresh Cursor session scoped to the `js-core` repository.

### Background

The sdk-test-harness defines a contract between itself and each SDK's **test service** (a small HTTP server that wraps the SDK). The test harness sends commands to the test service; the test service calls the SDK and then POSTs a callback to a URL provided by the harness.

For flag change listeners, the test service must:
1. Advertise the `"flag-change-listeners"` capability in its `/` status response
2. Handle three new commands: `registerFlagChangeListener`, `registerFlagValueChangeListener`, `unregisterListener`
3. When a listener fires, HTTP POST a `ListenerNotification` JSON body to the `callbackUri` supplied in the command params

### Reference Implementation

The **hooks** feature in `js-core` is the closest existing analog and should be used as a pattern. Find the hooks test service implementation (search for `"evaluation-hooks"` or `HookExecutionPayload` in the test service code) to understand:
- How a new capability is added to the status response
- How a new command is dispatched in the command handler
- How an HTTP callback is POSTed from the test service

### New Commands

#### `registerFlagChangeListener`

Registers a general flag change listener. The listener fires whenever the flag's configuration changes (regardless of whether the evaluated value changes). If `flagKey` is empty, the listener fires for any flag.

**JSON params** (field `registerFlagChangeListener` in the command body):
```json
{
  "listenerId": "listener-1",
  "flagKey": "my-flag",
  "callbackUri": "http://test-harness-host/callback/123"
}
```

**JS SDK mapping:**
```typescript
// Specific flag
client.on(`change:${flagKey}`, handler);

// All flags (flagKey is empty string)
client.on('change', handler);
```

**When the listener fires**, POST to `callbackUri`:
```json
{
  "listenerId": "listener-1",
  "flagKey": "my-flag"
}
```

> **Important:** For a general flag change listener, do NOT include `oldValue` or `newValue` in the notification. The test harness only checks `flagKey`.

> **JS caveat:** The `'change:key'` event fires with the current context as argument; the `'change'` event fires with `(context, changedFlagKeys[])`. For the general listener, you only need to POST the notification — no flag evaluation needed.

#### `registerFlagValueChangeListener`

Registers a value change listener. The listener fires only when the flag's evaluated value actually changes.

**JSON params** (field `registerFlagValueChangeListener` in the command body):
```json
{
  "listenerId": "listener-1",
  "flagKey": "my-flag",
  "context": { "kind": "user", "key": "user-key" },
  "defaultValue": "default",
  "callbackUri": "http://test-harness-host/callback/123"
}
```

> **JS caveat:** The `context` field should be **ignored** for client-side SDKs. The JS client evaluates for its own current context; the context sent by the harness is meaningless and should not be used.

**JS SDK mapping:**
```typescript
// Must capture oldValue at registration time, then re-evaluate after each change
let oldValue = client.variation(flagKey, defaultValue);

const handler = (context) => {
  const newValue = client.variation(flagKey, defaultValue);
  if (/* newValue !== oldValue */) {
    // POST notification
    oldValue = newValue;
  }
};

client.on(`change:${flagKey}`, handler);
```

> **Important note on value change detection:** The JS SDK's `'change:key'` event already fires only when the value changes, so the `oldValue !== newValue` check may be redundant. However, capturing `oldValue` at registration time and including both in the notification is required because the harness asserts on both `OldValue` and `NewValue` in `ExpectValueChangeNotification`.

**When the listener fires**, POST to `callbackUri`:
```json
{
  "listenerId": "listener-1",
  "flagKey": "my-flag",
  "oldValue": "value1",
  "newValue": "new-value"
}
```

#### `unregisterListener`

Removes a previously registered listener (either kind).

**JSON params** (field `unregisterListener` in the command body):
```json
{
  "listenerId": "listener-1"
}
```

**JS SDK mapping:**
```typescript
client.off(eventName, handler);
```

The test service must keep a map of `listenerId → { eventName, handler }` so it can call `client.off()` with the right arguments.

### Callback HTTP Protocol

- **Method**: POST
- **URL**: the `callbackUri` from the command params
- **Body**: JSON (`ListenerNotification` as shown above)
- **Expected response**: any 2xx; ignore the body
- **No retries**: fire-and-forget; if the POST fails, do not retry

### Listener Map (Bookkeeping)

The test service needs a map keyed by `listenerId` to:
1. Retrieve the handler for `unregisterListener`
2. Avoid memory leaks from orphaned listeners

Suggested shape:
```typescript
interface ListenerEntry {
  eventName: string;    // e.g. 'change:my-flag' or 'change'
  handler: Function;
}
const listeners = new Map<string, ListenerEntry>();
```

### Capability Declaration

In the `/` status response, add:
```json
"flag-change-listeners"
```

to the `capabilities` array.

### Streaming Setup for Listener Tests

The test harness listener tests always use **streaming** (not polling) to push flag updates. For JS-based SDKs, the test harness will set up two data sources:
1. A **polling** endpoint (for initial data — JS SDKs always poll first)
2. A **streaming/SSE** endpoint (for updates)

The test service does not need to do anything special here — this is handled by the test harness side. The test service just needs to connect to whatever endpoints the harness configures.

### Test Coverage

The test harness will run these tests against your implementation (once `"flag-change-listeners"` is in the capabilities):

**Flag change listener (general)**
- `receives notification when flag changes` — specific key, value changes
- `fires on config change even when value unchanged` — **SKIPPED** for client-side (server-side only)
- `filters by flag key` — only fires for the subscribed flag, not others
- `with empty flag key receives all flag changes` — `client.on('change', ...)` path

**Flag value change listener**
- `receives notification when value changes` — must include oldValue and newValue
- `does not notify when value is unchanged` — same value pushed, no POST expected
- `multiple listeners both receive notification` — two listeners for same flag, both fire
- `is context specific` — **SKIPPED** for client-side (server-side only)

---

### LaunchDarkly Documentation
- [Flag Changes (General)](https://launchdarkly.com/docs/sdk/features/flag-changes)
- [Go SDK Reference](https://pkg.go.dev/github.com/launchdarkly/go-server-sdk/v7)

### SDK Implementations Analyzed

#### Go Server SDK
- **Repository**: [launchdarkly/go-server-sdk](https://github.com/launchdarkly/go-server-sdk)
- **Interface**: `interfaces/flag_tracker.go`
- **Implementation**: `internal/flag_tracker_impl.go`
- **Tests**: `ldclient_listeners_test.go`, `ldclient_listeners_fdv2_test.go`
- **Pattern**: Go channels

#### .NET Server SDK
- **Repository**: [launchdarkly/dotnet-core](https://github.com/launchdarkly/dotnet-core)
- **Interface**: `pkgs/sdk/server/src/Interfaces/IFlagTracker.cs`
- **Implementation**: `pkgs/sdk/server/src/Internal/FlagTrackerImpl.cs`
- **Pattern**: C# Events (delegates)

#### JavaScript SDK
- **Repository**: [launchdarkly/js-core](https://github.com/launchdarkly/js-core)
- **Client**: `packages/sdk/browser/src/LDClient.ts`
- **Emitter**: `packages/shared/sdk-client/src/LDEmitter.ts`
- **Pattern**: EventEmitter

#### Python Server SDK
- **Repository**: [launchdarkly/python-server-sdk](https://github.com/launchdarkly/python-server-sdk)
- **Interface**: `ldclient/interfaces.py` (class `FlagTracker`)
- **Implementation**: `ldclient/impl/flag_tracker.py`
- **Tests**: `ldclient/testing/impl/test_flag_tracker.py`
- **Pattern**: Callback functions

### Test Harness
- [Test Harness Repository](https://github.com/launchdarkly/sdk-test-harness)
- [Service Specification](./docs/service_spec.md)
- [Writing Tests Guide](./docs/writing_tests.md)

---

## Change Log

| Date | Author | Changes |
|------|--------|---------|
| 2026-02-12 | Initial | Created document with Go SDK analysis and design proposal |
| 2026-02-13 | Update | Added multi-SDK analysis covering .NET, JavaScript, and Python implementations |
| 2026-02-13 | Update | Decided on separate commands for flag-change vs value-change listeners; updated all code examples to reflect two command types and payloads |
| 2026-02-13 | Update | Clarified capability definition location (test harness) vs. capability reporting (SDK test services); expanded Phase 1 and Phase 4 implementation details |
| 2026-02-13 | Update | Analyzed "Initial Value Handling" across Go, Python, JavaScript, and .NET SDKs; decided tests will NOT expect initial notifications, simplifying implementation |
| 2026-02-13 | Update | Analyzed "Async Callback Handling": confirmed 5-second timeout standard (matching hooks/events), confirmed SDKs don't batch listener notifications |
| 2026-02-13 | Update | Analyzed "Error Cases" and "Thread Safety": documented existing test harness patterns from hooks and client independence tests; decided to follow these established patterns |
| 2026-02-13 | Update | Analyzed all "Technical Questions" (HTTP semantics, ID format, cleanup): documented callback service patterns from hooks; all questions now decided using existing conventions |
| 2026-02-19 | Update | Added Client-Side SDK Analysis section: surveyed JS/React Native, .NET Client, and Android listener APIs; concluded callback architecture works for client-side; documented key differences in data format and context handling; created Phase 3B plan for refactoring tests to support both SDK kinds |

