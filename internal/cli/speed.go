package main

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
	"github.com/bluegradienthorizon/proxytoolbox/testrunner"
)

// SpeedTestSettings holds configuration for speed tests
type SpeedTestSettings struct {
	Concurrency int
	Rounds      int
	Timeout     time.Duration
	Mode        testers.SpeedTestMode
	TestLimit   int
	TargetBytes int64
}

func runSpeedTest(ctx context.Context, configs []parsers.ProxyConfig, stSettings SpeedTestSettings, runner *testrunner.TestRunner) ([]testers.SpeedTestResult, []parsers.ProxyConfig, error) {
	// Limit configs based on test limit
	if len(configs) > stSettings.TestLimit {
		configs = configs[:stSettings.TestLimit]
	}

	config := testrunner.SpeedTestRunnerSettings{
		BaseTestRunnerSettings: testrunner.BaseTestRunnerSettings{
			SortResults:  true,
			FilterFailed: true,
			Concurrency:  stSettings.Concurrency,
			Rounds:       stSettings.Rounds,
			Timeout:      stSettings.Timeout,
			RoundStartedCallback: func(round, outboundsLen int) {
				println(fmt.Sprintf("round %d/%d", round+1, stSettings.Rounds))
			},
			ProgressCallback: func(result testers.SpeedTestResult) {
				var t string
				if stSettings.Mode == testers.Download {
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
		TargetBytes: stSettings.TargetBytes,
		Mode:        stSettings.Mode,
		Provider:    testers.CloudflareProvider,
	}

	results, err := runner.RunSpeedTests(ctx, configs, config)
	if err != nil {
		return nil, nil, fmt.Errorf("speed test failed: %w", err)
	}

	return results.Results, configs, nil
}
