package sdktests

import (
	"time"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
)

// nonASCIIFlagValue is a string with 2-, 3- and 4-byte UTF-8 sequences: an e with an acute accent, two
// CJK characters, and an emoji outside the Basic Multilingual Plane.
const nonASCIIFlagValue = "caf\u00e9 \u6f22\u5b57 \U0001F600"

// NonASCIIValues verifies that flag values with multi-byte UTF-8 characters are not corrupted when they
// arrive in a stream put or patch event.
func (c CommonStreamingTests) NonASCIIValues(t *ldtest.T) {
	context := ldcontext.New("context-key")
	flagKey := "non-ascii-flag"
	expectedValue := ldvalue.String(nonASCIIFlagValue)
	defaultValue := ldvalue.String("default")

	t.Run("non-ASCII flag value in put event is preserved", func(t *ldtest.T) {
		stream, configurers := c.setupDataSources(t, c.makeSDKDataWithFlag(flagKey, 1, expectedValue))
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
		_ = requireStreamConnection(t, stream.Endpoint(), time.Second*5)

		m.In(t).Assert(basicEvaluateFlag(t, client, flagKey, context, defaultValue), m.Equal(expectedValue))
	})

	t.Run("non-ASCII flag value in patch event is preserved", func(t *ldtest.T) {
		valueBefore := ldvalue.String("ascii")
		stream, configurers := c.setupDataSources(t, c.makeSDKDataWithFlag(flagKey, 1, valueBefore))
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(configurers...)...)
		_ = requireStreamConnection(t, stream.Endpoint(), time.Second*5)
		m.In(t).Assert(basicEvaluateFlag(t, client, flagKey, context, defaultValue), m.Equal(valueBefore))

		stream.StreamingService().PushUpdate("flags", flagKey, c.makeFlagData(flagKey, 2, expectedValue))

		pollUntilFlagValueUpdated(t, client, flagKey, context, valueBefore, expectedValue, defaultValue)
	})
}

// NonASCIIValues verifies that flag values with multi-byte UTF-8 characters are not corrupted when they
// arrive in a poll response.
func (c CommonPollingTests) NonASCIIValues(t *ldtest.T) {
	t.Run("non-ASCII flag value is preserved", func(t *ldtest.T) {
		flagKey := "non-ascii-flag"
		expectedValue := ldvalue.String(nonASCIIFlagValue)
		// The streaming tests build flag data in the same way for each kind of SDK.
		data := CommonStreamingTests(c).makeSDKDataWithFlag(flagKey, 1, expectedValue)
		dataSource := NewSDKDataSource(t, data, DataSourceOptionPolling())
		client := NewSDKClient(t, c.baseSDKConfigurationPlus(dataSource)...)
		_ = dataSource.Endpoint().RequireConnection(t, time.Second*5)

		m.In(t).Assert(basicEvaluateFlag(t, client, flagKey, ldcontext.New("context-key"), ldvalue.String("default")),
			m.Equal(expectedValue))
	})
}
