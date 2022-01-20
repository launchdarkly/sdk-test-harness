# SDK test service specification

## Service endpoints

### Status resource: `GET /`

This resource should return a 200 status to indicate that the service has started. Optionally, it can also return a JSON object in the response body, with the following properties:

* `name`: Identifies the SDK being tested by the service, such as `"go-server-sdk"`.
* `clientVersion`: The version string of the SDK.
* `capabilities`: An array of strings describing optional features that this SDK supports (see below).

The test harness will use the `capabilities` information to decide whether to run optional parts of the test suite that relate to those capabilities.

#### Capability `"server-side"`

This means that the SDK a server-side SDK. The test harness will also support client-side SDKs in the future.

#### Capability `"strongly-typed"`

This means that the SDK has separate APIs for evaluating flags of specific variation types, such as boolean or string.

#### Capability `"all-flags-with-reasons"`

This means that the SDK's method for evaluating all flags at once has an option for including evaluation reasons.

#### Capability `"all-flags-client-side-only"`

This means that the SDK's method for evaluating all flags at once has an option for filtering the result to only include flags that are enabled for client-side use.

#### Capability `"all-flags-details-only-for-tracked-flags"`

This means that the SDK's method for evaluating all flags at once has an option for filtering the result to only include evaluation reason data if the SDK will need it for events (due to event tracking or debugging or an experiment).

### Stop test service: `DELETE /`

The test harness sends this request at the end of a test run if you have specified `--stop-service-at-end` on the [command line](./running.md). The test service should simply quit. This is a convenience so CI scripts can simply start the test service in the background and assume it will be stopped for them.

### Create SDK client: `POST /`

A `POST` request indicates that the test harness wants to start an instance of the SDK client. The request body is a JSON object with the following properties. Any property that is omitted should be considered the same as null (or `false` for a boolean).

* `tag` (string, required): A string describing the current test, if desired for logging.
* `configuration` (object, required): SDK configuration. Properties are:
  * `credential` (string, required): The SDK key.
  * `startWaitTimeMs` (number, optional): The initialization timeout in milliseconds. If omitted or zero, the default is 5000 (5 seconds).
  * `timeoutOk` (boolean, optional): If true, the test service should _not_ return an error for client initialization timing out (that is, a timeout is an expected condition in this test and we still want to be able to use the client). The default behavior is that a timeout should return a `500` error.
  * `streaming` (object, optional): Enables streaming mode and provides streaming configuration. Currently the test harness only supports streaming mode, so this will be inferred if it is omitted. Properties are
    * `baseUri` (string, optional): The base URI for the streaming service. For contract testing, this will be the URI of a simulated streaming endpoint that the test harness provides. If it is null or an empty string, the SDK should connect to the real LaunchDarkly streaming service.
  * `events` (object, optional): Enables events and provides events configuration, or disables events if it is omitted or null. Properties are:
    * `baseUri` (string, optional): The base URI for the events service. For contract testing, this will be the URI of a simulated event-recorder endpoint that the test harness provides. If it is null or an empty string, the SDK should connect to the real LaunchDarkly events service.
    * `enableDiagnostics` (boolean, optional): If true, diagnostic events should be enabled. Otherwise they should be disabled.
    * `allAttributesPrivate` (boolean, optional): Corresponds to the SDK configuration property of the same name.
    * `globalPrivateAttributes` (array, optional): Corresponds to the `privateAttributes` property in the SDK configuration (rather than in an individual user).
    * `flushIntervalMs` (number, optional): The event flush interval in milliseconds. If omitted or zero, use the SDK's default value.
    * `inlineUsers` (boolean, optional): Corresponds to the SDK configuration property of the same name.

The response to a valid request is any HTTP `2xx` status, with a `Location` header whose value is the URL of the test service resource representing this SDK client instance (that is, the one that would be used for "Close client" or "Send command" as described below).

If any parameters are invalid, return HTTP `400`.

If client initialization throws an exception, or it times out and `timeoutOk` was _not_ set to true, return HTTP `500`.

### Send command: `POST <URL of SDK client instance>`

A `POST` request to the resource that was returned by "Create SDK client" means the test harness wants to do something with an existing SDK client instance.

The request body is a JSON object. It always has a string property `command` which identifies the command. For each command that takes parameters, there is an optional property with the same name as that command, containing its parameters, which will be present only for that command. This simplifies the implementation of the test service by not requiring a separate endpoint for each command.

Whenever there is a `user` property, the JSON object for the user follows the standard schema used by all LaunchDarkly SDKs.

#### Evaluate flag

If `command` is `"evaluate"`, the test service should perform a single feature flag evaluation. The SDK methods for this normally have names ending in `Variation` or `VariationDetail`.

The `evaluate` property in the request body will be a JSON object with these properties:

* `flagKey` (string): The flag key.
* `user` (object): The user properties.
* `valueType` (string): For strongly-typed SDKs, this can be `"bool"`, `"int"`, `"double"`, `"string"`, or `"any"`, indicating which typed `Variation` or `VariationDetail` method to use (`any` is called "JSON" in most SDKs). For weakly-typed SDKs, it can be ignored.
* `defaultValue` (any): A JSON value whose type corresponds to `valueType`. This should be used as the application default/fallback parameter for the `Variation` or `VariationDetail` method.
* `detail` (boolean): If true, use `VariationDetail`. If false or omitted, use `Variation`.

The response should be a JSON object with the following properties:

* `value` (any): The JSON value of the result.
* `variationIndex` (int or null): The variation index of the result, if any-- only if `VariationDetail` was called.
* `reason` (object or null): The evaluation reason of the result, if any-- only if `VariationDetail` was called-- in the standard schema for evaluation reasons used by all LaunchDarkly SDKs.

#### Evaluate all flags

If `command` is `"evaluateAll"`, the test service should call the SDK method that evaluates all flags at once, which is normally called `AllFlags` or `AllFlagsState`.

The `evaluateAll` property in the request body will be a JSON object with these properties:

* `user` (object): The user properties.
* `withReasons` (boolean, optional): If true, enables the SDK option for including evaluation reasons in the result. The test harness will only set this option if the test service has the capability `"all-flags-with-reasons"`.
* `clientSideOnly` (boolean, optional): If true, enables the SDK option for filtering the result to only include flags that are enabled for client-side use. The test harness will only set this option if the test service has the capability `"all-flags-client-side-only"`.
* `detailsOnlyForTrackedFlags` (boolean, optional): If true, enables the SDK option for filtering the result to only include evaluation reason data if the SDK will need it for events (due to event tracking or debugging or an experiment). The test harness will only set this option if the test service has the capability `"all-flags-details-only-for-tracked-flags"`.

The response should be a JSON object with a single property, `state`. The value of `state` is the JSON representation that the SDK provides for the result of the `AllFlagsState` call into JSON, in the format that is expected by the JS browser SDK: a JSON object where there is a key-value pair for each flag key and flag value, plus a `$flagMetadata` key containing additional metadata. Example:

```json
{
  "state": {
    "flagkey1": "value1",
    "flagkey2": "value2",
    "$flagsState": {
      "flagKey1": { "variation": 0, "version": 100 }
      "flagKey2": { "variation": 1, "version": 200 }
    },
    "$valid": true
  }
}
```

#### Send identify event

If `command` is `"identifyEvent"`, the test service should call the SDK's `Identify` method.

The `identifyEvent` property in the request body will be a JSON object with these properties:

* `user` (object): The user properties.

The response should be an empty 2xx response.

#### Send custom event

If `command` is `"customEvent"`, the test service should tell the SDK to send a custom event. The methods for this normally have names starting with `Track`.

The `customEvent` property in the request body will be a JSON object with these properties:

* `eventKey` (string): The event key.
* `user` (object): The user properties.
* `data` (any): If present, a JSON value for the `data` parameter.
* `omitNullData` (boolean or null): See below.
* `metricValue` (number or null): If present, a metric value.

Some SDKs have multiple variants or overloads of `Track`: one that takes both `data` and `metricValue` parameters, one with only `data`, one with neither, etc. To ensure full test coverage, the test service for such an SDK should interpret the parameters as follows:

* A `Track` variant with only `eventKey` and `user` should be called if `data` and `metricValue` are both null _and_ `omitNullData` is true.
* Otherwise, a variant with only `eventKey`, `user`, and `data` should be called if `metricValue` is null.
* Otherwise, call the variant that takes `eventKey`, `user`, `data`, and `metricValue`.

The response should be an empty 2xx response.

#### Send alias eent

If `command` is `"aliasEvent"`, the test service should tell the SDK to send an alias event.

The `aliasEvent` property in the request body will be a JSON object with these properties:

* `user` (object): The user properties of the new user.
* `previousUser` (object): The user properties of the previous user.

The response should be an empty 2xx response.

#### Flush events

If `command` is `"flush"`, the test service should tell the SDK to initiate an event flush.

The response should be an empty 2xx response.
