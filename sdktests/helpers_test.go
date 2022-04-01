package sdktests

import (
	"fmt"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	m "github.com/launchdarkly/go-test-helpers/v2/matchers"

	"github.com/stretchr/testify/assert"
)

func TestComputeExpectedBucketValue(t *testing.T) {
	// This broadly validates our implementation of the bucketing hash value algorithm with a short list
	// of precomputed hard-coded results, just to make sure we haven't broken it. These values are also
	// used in unit tests for some of the SDKs.
	for _, p := range []struct {
		flagOrSegmentKey, salt, userValue string
		seed                              ldvalue.OptionalInt
		expectedValue                     int
	}{
		{"hashKey", "saltyA", "userKeyA", ldvalue.OptionalInt{}, 42157},
		{"hashKey", "saltyA", "userKeyB", ldvalue.OptionalInt{}, 67084},
		{"hashKey", "saltyA", "userKeyC", ldvalue.OptionalInt{}, 10343},
		{"hashKey", "saltyA", "userKeyA", ldvalue.NewOptionalInt(61), 9801},
	} {
		t.Run(fmt.Sprintf("%+v", p), func(t *testing.T) {
			computedValue := computeExpectedBucketValue(
				p.userValue,
				p.flagOrSegmentKey,
				p.salt,
				ldvalue.OptionalString{},
				p.seed,
			)
			assert.Equal(t, p.expectedValue, computedValue, "computed value did not match expected value")

			for _, secondary := range []string{"abcdef", ""} {
				valueWithSecondaryKey := computeExpectedBucketValue(
					p.userValue,
					p.flagOrSegmentKey,
					p.salt,
					ldvalue.NewOptionalString(secondary),
					p.seed,
				)
				failureDesc := selectString(secondary == "", "empty secondary key", "empty-but-not-undefined secondary key") +
					" should have changed result, but did not"
				assert.NotEqual(t, p.expectedValue, valueWithSecondaryKey, failureDesc)
			}
		})
	}
}

func TestSetContextValueForAttrRef(t *testing.T) {
	key := "arbitrary-key"
	value := ldvalue.String("value")

	for _, p := range []struct {
		name          string
		attrRefString string
		expectContext ldcontext.Context
	}{
		{
			"simple", "attr1", ldcontext.NewBuilder(key).SetValue("attr1", value).Build(),
		},
		{
			"object property", "/attr1/subprop",
			ldcontext.NewBuilder(key).SetValue("attr1", ldvalue.ObjectBuild().Set("subprop", value).Build()).Build(),
		},
		{
			"nested property", "/attr1/subprop/subsub",
			ldcontext.NewBuilder(key).SetValue("attr1", ldvalue.ObjectBuild().Set("subprop",
				ldvalue.ObjectBuild().Set("subsub", value).Build()).Build()).Build(),
		},
		{
			"array index", "/attr1/1",
			ldcontext.NewBuilder(key).SetValue("attr1", ldvalue.ArrayOf(ldvalue.Null(), value)).Build(),
		},
	} {
		t.Run(p.name, func(t *testing.T) {
			b := ldcontext.NewBuilder(key)
			setContextValueForAttrRef(b, ldattr.NewRef(p.attrRefString), value)
			m.In(t).Assert(b.Build(), m.JSONEqual(p.expectContext))
		})
	}

	t.Run("object property", func(t *testing.T) {
		b := ldcontext.NewBuilder("key")
		setContextValueForAttrRef(b, ldattr.NewRef("/attr1/subprop"), value)
		assert.Equal(t, ldcontext.NewBuilder("key").SetValue("attr1",
			ldvalue.ObjectBuild().Set("subprop", value).Build()).Build(), b.Build())
	})
}
