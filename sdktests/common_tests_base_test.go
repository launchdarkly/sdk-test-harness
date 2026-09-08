package sdktests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

// The service spec promises that useReport and usePost are never set in the same configuration, so
// a test service does not need to define a precedence between them.
func TestWithFlagRequestMethodNeverSetsBothUseReportAndUsePost(t *testing.T) {
	c := commonTestsBase{isClientSide: true}
	for _, method := range []flagRequestMethod{flagRequestGET, flagRequestREPORT, flagRequestPOST} {
		t.Run(string(method), func(t *testing.T) {
			var config servicedef.SDKConfigParams
			require.NoError(t, c.withFlagRequestMethod(method).Configure(&config))

			clientSide := config.ClientSide.Value()
			assert.False(t, clientSide.UseReport.Value() && clientSide.UsePost.Value())
			assert.Equal(t, method == flagRequestREPORT, clientSide.UseReport.Value())
			assert.Equal(t, method == flagRequestPOST, clientSide.UsePost.Value())
		})
	}
}
