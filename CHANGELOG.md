# Change log

All notable changes to the project will be documented in this file. This project adheres to [Semantic Versioning](http://semver.org).

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
