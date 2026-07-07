package sdktests

import (
	"encoding/json"
	"os"

	"github.com/launchdarkly/sdk-test-harness/v2/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v2/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v2/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/require"
)

// OverrideFile is a temporary file containing a flag overrides document, created on a filesystem
// that is shared between the test harness and the SDK test service (the same arrangement used for
// the TLS custom CA file). The file is deleted automatically when the test scope exits.
type OverrideFile struct {
	// Path is the absolute path of the file, to be passed to the test service in
	// SDKConfigOverridesParams.FilePaths.
	Path string
}

// NewOverrideFile creates a temporary overrides file with the given initial contents.
func NewOverrideFile(t *ldtest.T, contents string) *OverrideFile {
	return newOverrideFileWithPattern(t, "sdk-test-harness-overrides*", contents)
}

// NewOverrideFileWithSuffix is the same as NewOverrideFile, but the file name ends with the given
// suffix (for instance ".yaml"), for SDKs that detect the document format from the file extension.
func NewOverrideFileWithSuffix(t *ldtest.T, suffix string, contents string) *OverrideFile {
	return newOverrideFileWithPattern(t, "sdk-test-harness-overrides*"+suffix, contents)
}

func newOverrideFileWithPattern(t *ldtest.T, pattern string, contents string) *OverrideFile {
	f, err := os.CreateTemp("", pattern)
	require.NoError(t, err)
	path := f.Name()
	t.Defer(func() {
		_ = os.Remove(path)
	})
	_, writeErr := f.WriteString(contents)
	closeErr := f.Close()
	require.NoError(t, writeErr)
	require.NoError(t, closeErr)
	return &OverrideFile{Path: path}
}

// Replace overwrites the file with new contents. This is deliberately a plain truncate-and-write
// rather than an atomic rename, because SDKs must tolerate non-atomic writes to override files.
func (f *OverrideFile) Replace(t *ldtest.T, contents string) {
	require.NoError(t, os.WriteFile(f.Path, []byte(contents), 0600))
}

// Clear overwrites the file with an empty overrides document.
func (f *OverrideFile) Clear(t *ldtest.T) {
	f.Replace(t, "{}")
}

// WithFileOverrides is used with StartSDKClient to enable the SDK's file-based flag overrides
// feature with the given parameters.
func WithFileOverrides(params servicedef.SDKConfigOverridesParams) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		configOut.Overrides = o.Some(params)
		return nil
	})
}

// evaluateFlagRawReasonResponse is the same as servicedef.EvaluateFlagResponse except that the
// reason is kept as raw JSON. The typed ldreason.EvaluationReason drops JSON properties it does
// not know about-- such as the "isOverride" marker set by the flag overrides feature-- so tests
// that need to assert on such properties must use this representation instead.
type evaluateFlagRawReasonResponse struct {
	Value          ldvalue.Value   `json:"value"`
	VariationIndex o.Maybe[int]    `json:"variationIndex,omitempty"`
	Reason         json.RawMessage `json:"reason,omitempty"`
}

// evaluateFlagDetailRawReason is the same as SDKClient.EvaluateFlag with Detail enabled, except
// that the reason in the response is kept as raw JSON (see evaluateFlagRawReasonResponse).
func evaluateFlagDetailRawReason(
	t *ldtest.T,
	client *SDKClient,
	params servicedef.EvaluateFlagParams,
) evaluateFlagRawReasonResponse {
	if params.ValueType == "" {
		params.ValueType = servicedef.ValueTypeAny
	}
	params.Detail = true
	var resp evaluateFlagRawReasonResponse
	require.NoError(t, client.sdkClientEntity.SendCommandWithParams(
		servicedef.CommandParams{
			Command:  servicedef.CommandEvaluateFlag,
			Evaluate: o.Some(params),
		},
		t.DebugLogger(),
		&resp,
	))
	return resp
}
