package main

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/internal/cli/utils"
	"github.com/bluegradienthorizon/proxytoolbox/runner"
	"github.com/bluegradienthorizon/proxytoolbox/worker"
)

// speedTestSettings holds configuration for speed tests
type speedTestSettings struct {
	Concurrency int
	Rounds      int
	Timeout     time.Duration
	Mode        runner.SpeedTestMode
	TestLimit   int
	TargetBytes int64
	Provider    worker.SpeedTestProvider
}

func runSpeedTest(ctx context.Context, tags []string, stSettings speedTestSettings, testRunner *runner.TestRunner) ([]runner.SpeedTestResult, []string, error) {
	// Limit configs based on test limit
	if stSettings.TestLimit > 0 {
		limit := min(stSettings.TestLimit, len(tags))
		tags = append([]string(nil), tags[:limit]...)
	}

	var printerChan chan runner.SpeedTestResult
	var printer *utils.StatsPrinter[runner.SpeedTestResult]
	var printDone chan bool

	config := runner.SpeedTestRunnerSettings{
		BaseTestRunnerSettings: runner.BaseTestRunnerSettings{
			SortResults:  true,
			FilterFailed: true,
			Concurrency:  stSettings.Concurrency,
			Rounds:       stSettings.Rounds,
			Timeout:      stSettings.Timeout,
			RoundStartedCallback: func(round, outboundsLen int) {
				println(fmt.Sprintf("speedtest round %d/%d", round+1, stSettings.Rounds))
				printerChan = make(chan runner.SpeedTestResult, outboundsLen)
				printer = utils.NewStatsPrinter(outboundsLen, printerChan, func(r runner.SpeedTestResult) bool { return r.Error != nil })
				printDone = make(chan bool)
				go printer.Start(printDone)
			},
			ProgressCallback: func(result runner.SpeedTestResult) {
				printerChan <- result
			},
			RoundEndedCallback: func(round int) {
				<-printDone
				close(printerChan)
			},
		},
		TargetBytes: stSettings.TargetBytes,
		Mode:        stSettings.Mode,
		Provider:    stSettings.Provider,
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
