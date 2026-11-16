package data

import (
	"regexp"
	"testing"

	"github.com/launchdarkly/sdk-test-harness/v2/data/testmodel"
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A reference to a constant or parameter, such as <TYPE_NAME>, that no substitution replaced.
var unfilledPlaceholder = regexp.MustCompile(`<[A-Z][A-Z0-9_]*>`)

func TestServerSideEvalDataFiles(t *testing.T) {
	checkEvalDataFiles[mockld.ServerSDKData](t, "server-side-eval")
}

func TestClientSideEvalDataFiles(t *testing.T) {
	checkEvalDataFiles[mockld.ClientSDKData](t, "client-side-eval")
}

// checkEvalDataFiles loads and parses every data file in a directory as the evaluation test
// suites do. It fails for a file that does not parse, a suite without a name or evaluations, an
// evaluation without a flag key, and a placeholder that no constant or parameter fills.
func checkEvalDataFiles[SDKDataType any](t *testing.T, dirName string) {
	sources, err := LoadAllDataFiles(dirName)
	require.NoError(t, err)
	require.NotEmpty(t, sources)

	for _, source := range sources {
		desc := source.BaseName + " " + source.ParamsString()

		var suite testmodel.EvalTestSuite[SDKDataType]
		if !assert.NoError(t, ParseJSONOrYAML(source.Data, &suite), desc) {
			continue
		}
		assert.NotEmpty(t, suite.Name, "suite name in %s", desc)
		assert.NotEmpty(t, suite.Evaluations, "evaluations in %s", desc)
		for i, eval := range suite.Evaluations {
			assert.NotEmpty(t, eval.FlagKey, "flag key of evaluation %d in %s", i, desc)
		}

		var fields map[string]any
		require.NoError(t, ParseJSONOrYAML(source.Data, &fields), desc)
		// The SDK data is not checked. A parameter set leaves the placeholders in the flags and
		// segments that it does not use, as in segment-recursion.yml.
		delete(fields, "sdkData")
		delete(fields, "constants")
		delete(fields, "parameters")
		assert.Empty(t, findPlaceholders(fields), "unfilled placeholders in %s", desc)
	}
}

// findPlaceholders returns every unfilled placeholder in the keys and string values of parsed
// JSON or YAML data.
func findPlaceholders(value any) []string {
	var found []string
	switch v := value.(type) {
	case string:
		found = append(found, unfilledPlaceholder.FindAllString(v, -1)...)
	case []any:
		for _, item := range v {
			found = append(found, findPlaceholders(item)...)
		}
	case map[string]any:
		for key, item := range v {
			found = append(found, unfilledPlaceholder.FindAllString(key, -1)...)
			found = append(found, findPlaceholders(item)...)
		}
	}
	return found
}
