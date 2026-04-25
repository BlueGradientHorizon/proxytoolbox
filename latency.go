package main

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/printers"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
	"github.com/bluegradienthorizon/proxytoolbox/testrunner"
)

type LatencyTestParams struct {
	Concurrency int
	Timeout     time.Duration
	Rounds      int
}

func runLatencyTest(ctx context.Context, profiles []parsers.ProxyProfile, params LatencyTestParams, testerPath string) ([]testers.LatencyTestResult, []parsers.ProxyProfile, error) {
	runner, err := testrunner.NewTestRunner(testrunner.TestRunnerConfig{
		TesterPath: testerPath,
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
