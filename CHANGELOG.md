# Change log

All notable changes to the project will be documented in this file. This project adheres to [Semantic Versioning](http://semver.org).

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
