package sdktests

import (
	"fmt"

	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/mockld"
	"github.com/launchdarkly/sdk-test-harness/v3/servicedef"
)

const defaultSDKKey = "test-sdk-key"

type SDKTestContext struct {
	harness *harness.TestHarness
	sdkKind mockld.SDKKind

	// protocol is the protocol that the mock data services use in this branch of the test tree. It is
	// empty above the "fdv1" and "fdv2" branches.
	protocol mockld.DataProtocol
}

func requireContext(t *ldtest.T) SDKTestContext {
	if c, ok := t.Context().(SDKTestContext); ok {
		return c
	}
	panic("SDKTestContext was not included in the global test configuration!" +
		" This is a basic mistake in the initialization logic.")
}

// dataProtocolCapabilities are the protocols that an SDK can use to get flag data, in the order that their
// branches of the test tree run, with the capability that enables each one.
var dataProtocolCapabilities = []struct { //nolint:gochecknoglobals
	protocol   mockld.DataProtocol
	capability string
}{
	{mockld.FDv1, servicedef.CapabilityFDv1},
	{mockld.FDv2, servicedef.CapabilityFDv2},
}

// runForEachDataProtocol runs the tests once for each data protocol, in a branch of the test tree that
// has the name of the protocol. The mock data services in a branch use its protocol. A branch is skipped
// if the test service does not declare the capability for its protocol.
func runForEachDataProtocol(t *ldtest.T, action func(*ldtest.T)) {
	for _, p := range dataProtocolCapabilities {
		context := requireContext(t)
		context.protocol = p.protocol
		capability := p.capability
		t.RunWithContext(string(p.protocol), context, func(t *ldtest.T) {
			t.RequireCapability(capability)
			action(t)
		})
	}
}

// dataProtocol returns the protocol that the mock data services use in the current test.
func dataProtocol(t *ldtest.T) mockld.DataProtocol {
	protocol := requireContext(t).protocol
	if protocol == "" {
		panic("test logic error: a test that uses the mock data services must run in the fdv1 or fdv2 branch")
	}
	return protocol
}

// requireDataProtocol skips the test unless it runs in the branch of the test tree for the specified
// protocol. Use it for a test of behavior that exists only in that protocol.
func requireDataProtocol(t *ldtest.T, protocol mockld.DataProtocol) {
	if dataProtocol(t) != protocol {
		t.SkipWithReason(fmt.Sprintf("this test applies only to the %s branch", protocol))
	}
}

// requireFDv2 skips the test unless it runs in the fdv2 branch of the test tree.
func requireFDv2(t *ldtest.T) {
	requireDataProtocol(t, mockld.FDv2)
}
