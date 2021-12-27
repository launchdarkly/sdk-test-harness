# Writing tests

Some test harnesses for other products are entirely scripted: that is, the test harness code contains only generic mechanisms, and all of the tests are specified in file data that is fed into the test harness.

But, due to the wide variety of test scenarios we need to support, the domain-specific language for such scripts would have to be very complex. So, this tool does not take that approach; tests are written in Go, using a model that will be familiar to most Go programmers. Tests _can_ be parameterized using file data, but that is up to the author of each test or test suite.

## Code structure

The `sdktests` package contains all of the high-level test logic. There is a single entry point for a test suite, for instance in `sdktests/testsuite_server_side.go`, and it defines a nested structure of tests and subtests as described below.

The `sdktests` package also contains test control APIs that are specific to SDK testing. Other supporting packages include:

* `framework` and its subpackages: Lower-level test framework APIs. Test logic will normally not interact with these directly, except:
  * `framework/ldtest`: Contains the `ldtest.T` type that represents a test scope. See "Conceptual model".
* `mockld`: The test fixture components that simulate LaunchDarkly services. Test logic will not interact with most of these directly; they have facades in the `sdktests` package.
* `servicedef`: Go imlementation of the [test service specification](./service_spec.md).
* `testdata`: Data files for file-driven parameterized tests.
* `testmodel`: Go schemas of the data files, and tools for reading them.

## Conceptual model

Each test has a human-readable name. These should be concise, and ideally they should not change once defined, so that they can be referenced externally (for instance, to selectively run or disable certain tests in a script).

Any test can contain subtests. The parent test could be only a container for the subtests, or it could also perform some actions of its own. This is similar to how `t.Run(name, function)` is used in Go's `testing` package. Subtest names are concatenated with the parent test name delimited by slashes: `"top-level test name/subtest A/subtest A1 that is within A"`.

Every test function receives a parameter of type `*ldtest.T` that represents the scope of the test. Again this is very similar to Go's `testing.T`, and it implements some of the same basic interface methods such as `Errorf` and `FailNow`-- which means helper packages such as `testify/assert` can be used just as they would in regular Go tests.

As with `testing.T`, the test logic can signal any number of failures that accumulate and do not stop the test but mark it as failed; or it can signal a failure that immediately stops the test (implemented internally as a panic). Any other panic that occurs during a test also marks the test as failed, and does not propagate outside of that test.

## Manipulating the SDK and its environment

The `sdktests` package includes facades for the SDK test service that actually runs the SDK, and for the test fixtures that simulate LaunchDarkly services. The typical way for tests to use these is as follows:

1. Set up a test harness endpoint for providing SDK data, with `NewSDKDataSource`.
2. If the test will need to validate event output, set up a test harness endpoint that will receive events, with `NewSDKEventSink`.
3. Tell the test service to start the SDK, with `NewSDKClient`. This takes the objects created in steps 1 and 2 as parameters, so that it can pass the URLs of those test endpoints to the SDK.
4. Perform some operations on the object returned by `NewSDKClient`, such as evaluating flags.
5. If appropriate, use the object returned by `NewSDKEventSink` to verify that the expected events were received (after telling the client to flush events).

Depending on what the tests are doing, you may want to use a completely fresh and isolated state in each test-- that is, do steps 1-3 within one function-- or, reuse components within a subtest. For instance, if the state of the data source does not need to change and it can provide the same data each time, you could do step 1 in a parent test and then reference the same data source instance in many subtests. The lifecycle of these components is tied to whatever `ldtest.T` scope they were created in, and they will be automatically torn down when that scope exits.

See documentation comments for a full description of the available API. Here is a summary:

* `sdktests.SDKDataSource`: Currently this only supports providing an initial set of server-side SDK flag/segment data via a streaming endpoint. It will provide the same data every time an SDK connects to the test harness endpoint. In the future, it will also support sending `patch` updates, simulating a polling endpoint, and verifying the HTTP request/connection behavior of the SDK.
* `sdktests.SDKEventSink`: Currently this only supports inspecting received lists of analytics events. In the future, it will also support inspecting diagnostic events, and verifying the HTTP request/retry behavior of the SDK.
* `sdktests.SDKClient`: The methods of this type correspond to SDK methods that the test harness is telling the test service to call. They include evaluating flags, sending events, and flushing events.
