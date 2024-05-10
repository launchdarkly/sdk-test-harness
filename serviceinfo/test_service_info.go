// Package serviceinfo provides a data model for information provided by a service under test.
package serviceinfo

import "github.com/launchdarkly/sdk-test-harness/v2/framework"

// TestServiceInfo is status information returned by the test service from the initial status query.
type TestServiceInfo struct {
	TestServiceInfoBase

	// FullData is the entire response received from the test service, which might contain additional
	// properties beyond TestServiceInfoBase.
	FullData []byte
}

// TestServiceInfoBase is the basic set of properties that all test services must provide.
type TestServiceInfoBase struct {
	// Name is the name of the project that the test service is testing, such as "go-server-sdk".
	Name string `json:"name"`

	// Capabilities is a list of strings representing optional features of the test service.
	Capabilities framework.Capabilities `json:"capabilities"`
}

func Empty() TestServiceInfo {
	return TestServiceInfo{}
}
