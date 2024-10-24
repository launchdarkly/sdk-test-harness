# Change log

All notable changes to the project will be documented in this file. This project adheres to [Semantic Versioning](http://semver.org).

## [2.24.1](https://github.com/launchdarkly/sdk-test-harness/compare/v2.24.0...v2.24.1) (2024-10-15)


### Bug Fixes

* Make value optional for unknown feature. ([#245](https://github.com/launchdarkly/sdk-test-harness/issues/245)) ([e656904](https://github.com/launchdarkly/sdk-test-harness/commit/e656904ebdf4534b1772181cbf2db3f33f1e9126))

## [2.24.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.23.0...v2.24.0) (2024-10-15)


### Features

* Add `client-prereq-events` capability ([#242](https://github.com/launchdarkly/sdk-test-harness/issues/242)) ([3172672](https://github.com/launchdarkly/sdk-test-harness/commit/317267255c61f4ebe5b5fc3e8bb02bdbc00e6cb6))

## [2.23.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.22.0...v2.23.0) (2024-10-08)


### Features

* Test that client-side SDKs send correct version in events. ([#240](https://github.com/launchdarkly/sdk-test-harness/issues/240)) ([0b4df84](https://github.com/launchdarkly/sdk-test-harness/commit/0b4df847992c29a22d6c4bf9a3f3c41f4f2c5276))

## [2.22.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.21.0...v2.22.0) (2024-09-23)


### Features

* Expose headers to allow access to 'date' header. ([#234](https://github.com/launchdarkly/sdk-test-harness/issues/234)) ([64c8b41](https://github.com/launchdarkly/sdk-test-harness/commit/64c8b41a44bca81fa9a668b2c1ae52d12c616940))

## [2.21.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.20.0...v2.21.0) (2024-09-13)


### Features

* Add CORS support. ([#232](https://github.com/launchdarkly/sdk-test-harness/issues/232)) ([29364a8](https://github.com/launchdarkly/sdk-test-harness/commit/29364a828da9deb38b45874895338a6693ab3fe0))

## [2.20.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.19.0...v2.20.0) (2024-09-05)


### Features

* Add tests which evaluate complex JSON variations. ([#230](https://github.com/launchdarkly/sdk-test-harness/issues/230)) ([ec7e2b7](https://github.com/launchdarkly/sdk-test-harness/commit/ec7e2b7fdc5afc4074110b47b62846a9f9ef13b8))

## [2.19.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.18.1...v2.19.0) (2024-08-30)


### Features

* Add filtering-strict capability ([#227](https://github.com/launchdarkly/sdk-test-harness/issues/227)) ([ef81430](https://github.com/launchdarkly/sdk-test-harness/commit/ef81430b9ab13dc084d4770d6b82f24b8d12f63c))

## [2.18.1](https://github.com/launchdarkly/sdk-test-harness/compare/v2.18.0...v2.18.1) (2024-08-02)


### Bug Fixes

* Fix gzip compression tests for mobile and client SDKs ([#225](https://github.com/launchdarkly/sdk-test-harness/issues/225)) ([fb6b73f](https://github.com/launchdarkly/sdk-test-harness/commit/fb6b73fc374c277a04eecd08aa619a1a5ea502a2))

## [2.18.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.17.0...v2.18.0) (2024-07-31)


### Features

* Add support for wrapper name and version tests. ([#223](https://github.com/launchdarkly/sdk-test-harness/issues/223)) ([b6c3878](https://github.com/launchdarkly/sdk-test-harness/commit/b6c38787a2e5ff94a3297b41d9252576bb4688cc))

## [2.17.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.16.1...v2.17.0) (2024-07-24)


### Features

* Add `optional-event-gzip` capability ([#221](https://github.com/launchdarkly/sdk-test-harness/issues/221)) ([2a78109](https://github.com/launchdarkly/sdk-test-harness/commit/2a781096f2f95cc620f24f3a7528225802d9723f))

## [2.16.1](https://github.com/launchdarkly/sdk-test-harness/compare/v2.16.0...v2.16.1) (2024-06-26)


### Bug Fixes

* Make identify tests more lenient. ([#219](https://github.com/launchdarkly/sdk-test-harness/issues/219)) ([f236016](https://github.com/launchdarkly/sdk-test-harness/commit/f23601636ad42f7788a2d01ac0210e7f88207e7a))

## [2.16.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.15.0...v2.16.0) (2024-06-26)


### Features

* Add tests for omit anonymous contexts. ([#217](https://github.com/launchdarkly/sdk-test-harness/issues/217)) ([1b9c6f6](https://github.com/launchdarkly/sdk-test-harness/commit/1b9c6f626487552f373589e97510cd3e46e1de6c))

## [2.15.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.14.0...v2.15.0) (2024-05-30)


### Features

* add custom CA capability ([#215](https://github.com/launchdarkly/sdk-test-harness/issues/215)) ([7e25226](https://github.com/launchdarkly/sdk-test-harness/commit/7e2522657ea585a122e0a55aa7670582e4182655))

## [2.14.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.13.0...v2.14.0) (2024-05-15)


### Features

* Add support for gzipped event payloads ([#213](https://github.com/launchdarkly/sdk-test-harness/issues/213)) ([91de9cb](https://github.com/launchdarkly/sdk-test-harness/commit/91de9cb4410790b6444aaa4e5b3ddce1f3e94da7))

## [2.13.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.12.0...v2.13.0) (2024-05-10)


### Features

* support testing TLS options with two new capabilities ([#208](https://github.com/launchdarkly/sdk-test-harness/issues/208)) ([6a90eb0](https://github.com/launchdarkly/sdk-test-harness/commit/6a90eb0a95f066fcf5d450ad11a45b325e5e306d))

## [2.12.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.11.0...v2.12.0) (2024-04-29)


### Features

* Expand hook tests to include client side SDKs ([#205](https://github.com/launchdarkly/sdk-test-harness/issues/205)) ([00412a0](https://github.com/launchdarkly/sdk-test-harness/commit/00412a0e60cf7b56e106e1f0074ebd54ad57569e))

## [2.11.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.10.0...v2.11.0) (2024-04-24)


### Features

* allow passing Github Token to downloader script ([#206](https://github.com/launchdarkly/sdk-test-harness/issues/206)) ([22b2ba6](https://github.com/launchdarkly/sdk-test-harness/commit/22b2ba604d88d7be77fd6fe215f42f78531dece0))

## [2.10.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.9.0...v2.10.0) (2024-04-03)


### Features

* test hook error handling ([#202](https://github.com/launchdarkly/sdk-test-harness/issues/202)) ([256ae92](https://github.com/launchdarkly/sdk-test-harness/commit/256ae92376cb877627fc99f0ac423e81262f0414))

## [2.9.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.8.3...v2.9.0) (2024-03-22)


### Features

* Add hooks contract tests. ([#200](https://github.com/launchdarkly/sdk-test-harness/issues/200)) ([53331bb](https://github.com/launchdarkly/sdk-test-harness/commit/53331bb4d437265bf6e4b897ebb6114b0611e5d6))

## [2.8.3](https://github.com/launchdarkly/sdk-test-harness/compare/v2.8.2...v2.8.3) (2024-02-22)


### Bug Fixes

* remove bootstrap-sha from release-please-config.json ([#196](https://github.com/launchdarkly/sdk-test-harness/issues/196)) ([7bc0a38](https://github.com/launchdarkly/sdk-test-harness/commit/7bc0a382446adc1181a7b9ddf6f8a8d3ce9f31ce))

## [2.8.2](https://github.com/launchdarkly/sdk-test-harness/compare/v2.8.1...v2.8.2) (2024-02-21)


### Bug Fixes

* **deps:** upgrade to Go to 1.22, upgrade golanglintci to 1.56 ([4159378](https://github.com/launchdarkly/sdk-test-harness/commit/41593789eaa2c29e8046bda2c82813565938cdb3))

## [2.8.1] - 2024-02-07
### Fixed:
- Closed the gzip writer which flushes the gzip footer. Previously the footer would have been missing.

## [2.8.0] - 2024-01-30
### Added:
- Add test to verify SDK polling behavior with `Accept-Encoding: gzip`.

## [2.7.0] - 2024-01-29
### Added:
- Added optional capability `client-independence` for SDKs that support multiple client instances being used at the same time.

## [2.6.0] - 2024-01-22
### Added:
- Added optional capability for sending inlined contexts in feature events.
- Added optional capability for redacting anonymous contexts in feature events.
- Added support for PHP sending event schema v4 formats.

## [2.5.0] - 2024-01-18
### Added:
- Added the ability to specify the timeout for the status query during startup.

## [2.4.1] - 2024-01-18
### Fixed:
- Fixes issue in custom events test in which server payloads were sent to client SDKs.

## [2.4.0] - 2023-12-29
### Added:
- Added testing and supporting capability for re-using e-tag headers across re-starts.

## [2.3.0] - 2023-12-20
### Added:
- Add test verifying PHP's behavior with summary exclusion.
- Add PHP support for migration tests.
- Add context-comparison capability for testing context equality.

## [2.2.1] - 2023-10-17
### Changed:
- Auto Environment Attributes tests to handle ld_device being absent in certain SDKs.

## [2.2.0] - 2023-10-13
### Added:
- Added new capabilities and tests associated with the upcoming technology migration support use case.

### Fixed:
- Added a missing user type capability guard to existing context conversion tests.

## [2.1.2] - 2023-08-30
### Fixed:
- Relaxing content-type for server events. Java will include a charset. This is not required for application/json (because it is UTF-8 by its own standard), but it isn't explicitly forbidden.

## [2.1.1] - 2023-08-24
### Added:
- Downloader support for windows.

### Fixed:
- Relaxing context type test, now contains application/json
- Updated tags tests to account for fallback when id is invalid

## [2.1] - 2023-08-15
### Added:
- Add polling test with large payload size.
- Add contract tests for auto-populated environment attributes.
- Verify event payloads contain the correct content-type header.
- Add test which matches a user context in a multi-context.
- Add test which validates negating segment match operations.

## [2.0.0] - 2023-04-13
## Changed:
- This release of the SDK Contract Tests marks the beginning of support for the generally available [Contexts](https://docs.launchdarkly.com/guides/flags/intro-contexts) feature.

## [1.14.0] - 2023-04-07
### Added:
- Added a test to ensure targets take precedence over rules in the evaluation algorithm.
- Added support for Roku SDK alternative endpoints.
- Expanded coverage for existing segment tests.

## [1.13.0] - 2023-01-31
### Added:
- Server-side tests for environment filtering feature, under capability "filtering".

## [1.12.1] - 2022-11-28
### Fixed:
- Fixed a bug that caused a nil pointer panic when testing summary events in a non-mobile client-side SDK.

## [1.12.0] - 2022-11-15
### Added:
- Client-side SDK tests for `feature`, `debug`, and `summary` events.

## [1.11.0] - 2022-10-05
### Added:
- Analytics event tests for the PHP SDK.

## [1.10.1] - 2022-10-04
### Fixed:
- The test coverage for valid vs. invalid date and semver values was inadequate. Parameterized evaluation tests now include more test cases and are more clearly organized by name, to distinguish between different kinds of logic errors. This may cause some existing SDKs that are not fully compliant with the evaluation spec to show new test failures.

## [1.10.0] - 2022-10-04
### Added:
- The test harness can now run evaluation tests against the LaunchDarkly PHP SDK, a special case of LaunchDarkly server-side SDKs.

## [1.9.0] - 2022-08-26
### Added:
- New optional server-side test for secure mode hash.

### Fixed:
- Made stream retry tests less timing-sensitive.

## [1.8.1] - 2022-08-23
### Changed:
- Speeded up some client-side tests by using `custom` events instead of `identify` (in cases where the type of event doesn't really matter).

## [1.8.0] - 2022-07-26
### Added:
- Test for allFlagsState method not generating events in server-side SDKs.

## [1.7.2] - 2022-06-22
### Changed:
- Client-side tests now automatically set a default initial user if the test logic did not specifically do so, since client-side SDKs cannot work without an initial user.

## [1.7.1] - 2022-06-15
### Fixed:
- Fixed overly timing-sensitive tests in `streaming/validation`.

## [1.7.0] - 2022-05-04
### Added:
- Client-side SDK tests for streaming updates, polling, and experimentation evaluations.

### Fixed:
- Suppressed misleading panic stacktrace output related to `httphelpers.BrokenConnectionHandler`.
- SDKs are allowed to include an `api_key` scheme identifier in `Authorization` headers.

## [1.6.2] - 2022-04-29
### Fixed:
- Fixed client-side SDK test expectations for "wrong type" errors.

## [1.6.1] - 2022-04-29
### Changed:
- Tests for application tag behavior now include a non-critical test of the 64-character length limit.

### Fixed:
- Expectations about the `Authorization` header now allow the optional `api_key` scheme identifier that some SDKs include.

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
