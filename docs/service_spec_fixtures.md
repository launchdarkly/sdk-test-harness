# SDK test harness callback fixtures

As part of the contract tests, the test harness may need to simulate services that are external to the SDK. This allows it to control all of the data that the SDK sees. It does this by providing an endpoint within its own HTTP server that simulates the behavior of the external service, and telling the test service to configure the SDK to use the URL of that endpoint. These all have the same hostname and port, the same one that is controlled by the `-port` command-line parameter; the test harness does not listen on multiple ports (so it is safe to expose just one port if it is deployed in Docker).

In some cases, it is simulating a LaunchDarkly service such as the streaming service or the analytics events service-- verifying that the SDK interacts correctly with LaunchDarkly. But in other cases, it is simulating a component that would be implemented in application code or library code as a plug-in component to the SDK-- verifying the SDK interacts correctly with optional integrations. These tests rely on the SDK having a standardized interface for such components, so that a test fixture can be plugged in with preconfigured behavior.

How this works is that for each type of component used in these tests, the test service must provide a generic implementation of the interface for that component, where each method calls a corresponding endpoint in the test harness. The test harness will provide the base URI for these endpoints, and the test service is responsible for translating the parameters sent by the SDK into an HTTP request body, and then translating the response into whatever type the SDK expects.

These calls all have the following semantics:

* Whatever relative path is shown below for the call must be added to the `baseUri` property that the test harness provided for this type of component.
* The request is always a `POST` request (for simplicity, and to clarify that these requests are never cached).
* If the call takes any parameters, the request body is always a JSON object; its properties are shown below for each method. If the call takes no parameters, the request body is ignored.
* If the call returns any values, the response body is always a JSON object; its properties are shown below for each method.
* The response status should be 2xx for all successful calls (where "successful" means that the interface method would return without an error). For any non-2xx status, the implementation method should take the response body as a plain-text error message and return an error to the SDK in whatever way is normal for the platform (for instance, throwing an exception in Java). This is important because the test harness may deliberately return errors as part of a test.

## Persistent data store

If the test service has the capability `"persistent-data-store"`, and the SDK client configuration has a non-null `persistentDataStore` property, the test service must configure the SDK to enable persistent storage. Instead of a real database implementation, it must tell the SDK to use a custom persistent data store component defined in the test service code. This component must implement the basic operations for server-side SDK data stores: Init, Get, Get All, Upsert, and Is Initialized (exact names may vary per SDK).

What these tests are testing is all of the SDK behavior regarding databases that is _not_ specific to a particular database:

* The SDK correctly serializes and deserializes JSON data
* Caching works as expected
* Error handling works as expected if the database returns an error

### Init: `POST {baseUri}/init`

Request properties:

* `data` (array, required): An array of JSON objects with these properties:
  * `kind` (string, required): The standard namespace used for items of a particular kind: "features" for flags, "segments" for segments
  * `items` (array, required): An array of JSON objects with the same properties shown for *GetAll* below

No response body.

### Get: `POST {baseUri}/get`

Request properties:

* `kind` (string, required): The standard namespace used for items of a particular kind: "features" for flags, "segments" for segments
* `key` (string, required): The key of the desired flag or segment

Response properties:

* `item` (object, optional): Null means "not found"; otherwise a JSON object with these properties:
  * `version` (number, required): The version number of this item
  * `data` (string, required): The serialized data for this item

### Get All: `POST {baseUri}/getAll`

Request properties:

* `kind` (string, required): The standard namespace used for items of a particular kind: "features" for flags, "segments" for segments

Response properties:

* `items` (array, required): An array of JSON objects with these properties:
  * `key` (string, required): The key of this flag or segment
  * `item` (object, required): A JSON object with these properties:
    * `version` (number, required): The version number of this item
    * `data` (string, required): The serialized data for this item

### Upsert: `POST {baseUri}/upsert`

Request properties:

* `kind` (string, required): The standard namespace used for items of a particular kind: "features" for flags, "segments" for segments
* `key` (string, required): The key of the desired flag or segment
* `item` (object, required): A JSON object with these properties:
  * `version` (number, required): The version number of this item
  * `data` (string, optional): The serialized data for this item; if omitted or an empty string, this is a deleted item placeholder

Response properties:

* `updated` (boolean, required): True if the update was performed, false if it was not performed

### Is Initialized: `POST {baseUri}/isInitialized`

No parameters (request body is ignored).

Response properties:

* `result` (boolean, required): True if the data store reports that it has already been initialized

## Big Segment store

If the test service has the capability `"big-segments"`, and the SDK client configuration has a non-null `bigSegments` property, the test service must configure the SDK to enable Big Segments. Instead of a real database implementation, it must tell the SDK to use a custom Big Segment store component defined in the test service code. This component must implement the basic operations for server-side SDK Big Segment stores: Get Metadata and Get Membership (exact names may vary per SDK).

What these tests are testing is all of the SDK behavior regarding Big Segments that is _not_ specific to a particular database:

* The SDK does the expected membership query when a Big Segment is referenced in an evaluation
* The SDK generates the expected hash for this query
* The SDK correctly interprets membership data returned by queries
* The SDK correctly uses status information reported by the store

### Get Metadata: `POST {baseUri}/getMetadata`

No parameters (request body is ignored).

Response properties:

* `lastUpToDate` (number, required): The epoch millisecond time that the simulated store was last updated.

### Get Membership: `POST {baseUri}/getMembership`

Request properties:

* `userHash` (string, required): A hash of the user key. There is a standard algorithm for computing this; the test harness will check the hash to ensure that the SDK follows the specification.

Response properties:

* `values` (object, optional): A set of properties where each property key is a segment reference string (in the standard format used by SDK Big Segment data), and each value is either `true`, `false`, or `null`.

The test service's Big Segment store implementation should return a corresponding membership state to the SDK. When the SDK queries the membership for any given segment reference string, it gets either `true`, `false`, or "no value" (any key that does not exist in the object should be considered "no value", same as if it had a null value).

On platforms where the membership object is nullable, so that the query method could return null/nil instead of a membership object with no values, the test service should return null/nil if `values` is omitted or null. This lets the test harness verify that the SDK treats these two scenarios as equivalent and does not throw any kind of null reference error.
