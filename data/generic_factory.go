package data

import (
	"github.com/launchdarkly/sdk-test-harness/v2/mockld"
	"github.com/launchdarkly/sdk-test-harness/v2/servicedef"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
)

type GenericFactory[ParamT, ResultT any] interface {
	Get(param ParamT) ResultT
}

type MemoizingFactory[ParamT comparable, ResultT any] struct {
	factoryFn          func(ParamT) ResultT
	transformVersionFn func(ResultT, int) ResultT
	cache              map[ParamT]ResultT
	nextVersion        int
}

func NewMemoizingFlagFactory(
	startingVersion int,
	factoryFn func(servicedef.ValueType) ldmodel.FeatureFlag,
) *MemoizingFactory[servicedef.ValueType, ldmodel.FeatureFlag] {
	f := &MemoizingFactory[servicedef.ValueType, ldmodel.FeatureFlag]{
		factoryFn: factoryFn,
		transformVersionFn: func(f ldmodel.FeatureFlag, v int) ldmodel.FeatureFlag {
			f.Version = v
			return f
		},
		nextVersion: startingVersion,
	}
	return f
}

func NewMemoizingClientSideFlagFactory(
	startingVersion int,
	factoryFn func(servicedef.ValueType) mockld.ClientSDKFlagWithKey,
) *MemoizingFactory[servicedef.ValueType, mockld.ClientSDKFlagWithKey] {
	f := &MemoizingFactory[servicedef.ValueType, mockld.ClientSDKFlagWithKey]{
		factoryFn: factoryFn,
		transformVersionFn: func(f mockld.ClientSDKFlagWithKey, v int) mockld.ClientSDKFlagWithKey {
			f.Version = v
			return f
		},
		nextVersion: startingVersion,
	}
	return f
}

func (f *MemoizingFactory[P, R]) Create(param P) R {
	item := f.factoryFn(param)
	version := f.nextVersion
	if version == 0 {
		version++
	}
	f.nextVersion++
	item = f.transformVersionFn(item, version)
	f.nextVersion = version
	if f.cache == nil {
		f.cache = make(map[P]R)
	}
	f.cache[param] = item
	return item
}

func (f *MemoizingFactory[P, R]) GetOrCreate(param P) R {
	if item, ok := f.cache[param]; ok {
		return item
	}
	return f.Create(param)
}
