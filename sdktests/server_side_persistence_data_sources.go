package sdktests

// These tests verify which data the SDK writes to a persistent store in read-write mode. The data
// system spec requires each data source to state whether its data is persisted (requirements
// 1.1.7 and 1.2.8): LaunchDarkly data is persisted, and file data is not, so that a file does not
// replace the LaunchDarkly data in a store that other SDK instances read. The SDK's test data
// source is not persisted by default either. Both can be configured to request persistence.
//
// A test of data that must not be persisted runs against an empty store, and against a store
// seeded with LaunchDarkly data, as a store shared with other SDK instances would hold. It checks
// that the store keeps exactly what it held.

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

	// The service spec requires a file synchronizer to reload the files within this time of a change.
	fileReloadWindow = 10 * time.Second

	// How long a test watches the store for a write that must not happen.
	unwantedWriteWindow = time.Second

	// How long a test watches the store for a write back that must not happen after the store
	// recovers. An SDK checks the store's availability about every 500 ms, then writes back.
	unwantedWriteBackWindow = 3 * time.Second
)

// A test of data that must not be persisted runs once for each of these starting states.
var dataSourceStoreStates = []struct { //nolint:gochecknoglobals
	name string
	seed bool
}{
	{"empty store", false},
	{"store with LaunchDarkly data", true},
}

func (s *ServerSidePersistentTests) runDataSourcePersistenceTests(t *ldtest.T) {
	context := ldcontext.New("user-key")
	defaultValue := ldvalue.String("default")

	t.Run("file data", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityFileDataSource)

		for _, state := range dataSourceStoreStates {
			t.Run(state.name, func(t *ldtest.T) {
				s.runWithEmptyStore(t, "initializer does not write to the store", func(t *ldtest.T) {
					s.prepareStore(t, state.seed)
					file := NewDataSourceFile(t, s.fileDataDocument(t, 1, "file-value"))

					client := s.newDataSourceClient(t, WithFileInitializer(servicedef.SDKConfigFileDataParams{
						FilePaths: []string{file.Path},
					}))

					pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
						defaultValue, ldvalue.String("file-value"), defaultValue)
					s.requireStoreUnchanged(t, state.seed, unwantedWriteWindow)
				})

				s.runWithEmptyStore(t, "synchronizer does not write to the store, including reloads", func(t *ldtest.T) {
					s.prepareStore(t, state.seed)
					file := NewDataSourceFile(t, s.fileDataDocument(t, 1, "file-value"))

					client := s.newDataSourceClient(t, WithFileSynchronizer(servicedef.SDKConfigFileDataParams{
						FilePaths: []string{file.Path},
					}))

					pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
						defaultValue, ldvalue.String("file-value"), defaultValue)

					file.Replace(t, s.fileDataDocument(t, 2, "reloaded-value"))
					s.requireFileReload(t, client, context, "file-value", "reloaded-value")

					s.requireStoreUnchanged(t, state.seed, unwantedWriteWindow)
				})
			})
		}

		s.runWithEmptyStore(t, "synchronizer does not replace LaunchDarkly data that the SDK wrote", func(t *ldtest.T) {
			ldData := s.fdv2ServerData("xfer-full", "payload-missing", "initial",
				s.makeServerSideFlag("flag-key", 1, ldvalue.String("value")))
			dataSystem := NewSDKDataSystemCustom(t, ldData, DataSystemOptionPollingInitializer(ldData))
			dataSystem.CreateEndpoints()
			file := NewDataSourceFile(t, s.fileDataDocument(t, 1, "file-value"))

			client := s.newDataSourceClient(t, dataSystem, WithFileSynchronizer(servicedef.SDKConfigFileDataParams{
				FilePaths: []string{file.Path},
			}))

			// The polling initializer's LaunchDarkly data is written to the store. The file data that
			// replaces it in memory must not replace it in the store.
			pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
				defaultValue, ldvalue.String("file-value"), defaultValue)
			ldFlags := map[string]m.Matcher{"flag-key": basicFlagValidationMatcher("flag-key", 1, "value")}
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, ldFlags)
			s.neverInvalidateFlagData(t, ldFlags, unwantedWriteWindow,
				"the file data replaced the LaunchDarkly data in the store")
		})

		s.runWithEmptyStore(t, "LaunchDarkly data after a file initializer is written to the store", func(t *ldtest.T) {
			file := NewDataSourceFile(t, s.fileDataDocument(t, 1, "file-value"))

			// The streaming synchronizer holds its response until the test has checked that the
			// file data did not reach the store. Its basis would otherwise replace file data that
			// the SDK wrongly wrote, and hide the write.
			data := mockld.NewServerSDKDataBuilder().
				Flag(s.makeServerSideFlag("flag-key", 1, ldvalue.String("value"))).Build()
			dataSystem := NewSDKDataSystemWithoutEndpoints(t, data)
			endpoint, release := heldEndpoint(t, dataSystem.Synchronizers[0].streaming, "streaming synchronizer")

			// The client does not wait to start: an SDK may report that it is ready only after the
			// held synchronizer's basis.
			client := s.newDataSourceClient(t,
				WithWaitToStart(time.Millisecond, true),
				WithFileInitializer(servicedef.SDKConfigFileDataParams{FilePaths: []string{file.Path}}),
				WithStreamingSynchronizer(baseStreamConfig(endpoint)))
			t.Defer(release)

			pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
				defaultValue, ldvalue.String("file-value"), defaultValue)
			s.requireStoreUnchanged(t, false, unwantedWriteWindow)

			// The synchronizer's basis replaces the file data, and is written to the store.
			release()
			pollUntilFlagValueUpdated(t, client, "flag-key", context,
				defaultValue, ldvalue.String("value"), defaultValue)
			s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
			s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
				"flag-key": basicFlagValidationMatcher("flag-key", 1, "value"),
			})
		})

		t.Run("store recovery", func(t *ldtest.T) {
			t.RequireCapability(servicedef.CapabilityPersistentDataStoreRecovery)

			s.runWithEmptyStore(t, "does not write file data back to the store", func(t *ldtest.T) {
				s.runFileDataRecoveryTest(t, context, defaultValue)
			})
		})

		t.Run("persistence requested", func(t *ldtest.T) {
			t.RequireCapability(servicedef.CapabilityDataSourcePersistOptIn)

			s.runWithEmptyStore(t, "initializer writes to the store", func(t *ldtest.T) {
				file := NewDataSourceFile(t, s.fileDataDocument(t, 1, "file-value"))

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
				file := NewDataSourceFile(t, s.fileDataDocument(t, 1, "file-value"))

				_ = s.newDataSourceClient(t, WithFileSynchronizer(servicedef.SDKConfigFileDataParams{
					FilePaths: []string{file.Path},
					Persist:   o.Some(true),
				}))

				s.eventuallyRequireDataStoreInit(t, s.defaultPrefix)
				s.eventuallyValidateFlagData(t, s.defaultPrefix, map[string]m.Matcher{
					dataSourceFlagKey: persistedFlagMatcher("file-value"),
				})

				file.Replace(t, s.fileDataDocument(t, 2, "reloaded-value"))
				s.eventuallyValidateFlagDataWithin(t, fileReloadWindow, map[string]m.Matcher{
					dataSourceFlagKey: persistedFlagMatcher("reloaded-value"),
				})
			})

			s.runWithEmptyStore(t, "initializer does not write to a read-only store", func(t *ldtest.T) {
				s.prepareStore(t, true)
				file := NewDataSourceFile(t, s.fileDataDocument(t, 1, "file-value"))

				client := s.newDataSourceClientInMode(t, servicedef.DataStoreModeRead,
					WithFileInitializer(servicedef.SDKConfigFileDataParams{
						FilePaths: []string{file.Path},
						Persist:   o.Some(true),
					}))

				pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
					defaultValue, ldvalue.String("file-value"), defaultValue)
				s.requireStoreUnchanged(t, true, unwantedWriteWindow)
			})
		})
	})

	t.Run("test data", func(t *ldtest.T) {
		t.RequireCapability(servicedef.CapabilityTestDataSource)

		for _, state := range dataSourceStoreStates {
			t.Run(state.name, func(t *ldtest.T) {
				s.runWithEmptyStore(t, "does not write to the store, including updates", func(t *ldtest.T) {
					s.prepareStore(t, state.seed)

					client := s.newDataSourceClient(t, WithTestDataSynchronizer(servicedef.SDKConfigTestDataParams{
						Flags: map[string]ldvalue.Value{dataSourceFlagKey: ldvalue.String("test-value")},
					}))

					pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
						defaultValue, ldvalue.String("test-value"), defaultValue)

					client.UpdateTestData(t, dataSourceFlagKey, ldvalue.String("updated-value"))
					pollUntilFlagValueUpdated(t, client, dataSourceFlagKey, context,
						ldvalue.String("test-value"), ldvalue.String("updated-value"), defaultValue)

					s.requireStoreUnchanged(t, state.seed, unwantedWriteWindow)
				})
			})
		}

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

// runFileDataRecoveryTest checks that an SDK whose in-memory data last came from a file does not write
// that data back to the store when the store recovers from an outage (TXNS requirement 1.4.3.2).
//
// The SDK detects an outage only when a store operation fails, and it never writes file data. So the
// test holds a LaunchDarkly polling initializer's response until the store is unreachable: the SDK's
// write of that basis fails, and the SDK then receives the file synchronizer's data. The store is
// reachable when the client starts, because some store clients fail to construct otherwise.
func (s *ServerSidePersistentTests) runFileDataRecoveryTest(
	t *ldtest.T, context ldcontext.Context, defaultValue ldvalue.Value,
) {
	s.prepareStore(t, true)
	proxy, err := harness.NewTCPProxy(s.persistentStore.Addr())
	require.NoError(t, err)
	t.Defer(proxy.Close)

	ldData := s.fdv2ServerData("xfer-full", "payload-missing", "initial",
		s.makeServerSideFlag("flag-key", 1, ldvalue.String("value")))
	dataSystem := NewSDKDataSystemWithoutEndpoints(t, ldData, DataSystemOptionPollingInitializer(ldData))
	endpoint, release := heldEndpoint(t, dataSystem.Initializers[0].pollingService, "polling initializer")
	file := NewDataSourceFile(t, s.fileDataDocument(t, 1, "file-value"))

	client := s.newDataSourceClientWithDSN(t, servicedef.DataStoreModeReadWrite,
		s.persistentStore.DSNFor(proxy.Addr()),
		WithWaitToStart(time.Millisecond, true),
		WithDataInitializer(servicedef.DataInitializer{
			Polling: o.Some(servicedef.SDKConfigPollingParams{BaseURI: endpoint.BaseURL()}),
		}),
		WithFileSynchronizer(servicedef.SDKConfigFileDataParams{FilePaths: []string{file.Path}}))
	// Deferred cleanups run last-in-first-out: the stream is released and the store is reachable
	// again before the client closes.
	t.Defer(proxy.Restore)
	t.Defer(release)

	proxy.Break()
	release()
	h.RequireEventually(t,
		checkForUpdatedValue(t, client, dataSourceFlagKey, context, defaultValue, ldvalue.String("file-value"), defaultValue),
		fileReloadWindow, time.Millisecond*50, "the SDK did not serve the file data")

	// Keep the outage open long enough for the SDK's write of the LaunchDarkly basis to fail.
	time.Sleep(minOutageDuration)
	proxy.Restore()

	s.requireStoreUnchanged(t, true, unwantedWriteBackWindow)
}

// heldEndpoint serves the handler through a mock endpoint that holds every request until release is
// called. Release can be called more than once.
func heldEndpoint(t *ldtest.T, handler http.Handler, description string) (*harness.MockEndpoint, func()) {
	held := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(held) }) }
	endpoint := requireContext(t).harness.NewMockEndpoint(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-held:
				handler.ServeHTTP(w, r)
			case <-r.Context().Done():
			}
		}),
		t.DebugLogger(),
		harness.MockEndpointDescription(description+" held until released"))
	t.Defer(endpoint.Close)
	return endpoint, release
}

// newDataSourceClient starts an SDK client with an uncached persistent store in read-write mode and
// the given data sources.
func (s *ServerSidePersistentTests) newDataSourceClient(t *ldtest.T, configurers ...SDKConfigurer) *SDKClient {
	return s.newDataSourceClientInMode(t, servicedef.DataStoreModeReadWrite, configurers...)
}

// newDataSourceClientInMode is newDataSourceClient with the given store mode.
func (s *ServerSidePersistentTests) newDataSourceClientInMode(
	t *ldtest.T, mode servicedef.DataStoreMode, configurers ...SDKConfigurer,
) *SDKClient {
	return s.newDataSourceClientWithDSN(t, mode, s.persistentStore.DSN(), configurers...)
}

// newDataSourceClientWithDSN is newDataSourceClientInMode with the given store address.
func (s *ServerSidePersistentTests) newDataSourceClientWithDSN(
	t *ldtest.T, mode servicedef.DataStoreMode, dsn string, configurers ...SDKConfigurer,
) *SDKClient {
	persistence := NewPersistence()
	persistence.SetStore(servicedef.SDKConfigPersistentStore{
		Type: s.persistentStore.Type(),
		DSN:  dsn,
	})
	persistence.SetStoreMode(mode)
	persistence.SetCache(servicedef.SDKConfigPersistentCache{Mode: servicedef.CacheModeOff})

	return NewSDKClient(t, s.baseSDKConfigurationPlus(append(configurers, persistence)...)...)
}

// fileDataDocument returns a file data source document with one flag, which serves the given value
// to every context.
func (s *ServerSidePersistentTests) fileDataDocument(t *ldtest.T, version int, value string) string {
	flag := s.makeServerSideFlag(dataSourceFlagKey, version, ldvalue.String(value))
	document, err := json.Marshal(map[string]interface{}{
		"flags": map[string]interface{}{dataSourceFlagKey: flag},
	})
	require.NoError(t, err)
	return string(document)
}

// requireFileReload waits until the file data source serves the reloaded value.
func (s *ServerSidePersistentTests) requireFileReload(
	t *ldtest.T, client *SDKClient, context ldcontext.Context, previousValue, reloadedValue string,
) {
	h.RequireEventually(t,
		checkForUpdatedValue(t, client, dataSourceFlagKey, context,
			ldvalue.String(previousValue), ldvalue.String(reloadedValue), ldvalue.String("default")),
		fileReloadWindow, time.Millisecond*50, "the file data source did not reload the changed file")
}

// prepareStore seeds the store with LaunchDarkly data, as another SDK instance that shares the store
// would, or leaves it empty.
func (s *ServerSidePersistentTests) prepareStore(t *ldtest.T, seed bool) {
	if !seed {
		return
	}
	require.NoError(t, s.persistentStore.WriteMap(s.defaultPrefix, "features", s.initialFlags))
	require.NoError(t, s.persistentStore.Write(s.defaultPrefix, persistenceInitedKey, "1"))
}

// requireStoreUnchanged verifies that the store keeps what prepareStore left in it, for the given
// time: the seeded LaunchDarkly data, or nothing.
func (s *ServerSidePersistentTests) requireStoreUnchanged(t *ldtest.T, seeded bool, window time.Duration) {
	expected := map[string]string{}
	if seeded {
		expected = s.initialFlags
	}
	storeChanged := func() bool {
		data, err := s.persistentStore.GetMap(s.defaultPrefix, "features")
		require.NoError(t, err, "could not read the store")
		if len(data) != len(expected) || (len(expected) > 0 && !reflect.DeepEqual(data, expected)) {
			return true
		}
		inited, err := s.persistentStore.Get(s.defaultPrefix, persistenceInitedKey)
		require.NoError(t, err, "could not read the store")
		return inited.IsDefined() != seeded
	}
	h.RequireNever(t, storeChanged, window, time.Millisecond*20,
		"the SDK wrote data that must not be persisted to the store")
}

// neverInvalidateFlagData verifies that the store's flags keep matching the matchers for the given time.
func (s *ServerSidePersistentTests) neverInvalidateFlagData(
	t *ldtest.T, matchers map[string]m.Matcher, window time.Duration, message string,
) {
	h.RequireNever(t, func() bool {
		data, err := s.persistentStore.GetMap(s.defaultPrefix, "features")
		require.NoError(t, err, "could not read the store")
		return !validateFlagData(data, matchers)
	}, window, time.Millisecond*20, message)
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
