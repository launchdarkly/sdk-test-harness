# Change log

All notable changes to the project will be documented in this file. This project adheres to [Semantic Versioning](http://semver.org).

## [1.6.0] - 2022-04-25
### Added:
- The test harness now supports testing client-side LaunchDarkly SDKs as well as server-side ones. The client-side test suite includes evaluation and event behavior, but is still missing test cases for some areas such as summary events, experimentation evaluations, and streaming updates.

### Fixed:
- Fixed a race condition in the test "`events/requests/new payload ID for each post`".

## [1.5.0] - 2022-04-14
### Added:
- Each release now includes binaries for the `arm64` architecture (these were added manually to the 1.4.0 release, but they will now be added automatically).
- Added a test for disabling events.
- Added opt-in "service endpoints" capability for SDKs that support this mechanism.

### Changed:
- The tool is now built with Go 1.18.

## [1.4.0] - 2022-04-12
### Added:
- Tests for basic HTTP behavior of analytics event posts (request path, headers, etc.).

## [1.3.0] - 2022-03-08
### Added:
- Tests for new SDK application metadata properties, enabled by the "tags" capability.

### Changed:
- Improved test coverage for private attributes in events.

## [1.2.0] - 2022-02-09
### Added:
- Command line options `-record-failures` and `-skip-from`.

## [1.1.6] - 2022-02-08
### Fixed:
- Analytics event tests no longer care about the order in which events appear in a payload; the order isn't of any significance to LaunchDarkly.

## [1.1.5] - 2022-02-04
### Fixed:
- Fixed a bug that could cause the program to crash with a panic when certain tests failed.

## [1.1.4] - 2022-02-03
### Fixed:
- Stacktraces now appear consistently for all failures. Previously they only appeared sometimes in console output (in a somewhat different format) and never appeared in JUnit output.
- Debug logging for a subtest now includes log output from components that were created in a parent test.
- In `evaluate` requests to the test service, `valueType` is always set.
- Duplicate event posts are ignored by default if they have the same `X-LaunchDarkly-Payload-Id` header value.

## [1.1.3] - 2022-02-01
### Fixed:
- Fixed excessive usage of sockets/file handles due to not always using Keep-Alive for HTTP requests.

## [1.1.2] - 2022-01-28
### Fixed:
- Many event-related tests have been rewritten for better separation of concerns, so that if the SDK behaves wrongly in a particular area such as the computation of user properties, the error will be more clearly visible in tests for that area and will not break other tests. Failure messages should now be clearer in general as well, due to changes in how the assertions are done.
- Fixed a bug that prevented the tool from running on Windows.

## [1.1.1] - 2022-01-27
### Fixed:
- The "all flags" tests now include test cases for experimentation behavior. There is a known issue in some of the SDKs where the "all flags" data has incorrect properties in these cases, so if contract test jobs start to fail on the `evaluation/all flags/experimentation` test when using this version, it is likely an actual SDK bug.

## [1.1.0] - 2022-01-27
### Added:
- For SDKs that support Big Segments, there are now tests for the non-database-specific parts of the Big Segments functionality, which are run if the test service includes "big-segments" in its capability list.
- New tests for the standard behavior of `feature` and `debug` events in analytics data.
- New `NonCritical` option allows for tests that can flag SDK inconsistencies without making the test run fail.

### Fixed:
- Fixed some incorrect expectations in `AllFlagsState` tests regarding the `version` property.
- Fixed spurious failures in some SDKs due to overly specific JSON expectations.
- The `downloader/run.sh` script now works correctly if `$PARAMS` contains strings in single quotes.

## [1.0.0] - 2022-01-24
First stable release of `sdk-test-harness`. See readme/docs for a detailed description of the functionality in this release.

Releases after this point will adhere to semantic versioning as follows:

* Patch release: fixing the behavior of existing tests in such a way that any new CI failures would reflect an actual SDK problem.
* Minor version release: adding a new test that either relies on existing test service capabilities, or will not be run unless the test service reports some new capability, in such a way that any new CI failures would reflect an actual SDK problem.
* Major version release: backward-incompatible changes that require test services to be modified before they will pass with this version.

## [1.0.0] - 2022-01-24
First stable release of `sdk-test-harness`. See readme/docs for a detailed description of the functionality in this release.

Releases after this point will adhere to semantic versioning as follows:

* Patch release: fixing the behavior of existing tests in such a way that any new CI failures would reflect an actual SDK problem.
* Minor version release: adding a new test that either relies on existing test service capabilities, or will not be run unless the test service reports some new capability, in such a way that any new CI failures would reflect an actual SDK problem.
* Major version release: backward-incompatible changes that require test services to be modified before they will pass with this version.
