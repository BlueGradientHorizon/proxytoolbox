package main

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/printers"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
	"github.com/bluegradienthorizon/proxytoolbox/testrunner"
)

// LatencyTestParams holds configuration for latency tests
type LatencyTestParams struct {
	Concurrency  int
	CoreType     testrunner.CoreType
	LogLevel     string
	Timeout      time.Duration
	Rounds       int
	UseHighLevel bool
}

// runHighLevelLatencyTest demonstrates using the test runner's high-level API
// for simple, automatic lifecycle management. This is the recommended approach
// for most use cases.
func runHighLevelLatencyTest(ctx context.Context, profiles []parsers.ProxyProfile, params LatencyTestParams) ([]testers.LatencyTestResult, []parsers.ProxyProfile, error) {
	runner, err := testrunner.NewTestRunner(testrunner.TestRunnerConfig{
		CoreType:    params.CoreType,
		LogLevel:    params.LogLevel,
		AutoCleanup: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create test runner: %w", err)
	}
	defer runner.Close()

	var printerChan chan testers.LatencyTestResult
	var printer *printers.StatsPrinter
	var printDone chan bool

	config := testrunner.LatencyTestRunnerConfig{
		BaseTestRunnerConfig: testrunner.BaseTestRunnerConfig{
			SortResults:  true,
			FilterFailed: true,
			Concurrency:  params.Concurrency,
			Timeout:      params.Timeout,
			Rounds:       params.Rounds,
			CoreCreatedCallback: func(validationErrors map[string]int) {
				println("validation errors:")
				for err, count := range validationErrors {
					fmt.Println(count, "x", err)
				}
			},
			RoundStartedCallback: func(round int, outboundsLen int) {
				println(fmt.Sprintf("round %d/%d", round+1, params.Rounds))
				printerChan = make(chan testers.LatencyTestResult, outboundsLen)
				printer = printers.NewStatsPrinter(outboundsLen, printerChan)
				printDone = make(chan bool)
				go printer.Start(printDone)
			},
			ProgressCallback: func(result testers.LatencyTestResult) {
				printerChan <- result
			},
			RoundEndedCallback: func(round int) {
				<-printDone
				close(printerChan)
			},
		},
		TestURL: testers.Google204,
	}

	testResults, err := runner.RunLatencyTests(ctx, profiles, config)
	if err != nil {
		return nil, nil, fmt.Errorf("latency tests failed: %w", err)
	}

	// Filter profiles to match successful results
	validProfiles := make([]parsers.ProxyProfile, 0, len(testResults.Results))
	for _, result := range testResults.Results {
		if result.Error == nil {
			for _, p := range profiles {
				if p.Config != nil && p.Config.Tag == result.Tag {
					validProfiles = append(validProfiles, p)
					break
				}
			}
		}
	}

	return testResults.Results, validProfiles, nil
}

// runLowLevelLatencyTest demonstrates using the test runner's low-level API
// for custom workflows with fine-grained control over the core lifecycle
// and test execution. This example shows:
//   - Manual core creation and lifecycle management
//   - Custom filtering logic between test rounds
//   - Direct access to outbounds for custom test logic
//   - Manual resource cleanup
//
// This approach provides maximum flexibility for advanced use cases where
// the high-level RunLatencyTests API is too restrictive.
func runLowLevelLatencyTest(ctx context.Context, profiles []parsers.ProxyProfile, params LatencyTestParams) ([]testers.LatencyTestResult, []parsers.ProxyProfile, error) {
	// Step 1: Create test runner with manual cleanup (AutoCleanup: false)
	runner, err := testrunner.NewTestRunner(testrunner.TestRunnerConfig{
		CoreType:    params.CoreType,
		LogLevel:    params.LogLevel,
		AutoCleanup: false, // We'll manage cleanup manually
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create test runner: %w", err)
	}
	// Ensure cleanup happens even on error or panic
	defer runner.Close()

	// Step 2: Set unique tags BEFORE creating core
	for i := range profiles {
		profiles[i].Config.Tag = fmt.Sprintf("outbound-%d", i)
	}

	printValidationErrors := func(m map[string]int) {
		println("validation errors:")
		for err, count := range m {
			fmt.Println(count, "x", err)
		}
	}

	// Step 3: Create core instance (without starting it yet)
	validationErrorsMap, err := runner.CreateCore(ctx, profiles)
	if err != nil {
		printValidationErrors(validationErrorsMap)
		return nil, nil, fmt.Errorf("failed to create core: %w", err)
	}

	// Print validation errors if any
	printValidationErrors(validationErrorsMap)

	if len(profiles) == 0 {
		return nil, nil, fmt.Errorf("no valid configurations after validation")
	}

	// Step 4: Start the core instance
	err = runner.StartCore()
	if err != nil {
		return nil, nil, fmt.Errorf("start core failed: %w", err)
	}

	fmt.Println("Core started successfully.")

	// Step 5: Get outbounds for testing
	var results []testers.LatencyTestResult
	latencyTestCtx, latencyTestCtxCancel := context.WithCancel(ctx)
	defer latencyTestCtxCancel()

	// Step 6: Run multiple test rounds with custom filtering logic
	for i := range params.Rounds {
		// Check for context cancellation
		if latencyTestCtx.Err() != nil {
			println("test ended prematurely: " + latencyTestCtx.Err().Error())
			break
		}

		var wrappedOutbounds []core.Outbound
		if i == 0 {
			// Get outbounds from test runner for first round
			wrappedOutbounds = runner.GetOutbounds()
		} else {
			// Rebuild outbounds from previous results by tag
			for _, r := range results {
				outbound, err := runner.GetCoreInstance().GetOutboundByTag(r.Tag)
				if err == nil {
					wrappedOutbounds = append(wrappedOutbounds, outbound)
				}
			}
		}

		if len(wrappedOutbounds) == 0 {
			println("no working profiles left")
			break
		}

		println(fmt.Sprintf("round %d/%d", i+1, params.Rounds))

		printerChan := make(chan testers.LatencyTestResult, len(wrappedOutbounds))
		printer := printers.NewStatsPrinter(len(wrappedOutbounds), printerChan)
		printDone := make(chan bool)
		go printer.Start(printDone)

		// Create latency test settings
		sett := testers.LatencyTestSettings{
			TestURL: testers.Google204,
			Timeout: params.Timeout,
		}

		// Run a single round of latency tests with progress callback
		roundResults, err := runner.RunLatencyTestRound(latencyTestCtx, wrappedOutbounds, sett, 0, func(result testers.LatencyTestResult) {
			printerChan <- result
		})
		if err != nil {
			println(err.Error())
			close(printerChan)
			continue
		}

		results = nil
		for _, r := range roundResults {
			if r.Error == nil {
				results = append(results, r)
			}
		}

		<-printDone
		close(printerChan)
	}

	// Step 7: Process final results with custom logic
	sortedResults := make([]testers.LatencyTestResult, 0, len(results))
	for _, r := range results {
		if r.Error == nil {
			sortedResults = append(sortedResults, r)
		}
	}

	slices.SortFunc(sortedResults, func(a, b testers.LatencyTestResult) int {
		if a.Delay < b.Delay {
			return -1
		}
		if a.Delay > b.Delay {
			return 1
		}
		return 0
	})

	// Step 8: Manual cleanup (StopCore)
	// Note: This is optional since defer runner.Close() will handle it,
	// but shown here for demonstration of manual control
	runner.StopCore()

	return sortedResults, profiles, nil
}
