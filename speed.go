package main

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
	"github.com/bluegradienthorizon/proxytoolbox/testrunner"
)

// SpeedTestParams holds configuration for speed tests
type SpeedTestParams struct {
	Concurrency  int
	Rounds       int
	CoreType     testrunner.CoreType
	LogLevel     string
	Timeout      time.Duration
	Mode         testers.SpeedTestMode
	TestLimit    int
	TargetBytes  int64
	UseHighLevel bool
}

// runHighLevelSpeedTest demonstrates using the test runner for speed testing
// with the high-level API. This can be used after latency testing to measure
// bandwidth performance of successful proxies.
func runHighLevelSpeedTest(ctx context.Context, profiles []parsers.ProxyProfile, params SpeedTestParams) ([]testers.SpeedTestResult, []parsers.ProxyProfile, error) {
	// Limit profiles based on test limit
	if len(profiles) > params.TestLimit {
		profiles = profiles[:params.TestLimit]
	}

	runner, err := testrunner.NewTestRunner(testrunner.TestRunnerConfig{
		CoreType:    params.CoreType,
		LogLevel:    params.LogLevel,
		AutoCleanup: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create test runner: %w", err)
	}

	config := testrunner.SpeedTestRunnerConfig{
		BaseTestRunnerConfig: testrunner.BaseTestRunnerConfig{
			SortResults:  true,
			FilterFailed: true,
			Concurrency:  params.Concurrency,
			Rounds:       params.Rounds,
			Timeout:      params.Timeout,
			RoundStartedCallback: func(round, outboundsLen int) {
				println(fmt.Sprintf("round %d/%d", round+1, params.Rounds))
			},
			ProgressCallback: func(result testers.SpeedTestResult) {
				var t string
				if params.Mode == testers.Download {
					t = "download"
				} else {
					t = "upload"
				}
				if result.Error == nil {
					fmt.Printf("%s: %.2f MB/s\n", t, result.Speed/1024/1024)
				} else {
					fmt.Printf("%s: %s\n", t, result.Error.Error())
				}
			},
			RoundEndedCallback: func(round int) {

			},
		},
		TargetBytes: params.TargetBytes,
		Mode:        params.Mode,
		Provider:    testers.CloudflareProvider,
	}

	results, err := runner.RunSpeedTests(ctx, profiles, config)
	if err != nil {
		runner.Close()
		return nil, nil, fmt.Errorf("speed test failed: %w", err)
	}

	runner.Close()

	return results.Results, profiles, nil
}

// runLowLevelSpeedTest demonstrates using the test runner's low-level API
// for custom workflows with fine-grained control over the core lifecycle
// and test execution. This example shows:
//   - Manual core creation and lifecycle management
//   - Custom filtering logic between test rounds
//   - Direct access to outbounds for custom test logic
//   - Manual resource cleanup
//
// This approach provides maximum flexibility for advanced use cases where
// the high-level RunSpeedTests API is too restrictive.
func runLowLevelSpeedTest(ctx context.Context, profiles []parsers.ProxyProfile, params SpeedTestParams) ([]testers.SpeedTestResult, []parsers.ProxyProfile, error) {
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
	var results []testers.SpeedTestResult
	speedTestCtx, speedTestCtxCancel := context.WithCancel(ctx)
	defer speedTestCtxCancel()

	// Limit profiles based on test limit
	var wrappedOutbounds []core.Outbound
	allOutbounds := runner.GetOutbounds()
	if len(allOutbounds) > params.TestLimit {
		wrappedOutbounds = allOutbounds[:params.TestLimit]
	} else {
		wrappedOutbounds = allOutbounds
	}

	if len(wrappedOutbounds) == 0 {
		return nil, nil, fmt.Errorf("no valid outbounds")
	}

	// Create speed test settings
	var sett testers.SpeedTestSettings
	if params.Mode == testers.Download {
		sett = testers.NewDownloadTestSettings()
	} else {
		sett = testers.NewUploadTestSettings()
	}
	sett.TargetBytes = params.TargetBytes
	sett.Timeout = params.Timeout
	sett.Provider = testers.CloudflareProvider

	// Step 6: Run multiple test rounds with custom filtering logic
	for i := range params.Rounds {
		// Check for context cancellation
		if speedTestCtx.Err() != nil {
			println("test ended prematurely: " + speedTestCtx.Err().Error())
			break
		}

		if i > 0 {
			// Rebuild outbounds from previous results by tag
			var nextOutbounds []core.Outbound
			for _, r := range results {
				outbound, err := runner.GetCoreInstance().GetOutboundByTag(r.Tag)
				if err == nil {
					nextOutbounds = append(nextOutbounds, outbound)
				}
			}
			wrappedOutbounds = nextOutbounds
		}

		if len(wrappedOutbounds) == 0 {
			println("no working profiles left")
			break
		}

		println(fmt.Sprintf("round %d/%d", i+1, params.Rounds))

		// Run a single round of speed tests with progress callback
		roundResults, err := runner.RunSpeedTestRound(speedTestCtx, wrappedOutbounds, sett, 0, func(result testers.SpeedTestResult) {
			var t string
			if params.Mode == testers.Download {
				t = "download"
			} else {
				t = "upload"
			}
			if result.Error == nil {
				fmt.Printf("%s: %.2f MB/s\n", t, result.Speed/1024/1024)
			} else {
				fmt.Printf("%s: %s\n", t, result.Error.Error())
			}
		})
		if err != nil {
			println(err.Error())
			continue
		}

		// Step 7: Filter successful results for next round
		results = nil
		for _, r := range roundResults {
			if r.Error == nil {
				results = append(results, r)
			}
		}
	}

	// Step 8: Manual cleanup (StopCore)
	// Note: This is optional since defer runner.Close() will handle it,
	// but shown here for demonstration of manual control
	runner.StopCore()

	return results, profiles, nil
}
