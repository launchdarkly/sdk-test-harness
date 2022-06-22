# SDK test service specification

## General guidelines

* Request and response bodies, if required for a particular endpoint, are always in JSON.
* For any optional property in a request or response, `"propertyName": null` should be treated the same as if `propertyName` were entirely omitted.
* If the test service is unable to decode a JSON request body, either because it's not valid JSON or because a property value is of the wrong type, it should return a 400 error.
* For any request where the body is irrelevant, the test service should not care whether there is a body and, if there is, should not attempt to decode it as any specific content type. Similarly, for callback endpoints, if no body data is specified for a `POST` request it does not matter what the test service puts in the callback request body.
* To simplify test service implementation, the test harness does not distinguish between different 2xx statuses, so for instance 201 and 202 are equally valid regardless of which one would be most appropriate in HTTP semantics.
* If an endpoint returns a 400 or 500 error status, it may put a plain text message in the response body which will be shown in the test harness log.

## Service endpoints

### Status resource: `GET /`

This resource should return a 200 status to indicate that the service has started. Optionally, it can also return a JSON object in the response body, with the following properties:

* `name`: Identifies the SDK being tested by the service, such as `"go-server-sdk"`.
* `clientVersion`: The version string of the SDK.
* `capabilities`: An array of strings describing optional features that this SDK supports (see below).

The test harness will use the `capabilities` information to decide whether to run optional parts of the test suite that relate to those capabilities.

#### SDK type capabilities: `"server-side"`, `"client-side"`, `"mobile"`

The most basic decision in this regard is what type of SDK is being tested: server-side, mobile client-side, or JavaScript-based client-side. The server-side test suite is much more detailed, since client-side SDKs do not have their own evaluation logic. In the client-side test suite, the two variants (mobile and JavaScript-based) mostly receive the same tests, but each variant uses somewhat different simulated LaunchDarkly services.

* If `"server-side"` is present, this is a server-side SDK.
* Otherwise, if `"client-side"` and `"mobile"` are present, this is a mobile client-side SDK.
* Otherwise, if `"client-side"` is present without `"mobile"`, this is a JavaScript-based client-side SDK.
* If none of the above are true, no tests can be run.

#### Capability `"singleton"`

This means that the SDK only allows a single client instance to be active at any time.

#### Capability `"strongly-typed"`

This means that the SDK has separate ___Variation/___VariationDetail APIs for evaluating flags of specific variation types, such as boolean or string. If it is not present, then the SDK has only a single Variation or VariationDetail method which can be used for a flag variation of any type.

#### Capability `"all-flags-with-reasons"`

This means that the SDK's method for evaluating all flags at once has an option for including evaluation reasons. This is only applicable to server-side SDKs.

#### Capability `"all-flags-client-side-only"`

This means that the SDK's method for evaluating all flags at once has an option for filtering the result to only include flags that are enabled for client-side use. This is only applicable to server-side SDKs.

#### Capability `"all-flags-details-only-for-tracked-flags"`

This means that the SDK's method for evaluating all flags at once has an option for filtering the result to only include evaluation reason data if the SDK will need it for events (due to event tracking or debugging or an experiment). This is only applicable to server-side SDKs.

#### Capability `"big-segments"`

This means that the SDK supports Big Segments and can be configured with a custom Big Segment store. This is only applicable to server-side SDKs.

For tests that involve Big Segments, the test harness will provide parameters in the `bigSegments` property of the configuration object, including a `callbackUri` that points to one of the test harness's callback services (see [Callback endpoints](#callback-endpoints)). The test service should configure the SDK with its own implementation of a Big Segment store, where every method of the store delegates to a corresponding endpoint in the callback service.

#### Capability `"server-side-polling"`

For a server-side SDK, this means that the SDK can be configured to use polling mode instead of streaming mode.

All server-side SDKs do support polling mode, but since it was not included in the original test service specification, it is an opt-in capability to indicate that the test service understands the `polling` configuration options.

#### Capability `"service-endpoints"`

This means that the SDK supports setting the base URIs for the streaming, polling, and events services separately from whether those services are enabled.

Certain tests are only possible to do if the SDK's configuration API works in this way. For instance, to test whether events can be disabled, the test harness has to be able to create a mock endpoint that _would_ receive events if they were sent, and then configure the SDK to know the base URI of that endpoint, while also telling the SDK not to send events. Such tests will use the `serviceEndpoints` part of the configuration object. They will be skipped if the capability is not present.

Note that even if this capability is present, the test harness may still choose to use the other method of setting base URIs per service (that is, specifying a `baseUri` property within `streaming` or `events`) since that is guaranteed to work for all test service implementations.

#### Capability `"tags"`

This means that the SDK supports the "tags" configuration option and will send the `X-LaunchDarkly-Tags` header in HTTP requests if tags are defined.

For tests that involve tags, the test harness will set the `tags` property of the configuration object.

### Stop test service: `DELETE /`

The test harness sends this request at the end of a test run if you have specified `--stop-service-at-end` on the [command line](./running.md). The test service should simply quit. This is a convenience so CI scripts can simply start the test service in the background and assume it will be stopped for them.

### Create SDK client: `POST /`

A `POST` request indicates that the test harness wants to start an instance of the SDK client. The request body is a JSON object with the following properties. Any property that is omitted should be considered the same as null (or `false` for a boolean).

* `tag` (string, required): A string describing the current test, if desired for logging.
* `configuration` (object, required): SDK configuration. Properties are:
  * `credential` (string, required): The SDK key for server-side SDKs, mobile key for mobile SDKs, or environment ID for JS-based SDKs.
  * `startWaitTimeMs` (number, optional): The initialization timeout in milliseconds. If omitted or zero, the default is 5000 (5 seconds).
  * `initCanFail` (boolean, optional): If true, the test service should _not_ return an error for client initialization failing in a way that still makes the client instance available (for instance, due to a timeout or a 401 error). See discussion of error handling below.
  * `serviceEndpoints` (object, optional): See notes on the `"service-endpoints"` capability. If this object is present, the test service should use it to set the corresponding service URIs in the SDK.
    * `streaming`, `polling`, `events` (string, optional): Each of these, if set, is the base URI for the corresponding service.
  * `streaming` (object, optional): Enables streaming mode and provides streaming configuration. If this is omitted _and_ `polling` is also omitted, then the test service can use streaming as a default; but if `streaming` is omitted and `polling` is provided, then streaming should be disabled. Properties are:
    * `baseUri` (string, optional): The base URI for the streaming service. For contract testing, this will be the URI of a simulated streaming endpoint that the test harness provides. If it is null or an empty string, the SDK should default to the value from `serviceEndpoints.streaming` if any, or if that is not set either, connect to the real LaunchDarkly streaming service.
    * `initialRetryDelayMs` (number, optional): The initial stream retry delay in milliseconds. If omitted, use the SDK's default value.
  * `polling` (object, optional): Enables polling mode and provides polling configuration. Properties are:
    * `baseUri` (string, optional): The base URI for the polling service. For contract testing, this will be the URI of a simulated polling endpoint that the test harness provides. If it is null or an empty string, the SDK should default to the value from `serviceEndpoints.polling` if any, or if that is not set either, connect to the real LaunchDarkly polling service.
    * `pollIntervalMs` (number, optional): The polling interval in milliseconds. If omitted, use the SDK's default value. For mobile SDKs that are configured with both streaming and polling enabled, this should be interpreted as the _background_ polling interval.
  * `events` (object, optional): Enables events and provides events configuration, or disables events if it is omitted or null. Properties are:
    * `baseUri` (string, optional): The base URI for the events service. For contract testing, this will be the URI of a simulated event-recorder endpoint that the test harness provides.  If it is null or an empty string, the SDK should default to the value from `serviceEndpoints.events` if any, or if that is not set either, connect to the real LaunchDarkly events service.
    * `capacity` (number, optional): If specified and greater than zero, the event buffer capacity should be set to this value.
    * `enableDiagnostics` (boolean, optional): If true, diagnostic events should be enabled. Otherwise they should be disabled.
    * `allAttributesPrivate` (boolean, optional): Corresponds to the SDK configuration property of the same name.
    * `globalPrivateAttributes` (array, optional): Corresponds to the `privateAttributes` property in the SDK configuration (rather than in an individual context).
    * `flushIntervalMs` (number, optional): The event flush interval in milliseconds. If omitted or zero, use the SDK's default value.
  * `bigSegments` (object, optional): Enables and configures Big Segments. Properties are:
    * `callbackUri` (string, required): The base URI for the big segments store callback fixture. See [Callback fixtures](#callback-fixtures).
    * `userCacheSize`, `userCacheTimeMs`, `statusPollIntervalMS`, `staleAfterMs`: These correspond to the standard optional configuration parameters for every SDK that supports Big Segments.
  * `tags` (object, optional): If specified, this has options for metadata/tags (that is, values that are translated into an `X-LaunchDarkly-Tags` header):
    * `applicationId` (string, optional): If present and non-null, the SDK should set the "application ID" property to this string.
    * `applicationVersion` (string, optional): If present and non-null, the SDK should set the "application version" property to this string.
  * `clientSide` (object): This is omitted for server-side SDKs, and required for client-side SDKs. Properties are:
    * `initialContext` (object, required): The context properties to initialize the SDK with. The test service for a client-side SDK can assume that the test harness will _always_ set this: if the test logic does not explicitly provide a value, the test harness will add a default one.
    * `evaluationReasons`, `useReport` (boolean, optional): These correspond to the SDK configuration properties of the same names.

The response to a valid request is any HTTP `2xx` status, with a `Location` header whose value is the URL of the test service resource representing this SDK client instance (that is, the one that would be used for "Close client" or "Send command" as described below).

If any parameters are invalid, return HTTP `400`.

If client initialization fails, the desired behavior depends on how it failed and whether `initCanFail` was set:

* If `initCanFail` was set to true, then the test service should tolerate any kind of initialization failure where the client instance is still available. For instance, if initialization times out, or stops immediately due to getting a 401 error from LaunchDarkly, all of our SDKs still allow the application to continue using the client instance even though it may not have valid flag data; that might be an expected condition in a test, in which case `initCanFail` will be true.
* If `initCanFail` was not set to true, then errors of that kind should be treated as unexpected failures and return an HTTP `500` error, preferably with some descriptive text in the response body that can be logged by the test harness.
* Any kind of error that does _not_ make the client instance available should always cause a `500`. For instance, in languages that support exceptions, if an exception is thrown from the constructor then there is no client instance.

### Send command: `POST <URL of SDK client instance>`

A `POST` request to the resource that was returned by "Create SDK client" means the test harness wants to do something with an existing SDK client instance.

The request body is a JSON object. It always has a string property `command` which identifies the command. For each command that takes parameters, there is an optional property with the same name as that command, containing its parameters, which will be present only for that command. This simplifies the implementation of the test service by not requiring a separate endpoint for each command.

Whenever there is a `context` property, the JSON object for the context follows the standard schema used by all LaunchDarkly SDKs.

#### Evaluate flag

If `command` is `"evaluate"`, the test service should perform a single feature flag evaluation. The SDK methods for this normally have names ending in `Variation` or `VariationDetail`.

The `evaluate` property in the request body will be a JSON object with these properties:

* `flagKey` (string): The flag key.
* `context` (object): The context properties. This is required for server-side SDKs, and omitted for client-side SDKs.
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

* `context` (object): The context properties. This is required for server-side SDKs, and omitted for client-side SDKs.
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
      "flagKey1": { "variation": 0, "version": 100 },
      "flagKey2": { "variation": 1, "version": 200 }
    },
    "$valid": true
  }
}
```

#### Send identify event

If `command` is `"identifyEvent"`, the test service should call the SDK's `Identify` method.

The `identifyEvent` property in the request body will be a JSON object with these properties:

* `context` (object): The context properties. This is always provided for both server-side and client-side SDKs.

The response should be an empty 2xx response.

#### Send custom event

If `command` is `"customEvent"`, the test service should tell the SDK to send a custom event. The methods for this normally have names starting with `Track`.

The `customEvent` property in the request body will be a JSON object with these properties:

* `eventKey` (string): The event key.
* `context` (object): The context properties. This is required for server-side SDKs, and omitted for client-side SDKs.
* `data` (any): If present, a JSON value for the `data` parameter.
* `omitNullData` (boolean or null): See below.
* `metricValue` (number or null): If present, a metric value.

Some SDKs have multiple variants or overloads of `Track`: one that takes both `data` and `metricValue` parameters, one with only `data`, one with neither, etc. To ensure full test coverage, the test service for such an SDK should interpret the parameters as follows:

* A `Track` variant with only `eventKey` and `context` should be called if `data` and `metricValue` are both null _and_ `omitNullData` is true.
* Otherwise, a variant with only `eventKey`, `context`, and `data` should be called if `metricValue` is null.
* Otherwise, call the variant that takes `eventKey`, `context`, `data`, and `metricValue`.

The response should be an empty 2xx response.

#### Flush events

If `command` is `"flush"`, the test service should tell the SDK to initiate an event flush.

The request body, if any, is irrelevant.

The response should be an empty 2xx response.

#### Get big segment store status

If `command` is `""`, the test service should ask the SDK for the big segment store status.

The test harness will only send this command if the test service has the `"big-segments"` capability.

The request body, if any, is irrelevant.

The response should be a JSON object with two boolean properties, `available` and `stale`, corresponding to the standard properties of this status object in all SDKs that support Big Segments.

#### Build a context

If `command` is `"contextBuild"`, the test service should use the SDK's context builder to construct a context and then return a JSON representation of it.

The test harness will only send this command if the test service has the `"strongly-typed"` capability.

The `contextBuild` property in the request body will be a JSON object with these properties:

* `single` (object, optional): If present, this is a JSON object with properties for a single-kind context. The test service should pass these values to the corresponding builder methods if they are present.
  * `kind` (string, optional): Even though a context always has a kind, this is optional because the builder should use `"user"` as a default.
  * `key` (string, required)
  * `name` (string, optional)
  * `transient` (boolean, optional)
  * `secondary` (string, optional)
  * `private` (array of strings, optional): These strings should be treated as attribute references, i.e. they may be slash-delimited paths.
  * `custom` (object, optional): If present, these are name-value pairs for custom attributes.
* `multi` (array, optional): If present, this is an array of objects in the same format as shown for `single` above, for a multi-kind context. Only one of `single` or `multi` will be present.

The response should be a JSON object with these properties:

* `output` (string, optional): If successful, this is the JSON representation of the context as a string.
* `error` (string, optional): If present, this is an error message indicating that the SDK said the context was invalid or could not serialize it.

If the SDK returns an error for this operation, the test service should _not_ return a `4xx` response, but just return the error message in the `error` property. This is because some tests have intentionally invalid values of `input`, but the test service command itself is still valid. That is also why `input` is passed as a serialized string, rather than just being the JSON value itself, since it may be intentionally malformed. The test service should only return an HTTP error code if the request did not use the format described above.

#### Convert a context

If `command` is `"contextConvert"`, the test service should use the SDK's JSON conversions for the context type to parse a context from JSON and then return a JSON representation of the result. This verifies that parsing works correctly _and_ that the SDK does any necessary transformations, such as converting an old-style user to a context, or dropping properties that have null values.

The test harness will only send this command if the test service has the `"strongly-typed"` capability.

The `contextConvert` property in the request body will be a JSON object with these properties:

* `input` (string, required): A string that should be treated as JSON.

The response body and response status are the same as for the `contextBuild` command.

### Close client: `DELETE <URL of SDK client instance>`

The test harness sends this request when it is finished using a specific client instance. The test service should use the appropriate SDK operation to shut down the client (normally this is called `Close` or `Dispose`).

The response should be an empty 2xx response if successful, or 500 if the close operation returned an error (for SDKs where that is possible).

## Callback endpoints

As part of the contract tests, the test harness may need to simulate services that are external to the SDK. This allows it to control all of the data that the SDK sees.

The test harness will tell the service where to find these simulated services by passing `baseUri` or `callbackUri` parameters in the service configuration. All of these URIs will point to some endpoint created by the test harness, which is only valid during the lifetime of the specific tests(s) where it is used. They will all have the same hostname and port, the same one that is controlled by the `-port` command-line parameter; the test harness does not listen on multiple ports (so it is safe to expose just one port if it is deployed in Docker).

### Streaming service

Most of the tests involve injecting some simulated LaunchDarkly environment data into the SDK. The test harness does this with a callback service that mimics the behavior of the LaunchDarkly streaming endpoints.

### Big segments service

SDKs that support Big Segments normally allow the application to configure them with one of several database integrations, using a generic "Big Segment store" interface. The test harness cannot test the integrations for specific databases such as Redis, but it can test whether the SDK sends the expected queries to the database and handles the results correctly. It does this by setting `bigSegments.callbackUri` in the test service configuration to point to a callback service.

The service supports the following requests. For simplicity and to ensure that HTTP clients will not do any caching, they all use the `POST` method.

#### Get metadata: `POST /getMetadata`

The test service should send this request when the SDK calls the method for getting store metadata. The request body, if any, is ignored. The response is a JSON object with these properties:

* `lastUpToDate` (number, required): The epoch millisecond time that the simulated store was last updated.

#### Get context membership: `POST /getMembership`

The test service should send this request when the SDK calls the method for getting a context's Big Segment membership.

The request body is a JSON object with these properties:

* `contextHash` (string, required): A hash of the context key. There is a standard algorithm for computing this; the test harness will check the hash to ensure that the SDK follows the specification.

The response body is a JSON object with these properties:

* `values` (object, optional): A set of properties where each property key is a segment reference string (in the standard format used by SDK Big Segment data), and each value is either `true`, `false`, or `null`.

The test service's Big Segment store implementation should return a corresponding membership state to the SDK. When the SDK queries the membership for any given segment reference string, it gets either `true`, `false`, or "no value" (any key that does not exist in the object should be considered "no value", same as if it had a null value).

On platforms where the membership object is nullable, so that the query method could return null/nil instead of a membership object with no values, the test service should return null/nil if `values` is omitted or null. This lets the test harness verify that the SDK treats these two scenarios as equivalent and does not throw any kind of null reference error.
