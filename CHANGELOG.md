# Change log

All notable changes to the project will be documented in this file. This project adheres to [Semantic Versioning](http://semver.org).

## [2.41.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.40.0...v2.41.0) (2026-08-19)


### Features

* Verify per-context private attributes are not applied to other contexts ([#430](https://github.com/launchdarkly/sdk-test-harness/issues/430)) ([4d9b3a6](https://github.com/launchdarkly/sdk-test-harness/commit/4d9b3a6a2a98bb3d36c5be99650787bc9e0534bd))

## [2.40.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.39.0...v2.40.0) (2026-08-19)


### Features

* add RETRY-conformance contract tests for FDv1 streaming and polling ([#404](https://github.com/launchdarkly/sdk-test-harness/issues/404)) ([ee4d85f](https://github.com/launchdarkly/sdk-test-harness/commit/ee4d85f5847ba039b36f9f90a3e338f330011997))


### Bug Fixes

* Retry the test harness download and surface curl errors ([#411](https://github.com/launchdarkly/sdk-test-harness/issues/411)) ([2a7533b](https://github.com/launchdarkly/sdk-test-harness/commit/2a7533b4ea4ed3d660cb42ea7b30262ce6203da2))

## [2.39.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.38.2...v2.39.0) (2026-08-12)


### Features

* add client-side streaming retry and error-recovery contract tests ([#395](https://github.com/launchdarkly/sdk-test-harness/issues/395)) ([5e512a0](https://github.com/launchdarkly/sdk-test-harness/commit/5e512a0c338947dc345e959c6e8cb7364ff921d1))
* Add contract test for streaming connection close on client shutdown ([#377](https://github.com/launchdarkly/sdk-test-harness/issues/377)) ([35acfd4](https://github.com/launchdarkly/sdk-test-harness/commit/35acfd48c2730c266de7311bfb1578f592dd5f05))
* Add hook-environment-id capability and test ([#410](https://github.com/launchdarkly/sdk-test-harness/issues/410)) ([dae3bcd](https://github.com/launchdarkly/sdk-test-harness/commit/dae3bcd357ca45e5a04d8ee2542e1f1e438cd406))

## [2.38.2](https://github.com/launchdarkly/sdk-test-harness/compare/v2.38.1...v2.38.2) (2026-07-21)


### Bug Fixes

* Correct privateAttributes JSON tag in context comparison params ([#391](https://github.com/launchdarkly/sdk-test-harness/issues/391)) ([a45f514](https://github.com/launchdarkly/sdk-test-harness/commit/a45f51497e63004f8bcdd786540908d53d31ffb6))

## [2.38.1](https://github.com/launchdarkly/sdk-test-harness/compare/v2.38.0...v2.38.1) (2026-07-21)


### Bug Fixes

* Only expect custom-event anonymous redaction for server SDKs ([#388](https://github.com/launchdarkly/sdk-test-harness/issues/388)) ([c25e087](https://github.com/launchdarkly/sdk-test-harness/commit/c25e087bd8c099ab58f540f96eba5680d7adc5df))

## [2.38.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.37.0...v2.38.0) (2026-07-20)


### Features

* Add contract tests for client-side secure mode hash (h query param) ([#358](https://github.com/launchdarkly/sdk-test-harness/issues/358)) ([16ed598](https://github.com/launchdarkly/sdk-test-harness/commit/16ed5984cecd66ddb8066e34c5465ec2efb97d5c))
* Add migration op event context redaction tests ([#383](https://github.com/launchdarkly/sdk-test-harness/issues/383)) ([daa7ad4](https://github.com/launchdarkly/sdk-test-harness/commit/daa7ad4b39ed38f99aebcb6026c22b921c1c50ea))
* Contract tests for client-side prerequisite cycle detection ([#384](https://github.com/launchdarkly/sdk-test-harness/issues/384)) ([3605eb7](https://github.com/launchdarkly/sdk-test-harness/commit/3605eb7146c949968e2d365eacf6110ac45adbf4))


### Bug Fixes

* Fix capability guarding custom event redaction ([#386](https://github.com/launchdarkly/sdk-test-harness/issues/386)) ([357898f](https://github.com/launchdarkly/sdk-test-harness/commit/357898fdab0d9c2ba287bb92d1324c80c5f37b85))

## [2.37.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.36.0...v2.37.0) (2026-06-08)


### Features

* Verify the in operator keeps booleans and numbers distinct ([2943c1d](https://github.com/launchdarkly/sdk-test-harness/commit/2943c1db3a37f5fa548505150c1be712d6eb5d80))

## [2.36.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.35.0...v2.36.0) (2026-05-15)


### Features

* Assert hook execution order for evaluation and track stages ([#341](https://github.com/launchdarkly/sdk-test-harness/issues/341)) ([ecea002](https://github.com/launchdarkly/sdk-test-harness/commit/ecea00274a72ea91132e7c4b5bd9486d4943b472))

## [2.35.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.34.0...v2.35.0) (2026-04-22)


### Features

* Include hook coverage for PHP tests ([#331](https://github.com/launchdarkly/sdk-test-harness/issues/331)) ([78e9a56](https://github.com/launchdarkly/sdk-test-harness/commit/78e9a56583ca1e3b5e71a95d3780a8365126baaa))


### Bug Fixes

* js client sdk header tests can match auth headers ([#316](https://github.com/launchdarkly/sdk-test-harness/issues/316)) ([01f2610](https://github.com/launchdarkly/sdk-test-harness/commit/01f26104ea3742ca5d994f6a52f597b64470ccc4))

## [2.34.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.33.0...v2.34.0) (2026-02-23)


### Features

* Expand `http-proxy` capability to test server-side ([#318](https://github.com/launchdarkly/sdk-test-harness/issues/318)) ([2b75c1d](https://github.com/launchdarkly/sdk-test-harness/commit/2b75c1dca0620e05b28ec4586faf1036e2a32d72))

## [2.33.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.32.1...v2.33.0) (2025-10-31)


### Features

* Add test count summary to display total and skipped tests ([#307](https://github.com/launchdarkly/sdk-test-harness/issues/307)) ([7662be6](https://github.com/launchdarkly/sdk-test-harness/commit/7662be6e1ccf7102f1478a8a912dfcad740246f1))

## [2.32.1](https://github.com/launchdarkly/sdk-test-harness/compare/v2.32.0...v2.32.1) (2025-10-17)


### Bug Fixes

* Only use h.RequireNever, not require.Never. ([#305](https://github.com/launchdarkly/sdk-test-harness/issues/305)) ([8b95fa5](https://github.com/launchdarkly/sdk-test-harness/commit/8b95fa5d446417d6cb2c159a7ecf628a68645832))

## [2.32.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.31.0...v2.32.0) (2025-04-25)


### Features

* Add `track-hooks` capability ([#299](https://github.com/launchdarkly/sdk-test-harness/issues/299)) ([dfd6ed6](https://github.com/launchdarkly/sdk-test-harness/commit/dfd6ed6f7bf197692660025262ef18d3fbdfccf0))

## [2.31.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.30.2...v2.31.0) (2025-04-22)


### Features

* Add support for client-side per-context summary events. ([#294](https://github.com/launchdarkly/sdk-test-harness/issues/294)) ([0a225b3](https://github.com/launchdarkly/sdk-test-harness/commit/0a225b36c7082a87aeab04a5d207fc555df1c14a))

## [2.30.2](https://github.com/launchdarkly/sdk-test-harness/compare/v2.30.1...v2.30.2) (2025-04-15)


### Bug Fixes

* Adjust application tag tests to consider auto env capability ([#292](https://github.com/launchdarkly/sdk-test-harness/issues/292)) ([faab725](https://github.com/launchdarkly/sdk-test-harness/commit/faab725f00d393ece15572f74bc66c3205d48af0))

## [2.30.1](https://github.com/launchdarkly/sdk-test-harness/compare/v2.30.0...v2.30.1) (2025-03-21)


### Bug Fixes

* Fix generation of `tags` capability tests ([#290](https://github.com/launchdarkly/sdk-test-harness/issues/290)) ([9046560](https://github.com/launchdarkly/sdk-test-harness/commit/90465605267fff8b6d5e24187e62236db759d59c))

## [2.30.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.29.3...v2.30.0) (2025-03-20)


### Features

* Add `instance-id` capability to ensure headers are set ([#287](https://github.com/launchdarkly/sdk-test-harness/issues/287)) ([a889661](https://github.com/launchdarkly/sdk-test-harness/commit/a889661caae870f22365947474500f6912edc1b4))

## [2.29.3](https://github.com/launchdarkly/sdk-test-harness/compare/v2.29.2...v2.29.3) (2025-03-18)


### Bug Fixes

* Add missing capability guard to custom event test ([#285](https://github.com/launchdarkly/sdk-test-harness/issues/285)) ([8464e9f](https://github.com/launchdarkly/sdk-test-harness/commit/8464e9ffd0f93920a13cc8181967097277d12491))

## [2.29.2](https://github.com/launchdarkly/sdk-test-harness/compare/v2.29.1...v2.29.2) (2025-03-18)


### Bug Fixes

* Add missing capability guard to custom event test ([#283](https://github.com/launchdarkly/sdk-test-harness/issues/283)) ([8fcb395](https://github.com/launchdarkly/sdk-test-harness/commit/8fcb395efec927ab6fd5972b57e3ac00110f75cf))

## [2.29.1](https://github.com/launchdarkly/sdk-test-harness/compare/v2.29.0...v2.29.1) (2025-03-13)


### Bug Fixes

* Replace custom event redaction test with appropriate capability ([#281](https://github.com/launchdarkly/sdk-test-harness/issues/281)) ([ce976cd](https://github.com/launchdarkly/sdk-test-harness/commit/ce976cda29a40ca0708a919f325e275c59b75441))

## [2.29.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.28.0...v2.29.0) (2025-03-13)


### Features

* Inline context for custom and migration op events ([#278](https://github.com/launchdarkly/sdk-test-harness/issues/278)) ([c5d13d1](https://github.com/launchdarkly/sdk-test-harness/commit/c5d13d1633cac4ea26cdd06d1306330ca7461d5c))

## [2.28.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.27.0...v2.28.0) (2025-01-15)


### Features

* Expand big segment support to PHP ([#276](https://github.com/launchdarkly/sdk-test-harness/issues/276)) ([0f4591a](https://github.com/launchdarkly/sdk-test-harness/commit/0f4591abc22dac807fd6de9f96c0fb959bb14614))

## [2.27.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.26.0...v2.27.0) (2024-11-19)


### Features

* add http proxy test for client-side events ([#265](https://github.com/launchdarkly/sdk-test-harness/issues/265)) ([dfec55f](https://github.com/launchdarkly/sdk-test-harness/commit/dfec55fda44142fd16f5755fcc098e78fad15a1c))
* add http proxy test for client-side polling ([#264](https://github.com/launchdarkly/sdk-test-harness/issues/264)) ([1ef3bc2](https://github.com/launchdarkly/sdk-test-harness/commit/1ef3bc261d1de20cf9a2e13789a4865a12d4c2f3))

## [2.26.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.25.1...v2.26.0) (2024-11-18)


### Features

* add streaming mode http proxy test ([#258](https://github.com/launchdarkly/sdk-test-harness/issues/258)) ([b4e149e](https://github.com/launchdarkly/sdk-test-harness/commit/b4e149e1b5beca3c2bdb9b996678277c540bde23))

## [2.25.1](https://github.com/launchdarkly/sdk-test-harness/compare/v2.25.0...v2.25.1) (2024-11-06)


### Bug Fixes

* Allow for more flexible flag key in tombstone ([#256](https://github.com/launchdarkly/sdk-test-harness/issues/256)) ([2a0e8e7](https://github.com/launchdarkly/sdk-test-harness/commit/2a0e8e7293696b0c5934209cc8072897b6a8156e))

## [2.25.0](https://github.com/launchdarkly/sdk-test-harness/compare/v2.24.2...v2.25.0) (2024-10-31)


### Features

* Introduce persistent store testing  support ([#254](https://github.com/launchdarkly/sdk-test-harness/issues/254)) ([cd03f57](https://github.com/launchdarkly/sdk-test-harness/commit/cd03f57382f6e1a16d1aa289aeaf5e614557d63d))

## [2.24.2](https://github.com/launchdarkly/sdk-test-harness/compare/v2.24.1...v2.24.2) (2024-10-29)


### Bug Fixes

* don't send 'null' when no prereqs present ([#251](https://github.com/launchdarkly/sdk-test-harness/issues/251)) ([6480865](https://github.com/launchdarkly/sdk-test-harness/commit/6480865eb7f7c51d3e57c243e10dc4392036ca3c))
* summary events should allow null default value in counter ([#253](https://github.com/launchdarkly/sdk-test-harness/issues/253)) ([9b4d98f](https://github.com/launchdarkly/sdk-test-harness/commit/9b4d98ffd06aece34a8def9ddb2ba574946e053a))

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
