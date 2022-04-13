package sdktests

import (
	"fmt"
	"testing"

	h "github.com/launchdarkly/sdk-test-harness/framework/helpers"
	o "github.com/launchdarkly/sdk-test-harness/framework/opt"

	"github.com/stretchr/testify/assert"
)

func TestComputeExpectedBucketValue(t *testing.T) {
	// This broadly validates our implementation of the bucketing hash value algorithm with a short list
	// of precomputed hard-coded results, just to make sure we haven't broken it. These values are also
	// used in unit tests for some of the SDKs.
	for _, p := range []struct {
		flagOrSegmentKey, salt, userValue string
		seed                              o.Maybe[int]
		expectedValue                     int
	}{
		{"hashKey", "saltyA", "userKeyA", o.None[int](), 42157},
		{"hashKey", "saltyA", "userKeyB", o.None[int](), 67084},
		{"hashKey", "saltyA", "userKeyC", o.None[int](), 10343},
		{"hashKey", "saltyA", "userKeyA", o.Some(61), 9801},
	} {
		t.Run(fmt.Sprintf("%+v", p), func(t *testing.T) {
			computedValue := computeExpectedBucketValue(
				p.userValue,
				p.flagOrSegmentKey,
				p.salt,
				o.None[string](),
				p.seed,
			)
			assert.Equal(t, p.expectedValue, computedValue, "computed value did not match expected value")

			for _, secondary := range []string{"abcdef", ""} {
				valueWithSecondaryKey := computeExpectedBucketValue(
					p.userValue,
					p.flagOrSegmentKey,
					p.salt,
					o.Some(secondary),
					p.seed,
				)
				failureDesc := h.IfElse(secondary == "", "empty secondary key", "empty-but-not-undefined secondary key") +
					" should have changed result, but did not"
				assert.NotEqual(t, p.expectedValue, valueWithSecondaryKey, failureDesc)
			}
		})
	}
}
