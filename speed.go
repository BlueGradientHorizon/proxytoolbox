package main

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
	"github.com/bluegradienthorizon/proxytoolbox/testrunner"
)

// SpeedTestParams holds configuration for speed tests
type SpeedTestParams struct {
	Concurrency int
	Rounds      int
	Timeout     time.Duration
	Mode        testers.SpeedTestMode
	TestLimit   int
	TargetBytes int64
}

func runSpeedTest(ctx context.Context, profiles []parsers.ProxyProfile, params SpeedTestParams, testerPath string) ([]testers.SpeedTestResult, []parsers.ProxyProfile, error) {
	// Limit profiles based on test limit
	if len(profiles) > params.TestLimit {
		profiles = profiles[:params.TestLimit]
	}

	runner, err := testrunner.NewTestRunner(testrunner.TestRunnerConfig{
		TesterPath:  testerPath,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create test runner: %w", err)
	}
	defer runner.Close()

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
		return nil, nil, fmt.Errorf("speed test failed: %w", err)
	}

	return results.Results, profiles, nil
}
