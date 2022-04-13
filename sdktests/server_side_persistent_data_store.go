package sdktests

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/mockld"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
	cf "github.com/launchdarkly/sdk-test-harness/servicedef/callbackfixtures"

	m "github.com/launchdarkly/go-test-helpers/v2/matchers"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"

	"github.com/stretchr/testify/assert"
)

const flagsKind, segmentsKind = "features", "segments"

type dataStoreCachingParams struct {
	name           string
	storeParams    servicedef.SDKConfigPersistentDataStoreParams
	shouldBeCached bool
}

func makeAllDataStoreCachingParams() []dataStoreCachingParams {
	return []dataStoreCachingParams{
		{
			name:           "uncached",
			storeParams:    servicedef.SDKConfigPersistentDataStoreParams{CacheTimeMS: ldvalue.NewOptionalInt(0)},
			shouldBeCached: false,
		},
		{
			name:           "cached explicitly",
			storeParams:    servicedef.SDKConfigPersistentDataStoreParams{CacheTimeMS: ldvalue.NewOptionalInt(100000)},
			shouldBeCached: true,
		},
		{
			name:           "cached by default",
			storeParams:    servicedef.SDKConfigPersistentDataStoreParams{},
			shouldBeCached: true,
		},
	}
}

type basicSerializedItem interface {
	getKey() string
	getVersion() int
	dataMatcher() m.Matcher
}

type basicSerializedFlag struct {
	index, version int
}

func (f basicSerializedFlag) getKey() string {
	return fmt.Sprintf("flag%d.%d", f.index, f.version)
}

func (f basicSerializedFlag) getVersion() int { return f.version }

func (f basicSerializedFlag) json() json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"key":%q,"version":%d,"variations":["var%d.%d"]}`,
		f.getKey(), f.version, f.index, f.version))
}

func (f basicSerializedFlag) dataMatcher() m.Matcher {
	return m.JSONMap().Should(m.MapIncluding(
		m.KV("key", m.Equal(f.getKey())),
		m.KV("version", m.Equal(f.version)),
		m.KV("variations", m.Items(m.Equal(fmt.Sprintf("var%d.%d", f.index, f.version)))),
	))
}

type basicSerializedSegment struct {
	index, version int
}

func (s basicSerializedSegment) getKey() string {
	return fmt.Sprintf("segment%d.%d", s.index, s.version)
}

func (s basicSerializedSegment) getVersion() int { return s.version }

func (s basicSerializedSegment) json() json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"key":%q,"version":%d,"included":["key%d.%d"]}`,
		s.getKey(), s.version, s.index, s.version))
}

func (s basicSerializedSegment) dataMatcher() m.Matcher {
	return m.JSONMap().Should(m.MapIncluding(
		m.KV("key", m.Equal(s.getKey())),
		m.KV("version", m.Equal(s.version)),
		m.KV("included", m.Items(m.Equal(fmt.Sprintf("key%d.%d", s.index, s.version)))),
	))
}

func doServerSidePersistentDataStoreTests(t *ldtest.T) {
	t.Run("init", doServerSidePersistentDataStoreInitTests)
	t.Run("get", doServerSidePersistentDataStoreGetTests)
	t.Run("getAll", doServerSidePersistentDataStoreGetAllTests)
}

func doServerSidePersistentDataStoreInitTests(t *ldtest.T) {
	var dm serializedDataMatchers

	t.Run("called for each stream put event", func(t *ldtest.T) {
		flag1, flag2 := basicSerializedFlag{1, 1}, basicSerializedFlag{2, 1}
		segment1, segment2 := basicSerializedSegment{1, 1}, basicSerializedSegment{2, 1}
		initialData := mockld.NewServerSDKDataBuilder().
			RawFlag(flag1.getKey(), flag1.json()).
			RawFlag(flag2.getKey(), flag2.json()).
			RawSegment(segment1.getKey(), segment1.json()).
			RawSegment(segment2.getKey(), segment2.json()).
			Build()
		dataSource := NewSDKDataSource(t, initialData)
		store := NewPersistentDataStore(t)
		initCh := store.SetupInitCapture()
		_ = NewSDKClient(t, dataSource, store)

		data1 := requireValue(t, initCh, time.Second*5)
		m.In(t).For("initial data").Require(data1.Data, m.Items(
			m.AllOf(
				dm.collectionKind().Should(m.Equal(segmentsKind)),
				dm.collectionItems().Should(m.ItemsInAnyOrder(
					dm.keyedItemIs(segment1),
					dm.keyedItemIs(segment2),
				)),
			),
			m.AllOf(
				dm.collectionKind().Should(m.Equal(flagsKind)),
				dm.collectionItems().Should(m.ItemsInAnyOrder(
					dm.keyedItemIs(flag1),
					dm.keyedItemIs(flag2),
				)),
			),
		))

		flag1v2 := flag1
		flag1v2.version = 2
		segment3 := basicSerializedSegment{3, 1}
		newData := mockld.NewServerSDKDataBuilder().
			RawFlag(flag1v2.getKey(), flag1v2.json()).
			RawSegment(segment3.getKey(), segment3.json()).
			Build()

		dataSource.streamingService.PushInit(newData)

		data2 := requireValue(t, initCh, time.Second*5)
		m.In(t).For("updated data").Assert(data2.Data, m.Items(
			m.AllOf(
				dm.collectionKind().Should(m.Equal(segmentsKind)),
				dm.collectionItems().Should(m.ItemsInAnyOrder(
					dm.keyedItemIs(segment3),
				)),
			),
			m.AllOf(
				dm.collectionKind().Should(m.Equal(flagsKind)),
				dm.collectionItems().Should(m.ItemsInAnyOrder(
					dm.keyedItemIs(flag1v2),
				)),
			),
		))
	})
}

func doServerSidePersistentDataStoreGetTests(t *ldtest.T) {
	expectedValue, wrongValue, defaultValue := ldvalue.String("yes"), ldvalue.String("no"), ldvalue.String("default")
	user := lduser.NewUser("userkey")

	basicFlag := ldbuilders.NewFlagBuilder("flagkey").
		Version(1).
		Variations(wrongValue, expectedValue).
		On(true).
		FallthroughVariation(1).
		Build()

	basicSegment := ldbuilders.NewSegmentBuilder("segmentkey").
		Version(1).
		Included(user.GetKey()).
		Build()

	flagForSegment := makeFlagToCheckSegmentMatch("flagkey", basicSegment.Key, wrongValue, expectedValue)

	// Note: in tests where we do not want caching to be a factor, we deliberately use an *empty*
	// initial data set to ensure that nothing is in the cache yet.
	emptyDataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())

	for _, cacheParams := range makeAllDataStoreCachingParams() {
		t.Run(cacheParams.name, func(t *ldtest.T) {
			t.Run("called to get flag for evaluation", func(t *ldtest.T) {
				dataForGet := mockld.NewServerSDKDataBuilder().Flag(basicFlag).Build()
				store := NewPersistentDataStore(t)
				_ = store.SetupInitCapture()
				paramsCh := store.SetupGetCapture(dataForGet)

				client := NewSDKClient(t, emptyDataSource, WithPersistentDataStoreConfig(cacheParams.storeParams), store)

				result := basicEvaluateFlag(t, client, basicFlag.Key, user, defaultValue)

				params := requireValue(t, paramsCh, time.Second*5)
				assert.Equal(t, flagsKind, params.Kind)
				assert.Equal(t, basicFlag.Key, params.Key)

				m.In(t).Assert(result, m.JSONEqual(expectedValue))
			})

			t.Run("called to get segment for evaluation", func(t *ldtest.T) {
				dataForGet := mockld.NewServerSDKDataBuilder().Flag(flagForSegment).Segment(basicSegment).Build()
				store := NewPersistentDataStore(t)
				_ = store.SetupInitCapture()
				paramsCh := store.SetupGetCapture(dataForGet)

				client := NewSDKClient(t, emptyDataSource, WithPersistentDataStoreConfig(cacheParams.storeParams), store)

				result := basicEvaluateFlag(t, client, basicFlag.Key, user, defaultValue)

				params1 := requireValue(t, paramsCh, time.Second*5)
				assert.Equal(t, flagsKind, params1.Kind)
				assert.Equal(t, flagForSegment.Key, params1.Key)

				params2 := requireValue(t, paramsCh, time.Second*5)
				assert.Equal(t, segmentsKind, params2.Kind)
				assert.Equal(t, basicSegment.Key, params2.Key)

				m.In(t).Assert(result, m.JSONEqual(expectedValue))
			})

			t.Run("re-evaluating flag that was already retrieved", func(t *ldtest.T) {
				dataForGet := mockld.NewServerSDKDataBuilder().Flag(basicFlag).Build()
				store := NewPersistentDataStore(t)
				_ = store.SetupInitCapture()
				paramsCh := store.SetupGetCapture(dataForGet)

				client := NewSDKClient(t, emptyDataSource, WithPersistentDataStoreConfig(cacheParams.storeParams), store)

				result1 := basicEvaluateFlag(t, client, basicFlag.Key, user, defaultValue)
				result2 := basicEvaluateFlag(t, client, basicFlag.Key, user, defaultValue)
				m.In(t).Assert(result1, m.JSONEqual(expectedValue))
				m.In(t).Assert(result2, m.JSONEqual(expectedValue))

				params1 := requireValue(t, paramsCh, time.Second*5)
				assert.Equal(t, flagsKind, params1.Kind)
				assert.Equal(t, basicFlag.Key, params1.Key)

				if cacheParams.shouldBeCached {
					requireNoMoreValues(t, paramsCh, time.Millisecond*10)
				} else {
					params2 := requireValue(t, paramsCh, time.Millisecond*10)
					assert.Equal(t, params1, params2)
				}
			})

			t.Run("re-evaluating segment that was already retrieved", func(t *ldtest.T) {
				dataForGet := mockld.NewServerSDKDataBuilder().Flag(flagForSegment).Segment(basicSegment).Build()
				store := NewPersistentDataStore(t)
				_ = store.SetupInitCapture()
				paramsCh := store.SetupGetCapture(dataForGet)

				client := NewSDKClient(t, emptyDataSource, WithPersistentDataStoreConfig(cacheParams.storeParams), store)

				result1 := basicEvaluateFlag(t, client, flagForSegment.Key, user, defaultValue)
				result2 := basicEvaluateFlag(t, client, flagForSegment.Key, user, defaultValue)
				m.In(t).Assert(result1, m.JSONEqual(expectedValue))
				m.In(t).Assert(result2, m.JSONEqual(expectedValue))

				params1 := requireValue(t, paramsCh, time.Second*5)
				assert.Equal(t, flagsKind, params1.Kind)
				assert.Equal(t, flagForSegment.Key, params1.Key)
				params2 := requireValue(t, paramsCh, time.Second*5)
				assert.Equal(t, segmentsKind, params2.Kind)
				assert.Equal(t, basicSegment.Key, params2.Key)

				if cacheParams.shouldBeCached {
					requireNoMoreValues(t, paramsCh, time.Millisecond*10)
				} else {
					params3 := requireValue(t, paramsCh, time.Millisecond*10)
					params4 := requireValue(t, paramsCh, time.Millisecond*10)
					assert.Equal(t, params1, params3)
					assert.Equal(t, params2, params4)
				}
			})

			t.Run("get flag that was previously stored by init", func(t *ldtest.T) {
				dataWithFlag := mockld.NewServerSDKDataBuilder().Flag(basicFlag).Build()
				dataSource := NewSDKDataSource(t, dataWithFlag) // flag is present at init time
				store := NewPersistentDataStore(t)
				_ = store.SetupInitCapture()
				paramsCh := store.SetupGetCapture(mockld.NewServerSDKDataBuilder().
					Flag(basicFlag).Build())

				client := NewSDKClient(t, dataSource, WithPersistentDataStoreConfig(cacheParams.storeParams), store)

				result := basicEvaluateFlag(t, client, basicFlag.Key, user, defaultValue)
				m.In(t).Assert(result, m.JSONEqual(expectedValue))

				if cacheParams.shouldBeCached {
					requireNoMoreValues(t, paramsCh, time.Millisecond*10)
				} else {
					params := requireValue(t, paramsCh, time.Second*5)
					assert.Equal(t, flagsKind, params.Kind)
					assert.Equal(t, basicFlag.Key, params.Key)
				}
			})

			t.Run("get unknown flag", func(t *ldtest.T) {
				store := NewPersistentDataStore(t)
				_ = store.SetupInitCapture()
				paramsCh := store.SetupGetCapture(mockld.EmptyServerSDKData())

				client := NewSDKClient(t, emptyDataSource, WithPersistentDataStoreConfig(cacheParams.storeParams), store)

				detail := evaluateFlagDetail(t, client, basicFlag.Key, user, defaultValue)

				params := requireValue(t, paramsCh, time.Second*5)
				assert.Equal(t, flagsKind, params.Kind)
				assert.Equal(t, basicFlag.Key, params.Key)

				m.In(t).Assert(detail.Value, m.JSONEqual(defaultValue))
				m.In(t).Assert(detail.Reason, EqualReason(ldreason.NewEvalReasonError(ldreason.EvalErrorFlagNotFound)))
			})
		})
	}
}

func doServerSidePersistentDataStoreGetAllTests(t *ldtest.T) {
	expectedValue1, expectedValue2, wrongValue :=
		ldvalue.String("value1"), ldvalue.String("value2"), ldvalue.String("no")
	user := lduser.NewUser("userkey")

	flag1 := ldbuilders.NewFlagBuilder("flag1").
		Version(1).
		Variations(wrongValue, expectedValue1).
		On(true).
		FallthroughVariation(1).
		Build()
	flag2 := ldbuilders.NewFlagBuilder("flag2").
		Version(1).
		Variations(wrongValue, expectedValue2).
		On(true).
		FallthroughVariation(1).
		Build()

	emptyDataSource := NewSDKDataSource(t, mockld.EmptyServerSDKData())

	for _, cacheParams := range makeAllDataStoreCachingParams() {
		t.Run(cacheParams.name, func(t *ldtest.T) {
			var dataSourceToStartClientWithNoFlags *SDKDataSource
			var sdkConfig servicedef.SDKConfigParams
			if cacheParams.shouldBeCached {
				// For this test, if caching is enabled, we do *not* want the client to receive a "put" event
				// with empty data from the stream at startup time, because then it will cache the empty list
				// of flags. Instead we'll use a data source that hangs, and we'll allow the client to start up
				// in a not-yet-initialized state where it will have to query the data store the first time.
				dataSourceToStartClientWithNoFlags = NewSDKDataSource(t,
					mockld.BlockingUnavailableSDKData(mockld.ServerSideSDK))
				sdkConfig.InitCanFail = true
				sdkConfig.StartWaitTimeMS = ldtime.UnixMillisecondTime(1)
			} else {
				dataSourceToStartClientWithNoFlags = emptyDataSource
			}

			t.Run("called to get all flags", func(t *ldtest.T) {
				dataForGet := mockld.NewServerSDKDataBuilder().Flag(flag1, flag2).Build()
				store := NewPersistentDataStore(t)
				_ = store.SetupInitCapture()
				paramsCh := store.SetupGetAllCapture(dataForGet)

				client := NewSDKClient(t, WithConfig(sdkConfig),
					dataSourceToStartClientWithNoFlags,
					WithPersistentDataStoreConfig(cacheParams.storeParams),
					store)

				result := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: &user})

				params := requireValue(t, paramsCh, time.Second*5)
				assert.Equal(t, flagsKind, params.Kind)

				m.In(t).Assert(result.State, m.MapIncluding(
					m.KV(flag1.Key, m.JSONEqual(expectedValue1)),
					m.KV(flag2.Key, m.JSONEqual(expectedValue2)),
				))
			})

			t.Run("re-evaluating flags that were already retrieved", func(t *ldtest.T) {
				dataForGet := mockld.NewServerSDKDataBuilder().Flag(flag1, flag2).Build()
				store := NewPersistentDataStore(t)
				_ = store.SetupInitCapture()
				paramsCh := store.SetupGetAllCapture(dataForGet)

				client := NewSDKClient(t, WithConfig(sdkConfig),
					dataSourceToStartClientWithNoFlags,
					WithPersistentDataStoreConfig(cacheParams.storeParams),
					store)

				result1 := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: &user})
				result2 := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: &user})

				params1 := requireValue(t, paramsCh, time.Second*5)
				assert.Equal(t, flagsKind, params1.Kind)

				if cacheParams.shouldBeCached {
					requireNoMoreValues(t, paramsCh, time.Millisecond*10)
				} else {
					params2 := requireValue(t, paramsCh, time.Millisecond*10)
					assert.Equal(t, params1, params2)
				}

				assert.Equal(t, result1, result2)
			})

			t.Run("retrieve flags previously stored by init", func(t *ldtest.T) {
				dataWithFlags := mockld.NewServerSDKDataBuilder().Flag(flag1, flag2).Build()
				dataSource := NewSDKDataSource(t, dataWithFlags) // flags are present at init time
				store := NewPersistentDataStore(t)
				_ = store.SetupInitCapture()
				paramsCh := store.SetupGetAllCapture(dataWithFlags)

				client := NewSDKClient(t, dataSource, WithPersistentDataStoreConfig(cacheParams.storeParams), store)

				result1 := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: &user})
				result2 := client.EvaluateAllFlags(t, servicedef.EvaluateAllFlagsParams{User: &user})

				if cacheParams.shouldBeCached {
					requireNoMoreValues(t, paramsCh, time.Millisecond*10)
				} else {
					params1 := requireValue(t, paramsCh, time.Second*5)
					assert.Equal(t, flagsKind, params1.Kind)
					params2 := requireValue(t, paramsCh, time.Millisecond*10)
					assert.Equal(t, params1, params2)
				}

				assert.Equal(t, result1, result2)
			})
		})
	}
}

type serializedDataMatchers struct{} // just for namespacing

func (s serializedDataMatchers) stringAsJSON() m.MatcherTransform {
	return m.Transform("as JSON", func(value interface{}) (interface{}, error) {
		s := value.(string)
		return json.RawMessage(s), nil
	}).EnsureInputValueType("")
}

func (s serializedDataMatchers) itemKey() m.MatcherTransform {
	return m.Transform("serialized item key", func(value interface{}) (interface{}, error) {
		item := value.(cf.DataStoreKeyedItem)
		return item.Key, nil
	}).EnsureInputValueType(cf.DataStoreKeyedItem{})
}

func (s serializedDataMatchers) itemData() m.MatcherTransform {
	return m.Transform("serialized item data", func(value interface{}) (interface{}, error) {
		switch item := value.(type) {
		case cf.DataStoreSerializedItem:
			return json.RawMessage(item.Data), nil
		case cf.DataStoreKeyedItem:
			return json.RawMessage(item.Item.Data), nil
		default:
			return nil,
				fmt.Errorf("expected PersistentDataStoreSerializedItem or PersistentDataStoreKeyedItem but got %T", value)
		}
	})
}

func (s serializedDataMatchers) itemVersion() m.MatcherTransform {
	return m.Transform("serialized item version", func(value interface{}) (interface{}, error) {
		switch item := value.(type) {
		case cf.DataStoreSerializedItem:
			return item.Version, nil
		case cf.DataStoreKeyedItem:
			return item.Item.Version, nil
		default:
			return nil,
				fmt.Errorf("expected PersistentDataStoreSerializedItem or PersistentDataStoreKeyedItem but got %T", value)
		}
	}).EnsureInputValueType(cf.DataStoreKeyedItem{})
}

func (s serializedDataMatchers) collectionKind() m.MatcherTransform {
	return m.Transform("kind", func(value interface{}) (interface{}, error) {
		coll := value.(cf.DataStoreCollection)
		return coll.Kind, nil
	}).EnsureInputValueType(cf.DataStoreCollection{})
}

func (s serializedDataMatchers) collectionItems() m.MatcherTransform {
	return m.Transform("items", func(value interface{}) (interface{}, error) {
		coll := value.(cf.DataStoreCollection)
		return coll.Items, nil
	}).EnsureInputValueType(cf.DataStoreCollection{})
}

func (s serializedDataMatchers) keyedItemIs(item basicSerializedItem) m.Matcher {
	return m.AllOf(
		s.itemKey().Should(m.Equal(item.getKey())),
		s.itemVersion().Should(m.Equal(item.getVersion())),
		s.itemData().Should(item.dataMatcher()),
	)
}
