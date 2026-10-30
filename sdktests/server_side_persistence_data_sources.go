package sdktests

// These tests verify which data the SDK writes to a persistent store in read-write mode. The data
// system spec requires each data source to state whether its data is persisted (requirements
// 1.1.7 and 1.2.8): LaunchDarkly data is persisted, and file data is not, so that a file does not
// replace the LaunchDarkly data in a store that other SDK instances read. The SDK's test data
// source is not persisted by default either. Both can be configured to request persistence.
//
// A test of data that must not be persisted seeds the store with LaunchDarkly data, as a store
// shared with other SDK instances would hold, and checks that the store keeps exactly that data.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/stretchr/testify/require"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	h "github.com/launchdarkly/sdk-test-harness/v3/framework/helpers"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	o "github.com/launchdarkly/sdk-test-harness/v3/framework/opt"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

const (
	dataSourceFlagKey = "data-source-flag-key"

	// A file synchronizer reloads after it detects a change. SDKs detect changes by watching the
	// files or by polling them, so the window allows a few seconds.
	fileReloadWindow = 5 * time.Second

	// How long a test watches the store for a write that must not happen.
	unwantedWriteWindow = time.Second
)

func (s *ServerSidePersistentTests) runDataSourcePersistenceTests(t *ldtest.T) {
	context := ldcontext.New("user-key")
	defaultValue := ldvalue.String("default")

	t.Run("file data", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityFileDataSource)

		s.runWithEmptyStore(t, "initializer does not write to the store", func(t *ldtest.T) {
			s.seedStoreWithLaunchDarklyData(t)
			file := NewDataSourceFile(t, s.fileDataDocument(t, "file-value"))

			client := s.newDataSourceClient(t, WithFileInitializer(servicedef.SDKConfigFileDataParams{
				FilePaths: []string{file.Path},
			}))

			pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
				defaultValue, ldvalue.String("file-value"), defaultValue)
			s.requireStoreKeepsLaunchDarklyData(t)
		})

		s.runWithEmptyStore(t, "synchronizer does not write to the store, including reloads", func(t *ldtest.T) {
			s.seedStoreWithLaunchDarklyData(t)
			file := NewDataSourceFile(t, s.fileDataDocument(t, "file-value"))

			client := s.newDataSourceClient(t, WithFileSynchronizer(servicedef.SDKConfigFileDataParams{
				FilePaths: []string{file.Path},
			}))

			pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
				defaultValue, ldvalue.String("file-value"), defaultValue)

			file.Replace(t, s.fileDataDocument(t, "reloaded-value"))
			h.RequireEventually(t,
				checkForUpdatedValue(t, client, dataSourceFlagKey, context,
					ldvalue.String("file-value"), ldvalue.String("reloaded-value"), defaultValue),
				fileReloadWindow, time.Millisecond*50, "the file data source did not reload the changed file")

			s.requireStoreKeepsLaunchDarklyData(t)
		})

		s.runWithEmptyStore(t, "LaunchDarkly data after a file initializer is written to the store", func(t *ldtest.T) {
			file := NewDataSourceFile(t, s.fileDataDocument(t, "file-value"))

			// The streaming synchronizer holds its response until the test has checked that the
			// file data did not reach the store. Its basis would otherwise replace file data that
			// the SDK wrongly wrote, and hide the write.
			data := mockld.NewServerSDKDataBuilder().
				Flag(s.makeServerSideFlag("flag-key", 1, ldvalue.String("value"))).Build()
			dataSystem := NewSDKDataSystemWithoutEndpoints(t, data)
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseStream := func() { releaseOnce.Do(func() { close(release) }) }
			endpoint := requireContext(t).harness.NewMockEndpoint(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case <-release:
						dataSystem.Synchronizers[0].streaming.ServeHTTP(w, r)
					case <-r.Context().Done():
					}
				}),
				t.DebugLogger(),
				harness.MockEndpointDescription("streaming synchronizer held until released"))
			t.Defer(endpoint.Close)
			t.Defer(releaseStream)

			client := s.newDataSourceClient(t,
				WithFileInitializer(servicedef.SDKConfigFileDataParams{FilePaths: []string{file.Path}}),
				WithStreamingSynchronizer(baseStreamConfig(endpoint)))

			pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
				defaultValue, ldvalue.String("file-value"), defaultValue)
			h.RequireNever(t, func() bool {
				data, err := s.persistentStore.GetMap(s.defaultPrefix, "features")
				require.NoError(t, err, "could not read the store")
				inited, err := s.persistentStore.Get(s.defaultPrefix, persistenceInitedKey)
				require.NoError(t, err, "could not read the store")
				return len(data) > 0 || inited.IsDefined()
			}, unwantedWriteWindow, time.Millisecond*20, "the SDK wrote the file data to the store")

			// The synchronizer's basis replaces the file data, and is written to the store.
			releaseStream()
			pollUntilFlagValueUpdated(t, client, "flag-key", context,
				defaultValue, ldvalue.String("value"), defaultValue)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key": basicFlagValidationMatcher("flag-key", 1, "value"),
			})
		})

		t.Run("persistence requested", func(t *ldtest.T) {
			t.RequireCapability(servicedef.CapabilityDataSourcePersistOptIn)

			s.runWithEmptyStore(t, "initializer writes to the store", func(t *ldtest.T) {
				file := NewDataSourceFile(t, s.fileDataDocument(t, "file-value"))

				_ = s.newDataSourceClient(t, WithFileInitializer(servicedef.SDKConfigFileDataParams{
					FilePaths: []string{file.Path},
					Persist:   o.Some(true),
				}))

				s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
				s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
					dataSourceFlagKey: persistedFlagMatcher("file-value"),
				})
			})

			s.runWithEmptyStore(t, "synchronizer writes reloads to the store", func(t *ldtest.T) {
				file := NewDataSourceFile(t, s.fileDataDocument(t, "file-value"))

				_ = s.newDataSourceClient(t, WithFileSynchronizer(servicedef.SDKConfigFileDataParams{
					FilePaths: []string{file.Path},
					Persist:   o.Some(true),
				}))

				s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
				s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
					dataSourceFlagKey: persistedFlagMatcher("file-value"),
				})

				file.Replace(t, s.fileDataDocument(t, "reloaded-value"))
				s.eventuallyValidateFlagDataWithin(t, fileReloadWindow, map[string]m.Matcher{
					dataSourceFlagKey: persistedFlagMatcher("reloaded-value"),
				})
			})
		})
	})

	t.Run("test data", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityTestDataSource)

		s.runWithEmptyStore(t, "does not write to the store, including updates", func(t *ldtest.T) {
			s.seedStoreWithLaunchDarklyData(t)

			client := s.newDataSourceClient(t, WithTestDataSynchronizer(servicedef.SDKConfigTestDataParams{
				Flags: map[string]ldvalue.Value{dataSourceFlagKey: ldvalue.String("test-value")},
			}))

			pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
				defaultValue, ldvalue.String("test-value"), defaultValue)

			client.UpdateTestData(t, dataSourceFlagKey, ldvalue.String("updated-value"))
			pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
				ldvalue.String("test-value"), ldvalue.String("updated-value"), defaultValue)

			s.requireStoreKeepsLaunchDarklyData(t)
		})

		t.Run("persistence requested", func(t *ldtest.T) {
			t.RequireCapability(servicedef.CapabilityDataSourcePersistOptIn)

			s.runWithEmptyStore(t, "writes to the store, including updates", func(t *ldtest.T) {
				client := s.newDataSourceClient(t, WithTestDataSynchronizer(servicedef.SDKConfigTestDataParams{
					Flags:   map[string]ldvalue.Value{dataSourceFlagKey: ldvalue.String("test-value")},
					Persist: o.Some(true),
				}))

				s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
				s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
					dataSourceFlagKey: persistedFlagMatcher("test-value"),
				})

				client.UpdateTestData(t, dataSourceFlagKey, ldvalue.String("updated-value"))
				s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
					dataSourceFlagKey: persistedFlagMatcher("updated-value"),
				})
			})
		})
	})
}

// newDataSourceClient starts an SDK client with an uncached persistent store in read-write mode
// and the given data sources.
func (s *ServerSidePersistentTests) newDataSourceClient(t *ldtest.T, configurers ...SDKConfigurer) *SDKClient {
	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  s.persistentStore.DSN(),
	})
	persistence.SetStoreMode(servicedef.DataStoreModeReadWrite)
	persistence.SetCache(servicedef.SDKConfigPersistentCache{Mode: servicedef.CacheModeOff})

	return NewSDKClient(t, s.baseSDKConfigurationPlus(append(configurers, persistence)...)...)
}

// fileDataDocument returns a file data source document with one flag, which serves the given value
// to every context.
func (s *ServerSidePersistentTests) fileDataDocument(t *ldtest.T, value string) string {
	flag := s.makeServerSideFlag(dataSourceFlagKey, 1, ldvalue.String(value))
	document, err := json.Marshal(map[string]interface{}{
		"flags": map[string]interface{}{dataSourceFlagKey: flag},
	})
	require.NoError(t, err)
	return string(document)
}

// seedStoreWithLaunchDarklyData writes LaunchDarkly data to the store, as another SDK instance
// that shares the store would.
func (s *ServerSidePersistentTests) seedStoreWithLaunchDarklyData(t *ldtest.T) {
	require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))
	require.NoError(t, s.persistentStore.Write(s.defaultPrefix, persistenceInitedKey, "1"))
}

// requireStoreKeepsLaunchDarklyData verifies that the store keeps the data that
// seedStoreWithLaunchDarklyData wrote, unchanged, for a while.
func (s *ServerSidePersistentTests) requireStoreKeepsLaunchDarklyData(t *ldtest.T) {
	storeChanged := func() bool {
		data, err := s.persistentStore.GetMap(s.defaultPrefix, "features")
		require.NoError(t, err, "could not read the store")
		return !reflect.DeepEqual(data, s.initialFlags)
	}
	h.RequireNever(t, storeChanged, unwantedWriteWindow, time.Millisecond*20,
		"the SDK wrote data that must not be persisted to the store")

	data, err := s.persistentStore.GetMap(s.defaultPrefix, "features")
	require.NoError(t, err)
	require.Equal(t, s.initialFlags, data, "the store does not hold the LaunchDarkly data it was seeded with")
}

// eventuallyValidateFlagDataWithin is eventuallyValidateFlagData with a given window.
func (s *ServerSidePersistentTests) eventuallyValidateFlagDataWithin(
	t *ldtest.T, window time.Duration, matchers map[string]m.Matcher,
) {
	h.RequireEventually(t, func() bool {
		data, err := s.persistentStore.GetMap(s.defaultPrefix, "features")
		if err != nil {
			return false
		}
		return validateFlagData(data, matchers)
	}, window, time.Millisecond*50, "flag data did not match")
}

// persistedFlagMatcher matches the stored representation of the data source flag, serving the
// given string value. The data source decides the rest of the representation, such as the version
// and the other variations.
func persistedFlagMatcher(value string) m.Matcher {
	return m.AllOf(
		m.JSONProperty("key").Should(m.Equal(dataSourceFlagKey)),
		m.StringContains(fmt.Sprintf("%q", value)),
	)
}
