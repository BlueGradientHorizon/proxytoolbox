package main

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/internal/cli/utils"
	"github.com/bluegradienthorizon/proxytoolbox/presets"
	"github.com/bluegradienthorizon/proxytoolbox/runner"
)

type latencyTestSettings struct {
	Concurrency int
	Timeout     time.Duration
	Rounds      int
}

func runLatencyTest(ctx context.Context, tags []string, ltSettings latencyTestSettings, testRunner *runner.TestRunner) ([]runner.LatencyTestResult, []string, error) {
	var printerChan chan runner.LatencyTestResult
	var printer *utils.StatsPrinter
	var printDone chan bool

	config := runner.LatencyTestRunnerSettings{
		BaseTestRunnerSettings: runner.BaseTestRunnerSettings{
			SortResults:  true,
			FilterFailed: true,
			Concurrency:  ltSettings.Concurrency,
			Timeout:      ltSettings.Timeout,
			Rounds:       ltSettings.Rounds,
			RoundStartedCallback: func(round int, outboundsLen int) {
				println(fmt.Sprintf("latencytest round %d/%d", round+1, ltSettings.Rounds))
				printerChan = make(chan runner.LatencyTestResult, outboundsLen)
				printer = utils.NewStatsPrinter(outboundsLen, printerChan)
				printDone = make(chan bool)
				go printer.Start(printDone)
			},
			ProgressCallback: func(result runner.LatencyTestResult) {
				printerChan <- result
			},
			RoundEndedCallback: func(round int) {
				<-printDone
				close(printerChan)
			},
		},
		TestURL: presets.Google204,
	}

	testResults, err := testRunner.RunLatencyTests(ctx, tags, config)
	if err != nil {
		return nil, nil, fmt.Errorf("latency tests failed: %w", err)
	}

	validTags := make([]string, 0, len(testResults.Results))
	for _, result := range testResults.Results {
		if result.Error == nil {
			validTags = append(validTags, result.Tag)
		}
	}

	return testResults.Results, validTags, nil
}
