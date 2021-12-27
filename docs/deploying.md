# How to deploy this tool

In a CI job for an SDK, the most convenient way to run the test suite is by invoking the `downloader/run.sh` script, which downloads the compiled executable and runs it. You can download this script directly from GitHub and pipe it to `bash` or `sh`. This is similar to how tools such as Goreleaser are normally run. You must set `VERSION` to the desired version of the tool, and `PARAMS` to the command-line parameters.

```shell
curl -s https://raw.githubusercontent.com/launchdarkly/sdk-test-harness/master/downloader/run.sh \
  | VERSION=v1 PARAMS="--url http://localhost:8000" sh
```

You can specify an exact version string such as `v1.0.0` in `VERSION`, but it is better to specify only a major version so that you will automatically get any backward-compatible improvements in the test harness.
