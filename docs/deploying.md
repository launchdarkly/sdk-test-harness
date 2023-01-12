# How to deploy this tool

In a CI job for an SDK, the most convenient way to run the test suite is by invoking the `downloader/run.sh` script, which downloads the compiled executable and runs it. You can download this script directly from GitHub and pipe it to `bash` or `sh`. This is similar to how tools such as Goreleaser are normally run. You must set `VERSION` to the desired version of the tool, and `PARAMS` to the command-line parameters.

```shell
curl -s https://raw.githubusercontent.com/launchdarkly/sdk-test-harness/v2.0.0/downloader/run.sh \
  | VERSION=v2 PARAMS="--url http://localhost:8000" sh
```

In this example, `v2.0.0` in the URL is the version of the `run.sh` script to use. If there are any significant changes to the script, there will be a new major version, to ensure that CI jobs pinned to previous versions will not fail.

The `VERSION=v2` setting is what determines what version of the actual tests to use. It's best to specify only a major version so that you will automatically get any backward-compatible improvements in the test harness-- as long as you keep in mind that this might cause a build to fail that previously passed, if a more sensitive test is added (that is, if the test harness now detects a kind of noncompliance with the existing SDK specification that it did not previously check for). If you want to make sure your builds will never break due to such an improvement in the tests, you can instead pin to a specific version string such as `VERSION=v2.0.0`, but be aware that this could mean a bug is overlooked.
