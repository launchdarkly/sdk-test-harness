# Writing tests

(under construction)

Some test harnesses are entirely scripted-- that is, the test harness code contains only generic mechanisms, and tests are specified in file data that is fed to the harness. However, due to the wide variety of test scenarios we need to support, the domain-specific language for such scripts would have to be very complex. So, this tool does not take that approach; tests are written in Go, using a model that will be familiar to most Go programmers. Tests _can_ be parameterized using file data, but that is up to the author of each test or test suite.

## Conceptual model

Each test has a human-readable name. These should be concise, and ideally they should not change once defined, so that they can be referenced externally (for instance, to selectively run or disable certain tests in a script).

Any test can contain subtests. The parent test could be only a container for the subtests, or it could also perform some actions of its own. This is similar to how `t.Run(name, function)` is used in Go's `testing` package. Subtest names are concatenated with the parent test name delimited by slashes: `"top-level test name/subtest A/subtest A1 that is within A"`.

Every test function receives a parameter of type `*ldtest.T` that represents the scope of the test. Again this is very similar to Go's `testing.T`, and it implements some of the same basic interface methods such as `Errorf` and `FailNow`-- which means helper packages such as `testify/assert` can be used just as they would in regular Go tests.

As with `testing.T`, the test logic can signal any number of failures that accumulate and do not stop the test but mark it as failed; or it can signal a failure that immediately stops the test (implemented internally as a panic). Any other panic that occurs during a test also marks the test as failed, and does not propagate outside of that test.
