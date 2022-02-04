# Running the tests

## Test harness command line

```shell
./sdk-test-harness -url <test service base URL> [other options]
```

Options besides `-url`:

* `-host <NAME>` - sets the hostname to use in callback URLs, if not the same as the host the test service is running on (default: localhost)
* `-port <PORT>` - sets the callback port that test services will connect to (default: 8111)
* `-run <PATTERN>` - skips any tests whose names do not match the specified pattern (can specify more than one)
* `-skip <PATTERN>` - skips any tests whose names match the specified pattern (can specify more than one) 
* `-stop-service-at-end` - tells the test service to exit after the test run
* `-junit <FILEPATH>` - writes test results in JUnit XML format to the specified file
* `-debug` - enables verbose logging of test actions for failed tests
* `-debug-all` - enables verbose logging of test actions for all tests
* `-record-failures` - record test failures to the given file. Test failures recorded can be skipped by the next run of 
the test harness via -suppress-failures.
* `-suppress-failures` - path to test failures recorded by `-record-failures`. The test harness will automatically skip any tests contained in the file.

For `-run` and `-skip`, the rules for pattern matching are as follows:

* The match is done against the full path of the test. The full path is the string that appears between brackets in the test output. It may include slash-delimited subtests, such as `parent test name/subtest name/sub-subtest name`.
* In the pattern, each slash-delimited segment is treated as a separate regular expression. So, for instance, you can write `^evaluation$/a.c` to match `evaluation/abc` and `evaluation/xaxcx` but not match `xevaluationx/abc`.
* However, `(` and `)` will always be treated as literal characters, not regular expression operators. This is because some tests may have parens in their names. If you want "or" behavior, instead of `-run (a|b)` just use `-run a -run b` (in other words, there is an implied "or" for multiple values).
* If `-run` specifies a test that has subtests, then all of its subtests are also run.
* If `-skip` specifies a test that has subtests, then all of its subtests are also skipped.

## Output

While tests are running, when each test starts a line is printed to standard output with the full path of the test in brackets, such as:

```
[evaluation/all flags state/with reasons]
```

If the test is skipped, you will see an explanation starting with `SKIPPED:` on the next line. A test could be skipped because of filter parameters like `-run` or `-skip`, or due to `-suppress-failures`, or because the test is only for SDKs with a certain capability and the test service did not report having that capability.

If the test failed, you will see `FAILED:` and a failure message, which normally includes both a description of the problem and a stacktrace. The stacktrace refers to the `sdk-test-harness` code and may be helpful in understanding the test logic.

If you see `FAILED (non-critical):`, it means that the test failed but that the author of the test indicated SDKs don't necessarily need to pass it, by calling `t.NonCritical` within the test. This is normally accompanied by an explanatory message to help understand what kind of changes in the SDK's behavior are desirable.

At the end, if all tests passed-- not counting tests that were skipped-- the output is "All tests passed" and the program returns a zero exit code.

If some tests failed, it writes a summary of first the non-critical failures and then the regular failures to standard error. The program returns a non-zero exit code if there were any regular failures.

### JUnit output for CircleCI

When running in CircleCI, you will probably also want to create a JUnit-compatible test results file, since CircleCI knows how to parse the JUnit format. Do this by adding `-junit my_file_name.xml` to the command-line parameters, and make sure your CI job includes a directive like:

```yaml
      - store_test_results:
          path: my_file_name.xml
```

Then you'll be able to see test failures called out individually on the Tests tab for the CI run.

If there are non-critical failures, they will show up as failures in the results file since JUnit does not have a separate category for these. However, they will have "(non-critical)" appended to the name to make this clearer, and since the test harness still returns a zero exit code as long as all the failures were non-critical, the CI job will still pass.
