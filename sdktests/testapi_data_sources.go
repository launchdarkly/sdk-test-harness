package sdktests

import (
	"os"
	"path/filepath"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"

	"github.com/stretchr/testify/require"
)

// DataSourceFile is a temporary file in the file data source format, created on a filesystem
// that is shared between the test harness and the SDK test service, like an OverrideFile. The
// file is deleted automatically when the test scope exits.
type DataSourceFile struct {
	// Path is the absolute path of the file, to be passed to the test service in
	// SDKConfigFileDataParams.FilePaths.
	Path string
}

// NewDataSourceFile creates a temporary JSON data file with the given initial contents.
func NewDataSourceFile(t *ldtest.T, contents string) *DataSourceFile {
	f, err := os.CreateTemp("", "sdk-test-harness-data*.json")
	require.NoError(t, err)
	path := f.Name()
	t.Defer(func() {
		_ = os.Remove(path)
	})
	_, writeErr := f.WriteString(contents)
	closeErr := f.Close()
	require.NoError(t, writeErr)
	require.NoError(t, closeErr)
	return &DataSourceFile{Path: path}
}

// Replace replaces the file's contents in one operation: it writes them to a new file in the same
// directory and renames that file over this one, as the SDKs' file data source documentation
// recommends, so that an SDK never reads a partly written file.
func (f *DataSourceFile) Replace(t *ldtest.T, contents string) {
	replacement, err := os.CreateTemp(filepath.Dir(f.Path), "sdk-test-harness-data-replacement*.json")
	require.NoError(t, err)
	replacementPath := replacement.Name()
	_, writeErr := replacement.WriteString(contents)
	closeErr := replacement.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(replacementPath)
	}
	require.NoError(t, writeErr)
	require.NoError(t, closeErr)
	require.NoError(t, os.Rename(replacementPath, f.Path))
}

// WithDataInitializer is used with StartSDKClient to add a data initializer configuration.
// Initializers are used in order.
func WithDataInitializer(initializer servicedef.DataInitializer) SDKConfigurer {
	return helpers.ConfigOptionFunc[servicedef.SDKConfigParams](func(configOut *servicedef.SDKConfigParams) error {
		dataSystem := configOut.DataSystem.OrElse(servicedef.DataSystem{})
		dataSystem.Initializers = append(dataSystem.Initializers, initializer)
		configOut.DataSystem = o.Some(dataSystem)
		return nil
	})
}

// WithFileInitializer is a convenience for adding the file data source as an initializer.
func WithFileInitializer(params servicedef.SDKConfigFileDataParams) SDKConfigurer {
	return WithDataInitializer(servicedef.DataInitializer{File: o.Some(params)})
}

// WithFileSynchronizer is a convenience for adding the file data source as a synchronizer.
func WithFileSynchronizer(params servicedef.SDKConfigFileDataParams) SDKConfigurer {
	return WithDataSynchronizer(servicedef.DataSynchronizer{File: o.Some(params)})
}

// WithTestDataSynchronizer is a convenience for adding the test data source as a synchronizer.
func WithTestDataSynchronizer(params servicedef.SDKConfigTestDataParams) SDKConfigurer {
	return WithDataSynchronizer(servicedef.DataSynchronizer{TestData: o.Some(params)})
}

// UpdateTestData tells the SDK client's test data source to serve a new value for a flag to every
// context. The test harness will only call this method if the test service has the
// "test-data-source" capability.
func (c *SDKClient) UpdateTestData(t *ldtest.T, flagKey string, value ldvalue.Value) {
	require.NoError(t, c.sdkClientEntity.SendCommandWithParams(
		servicedef.CommandParams{
			Command: servicedef.CommandUpdateTestData,
			UpdateTestData: o.Some(servicedef.UpdateTestDataParams{
				FlagKey: flagKey,
				Value:   value,
			}),
		},
		t.DebugLogger(),
		nil,
	))
}
