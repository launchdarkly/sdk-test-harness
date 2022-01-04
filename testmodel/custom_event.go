package testmodel

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

type CustomEventTestSuite struct {
	Events []CustomEventTest `json:"events"`
}

type CustomEventTest struct {
	EventKey     string        `json:"eventKey"`
	User         lduser.User   `json:"user"`
	Data         ldvalue.Value `json:"value"`
	OmitNullData bool          `json:"omitNullData"`
	MetricValue  *float64      `json:"metricValue,omitempty"`
}
