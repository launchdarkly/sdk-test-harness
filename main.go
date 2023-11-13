package main

import (
	"bufio"
	_ "embed" // this is required in order for go:embed to work
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/launchdarkly/sdk-test-harness/v3/framework"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/harness"
	"github.com/launchdarkly/sdk-test-harness/v3/framework/ldtest"
	"github.com/launchdarkly/sdk-test-harness/v3/sdktests"
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

	results, err := run(params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !results.OK() {
		os.Exit(1)
	}
}

func run(params commandParams) (*ldtest.Results, error) {
	if params.skipFile != "" {
		if err := loadSuppressions(&params); err != nil {
			return nil, err
		}
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
		return nil, err
	}

	var testLogger ldtest.TestLogger
	consoleLogger := ldtest.ConsoleTestLogger{
		DebugOutputOnFailure: params.debug || params.debugAll,
		DebugOutputOnSuccess: params.debugAll,
	}
	if params.jUnitFile == "" {
		testLogger = consoleLogger
	} else {
		testLogger = &ldtest.MultiTestLogger{Loggers: []ldtest.TestLogger{
			consoleLogger,
			ldtest.NewJUnitTestLogger(params.jUnitFile, harness.TestServiceInfo(), params.filters),
		}}
	}

	results := sdktests.RunSDKTestSuite(harness, params.filters, testLogger)

	fmt.Println()
	logErr := testLogger.EndLog(results)

	if params.stopServiceAtEnd {
		fmt.Println("Stopping test service")
		if err := harness.StopService(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to stop test service: %s\n", err)
		}
	}

	if logErr != nil {
		return nil, fmt.Errorf("error writing log: %v", logErr)
	}

	if params.recordFailures != "" {
		f, err := os.Create(params.recordFailures)
		if err != nil {
			return nil, fmt.Errorf("cannot create suppression file: %v", err)
		}
		for _, test := range results.Failures {
			fmt.Fprintln(f, test.TestID)
		}
		_ = f.Close()
	}

	return &results, nil
}

func loadSuppressions(params *commandParams) error {
	file, err := os.Open(params.skipFile)
	if err != nil {
		return fmt.Errorf("cannot open provided suppression file: %v", err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Ignore blank lines
		if strings.TrimSpace(line) == "" {
			continue
		}
		escaped := regexp.QuoteMeta(line)
		if err := params.filters.MustNotMatch.Set(escaped); err != nil {
			return fmt.Errorf("cannot parse suppression: %v", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("while processing suppression file: %v", err)
	}
	return nil
}
