package main

import (
	_ "embed" // this is required in order for go:embed to work
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/framework"
	"github.com/launchdarkly/sdk-test-harness/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/sdktests"
	"github.com/launchdarkly/sdk-test-harness/servicedef"
)

const defaultPort = 8111
const statusQueryTimeout = time.Second * 10

//go:embed VERSION
var versionString string // comes from the VERSION file which we update for each release

func main() {
	fmt.Printf("sdk-test-harness v%s\n", strings.TrimSpace(versionString))

	var params commandParams
	if !params.Read(os.Args) {
		os.Exit(1)
	}

	mainDebugLogger := framework.NullLogger()
	if params.debugAll {
		mainDebugLogger = log.New(os.Stdout, "", log.LstdFlags)
	}

	harness, err := harness.NewTestHarness(
		params.serviceURL,
		params.host,
		params.port,
		statusQueryTimeout,
		mainDebugLogger,
		os.Stdout,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Test service error: %s\n", err)
		os.Exit(1)
	}

	testLogger := ldtest.ConsoleTestLogger{
		DebugOutputOnFailure: params.debug || params.debugAll,
		DebugOutputOnSuccess: params.debugAll,
	}

	var results ldtest.Results
	capabilities := harness.TestServiceInfo().Capabilities
	switch {
	case capabilities.Has(servicedef.CapabilityServerSide):
		fmt.Println("Running server-side SDK test suite")
		results = sdktests.RunServerSideTestSuite(harness, params.filters.Match, testLogger)
	case capabilities.Has(servicedef.CapabilityClientSide):
		fmt.Fprintln(os.Stderr, "Client-side SDK tests are not yet implemented")
		os.Exit(1)
	default:
		fmt.Fprintln(os.Stderr, `Test service has neither "client-side" nor "server-side" capability`)
		os.Exit(1)
	}
	fmt.Println()
	ldtest.PrintFilterDescription(params.filters, []string{}, harness.TestServiceInfo().Capabilities)

	fmt.Println()
	ldtest.PrintResults(results)

	if params.stopServiceAtEnd {
		fmt.Println("Stopping test service")
		if err := harness.StopService(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stop test service: %s\n", err)
		}
	}
	if !results.OK() {
		os.Exit(1)
	}
}
