package sdktests

import (
	"fmt"
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

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
				ldvalue.String(p.userValue),
				p.flagOrSegmentKey,
				p.salt,
				ldvalue.OptionalString{},
				p.seed,
			)
			assert.Equal(t, p.expectedValue, computedValue, "computed value did not match expected value")

			for _, secondary := range []string{"abcdef", ""} {
				valueWithSecondaryKey := computeExpectedBucketValue(
					ldvalue.String(p.userValue),
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
