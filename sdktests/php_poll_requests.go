package sdktests

import (
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
)

func doPHPPollRequestTests(t *ldtest.T) {
	NewCommonPollingTests(t, "doPHPPollRequestTests").RequestUserAgent(t)
}
