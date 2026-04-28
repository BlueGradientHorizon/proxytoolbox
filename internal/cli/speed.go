package main

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/runner"
	"github.com/bluegradienthorizon/proxytoolbox/worker"
)

// SpeedTestSettings holds configuration for speed tests
type SpeedTestSettings struct {
	Concurrency int
	Rounds      int
	Timeout     time.Duration
	Mode        worker.SpeedTestMode
	TestLimit   int
	TargetBytes int64
}

func runSpeedTest(ctx context.Context, configs []parsers.ProxyConfig, stSettings SpeedTestSettings, testRunner *runner.TestRunner) ([]worker.SpeedTestResult, []parsers.ProxyConfig, error) {
	// Limit configs based on test limit
	limit := min(stSettings.TestLimit, len(configs))
	configs = configs[:limit]

	config := runner.SpeedTestRunnerSettings{
		BaseTestRunnerSettings: runner.BaseTestRunnerSettings{
			SortResults:  true,
			FilterFailed: true,
			Concurrency:  stSettings.Concurrency,
			Rounds:       stSettings.Rounds,
			Timeout:      stSettings.Timeout,
			RoundStartedCallback: func(round, outboundsLen int) {
				println(fmt.Sprintf("round %d/%d", round+1, stSettings.Rounds))
			},
			ProgressCallback: func(result worker.SpeedTestResult) {
				var t string
				if stSettings.Mode == worker.Download {
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
		Provider:    worker.CloudflareProvider,
	}

	results, err := testRunner.RunSpeedTests(ctx, configs, config)
	if err != nil {
		return nil, nil, fmt.Errorf("speed test failed: %w", err)
	}

	return results.Results, configs, nil
}
