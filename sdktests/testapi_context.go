package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
)

const defaultSDKKey = "test-sdk-key"

type SDKTestContext struct {
	harness *harness.TestHarness
	sdkKind mockld.SDKKind
}

func requireContext(t *ldtest.T) SDKTestContext {
	if c, ok := t.Context().(SDKTestContext); ok {
		return c
	}
	panic("SDKTestContext was not included in the global test configuration!" +
		" This is a basic mistake in the initialization logic.")
}
