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
* `data`: Data files for file-driven parameterized tests.
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

## Test assertions

Since the `ldtest.T` type implements the same basic interface as `testing.T` in terms of reporting failures, you can use any assertion framework that acts on an equivalent interface rather than specifically on `testing.T`.

For instance, the `testify/assert` and `testify/require` packages will work with `ldtest.T` because they are written against an equivalent interface (`assert.TestingT` or `require.TestingT`). So, many of these tests use `assert` and `require` methods for convenience. The `github.com/launchdarkly/go-test-helpers/v2/matchers` API is also interoperable with that interface; `matchers` is particularly helpful if you need to make assertions about elements within a slice or map.

If an assertion fails, you may wish to either continue and allow more failures to accumulate, or immediately stop the test. That is up to you based on whether the failure is significant enough that it would make the rest of the test moot. The `ldtest.T` method `Errorf` records a failure without stopping, and `FailNow` makes it immediately terminate. When using the `testify` packages, the `assert` functions will call only `Errorf` whereas the `require` functions will call both `Errorf` and `FailNow`, so `require` always implies "stop here on failure". The `matchers` API follows the same conventions with its `Assert` and `Require` methods.

## Making test failures clear

If an SDK test fails, someone will have to interpret the test output and find the problem. They should not need to dig through the test harness code to understand the failure message (although the output does include a stacktrace to make that possible). So, if for instance the assertion is that the `variationIndex` property in some event data should have value X, ideally a failed assertion would not just print "expected value X but was Y", but should include some context to clarify that this is a `variationIndex`.

There are several ways to do this. When using `testify/assert` or `testify/require`, the assertion functions have optional parameters for an additional message to be printed on failure.

Also, using the `matchers` API sometimes results in more helpful error messages. For instance, if you want to assert that every element in a slice has a certain property, using `matchers.Items` or `matchers.ItemsInAnyOrder` along with a matcher for the desired property is better than doing an equality test on the whole slice against the entire expected data set, because it won't care about any irrelevant properties, and because the property matcher will automatically give you an appropriate failure message. It's also better than doing a `for` loop over the slice and doing an assertion on each element, because you won't have to construct your own message to explain that the failure is for item 0, item 1, etc.

Values in failure output may be formatted differently in `matchers` versus `testify`: for instance, values of types that have a JSON representation are shown as JSON in `matchers`, rather than using the result of `fmt.Sprintf("%v", value)` as `testify` does in most cases. The easiest way to see which kind of output would be most helpful for a particular test is to cause a deliberate failure in the test and see what you get.

## Non-critical tests

Sometimes we may want to standardize some aspect of SDK behavior, but we know that many of the SDKs don't yet comply with this standard and we don't consider it to be mandatory, just desirable.

In that case, at the top of a test you can call `t.NonCritical()`. This marks it as a "non-critical" test: if it fails, the failure will be reported, but it will be labeled as a non-critical failure and it will _not_ cause the overall test run to fail. `NonCritical` takes a string parameter which is an explanatory message to be included in the failure output, to save the reader trouble in interpreting the failure details. For instance:

```go
    t.NonCritical("If this test fails but the other evaluation tests passed, it means the SDK"+
        " considers burnt sienna and puce to be the same color. We would prefer for them to be"+
        " treated as two different colors.")
```

When using the `-junit` command-line option to produce a JUnit-compatible results file, non-critical failures are reported as regular failures, since JUnit does not have a separate category for these, but they will have "(non-critical)" appended to the test name.
