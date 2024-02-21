# Developer notes

This page is for people doing development of the SDK test harness itself. See also the [general documentation](./docs/index.md) for how to use this tool, and how to write SDK test services for it.

## Tools used

To build and test the tool locally, you will need Go 1.22 or higher.

You do not need to install any other development tools used for the SDKs in order to build the test harness. Generally, each SDK project will include a corresponding test service which will be built using the same tools as that SDK.

## Building and testing

To compile all the non-test code: `make build` or just `make`. This builds the executable `sdk-test-harness` in the current directory.

To run unit tests: `make test`

Code linting: `make lint`. Please do this before pushing changes-- linter errors will make CI fail, and this project does not use `pre-commit`.
