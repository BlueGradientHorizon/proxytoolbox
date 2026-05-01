package main

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/presets"
	"github.com/bluegradienthorizon/proxytoolbox/runner"
)

// SpeedTestSettings holds configuration for speed tests
type SpeedTestSettings struct {
	Concurrency int
	Rounds      int
	Timeout     time.Duration
	Mode        runner.SpeedTestMode
	TestLimit   int
	TargetBytes int64
}

func runSpeedTest(ctx context.Context, tags []string, stSettings SpeedTestSettings, testRunner *runner.TestRunner) ([]runner.SpeedTestResult, []string, error) {
	// Limit configs based on test limit
	if stSettings.TestLimit > 0 {
		limit := min(stSettings.TestLimit, len(tags))
		tags = tags[:limit]
	}

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
			ProgressCallback: func(result runner.SpeedTestResult) {
				var t string
				if stSettings.Mode == runner.SpeedTestModeDownload {
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
		Provider:    presets.CloudflareProvider,
	}

	results, err := testRunner.RunSpeedTests(ctx, tags, config)
	if err != nil {
		return nil, nil, fmt.Errorf("speed test failed: %w", err)
	}

	validTags := make([]string, 0, len(results.Results))
	for _, result := range results.Results {
		if result.Error == nil {
			validTags = append(validTags, result.Tag)
		}
	}

	return results.Results, validTags, nil
}
