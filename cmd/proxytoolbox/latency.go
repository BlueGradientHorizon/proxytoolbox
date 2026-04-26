package main

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/cmd/proxytoolbox/utils"
	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/pkg/ipcprotocol"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
	"github.com/bluegradienthorizon/proxytoolbox/testrunner"
)

type LatencyTestSettings struct {
	Concurrency int
	Timeout     time.Duration
	Rounds      int
}

func runLatencyTest(ctx context.Context, configs []parsers.ProxyConfig, ltSettings LatencyTestSettings, testerSettings testrunner.TesterSettings) ([]testers.LatencyTestResult, []parsers.ProxyConfig, error) {
	runner, err := testrunner.NewTestRunner(testerSettings)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create test runner: %w", err)
	}
	defer runner.Close()

	var printerChan chan testers.LatencyTestResult
	var printer *utils.StatsPrinter
	var printDone chan bool

	config := testrunner.LatencyTestRunnerSettings{
		BaseTestRunnerSettings: testrunner.BaseTestRunnerSettings{
			SortResults:  true,
			FilterFailed: true,
			Concurrency:  ltSettings.Concurrency,
			Timeout:      ltSettings.Timeout,
			Rounds:       ltSettings.Rounds,
			CoreCreatedCallback: func(validationErrors []ipcprotocol.ValidationError) {
				println("validation errors:")
				for _, errPair := range validationErrors {
					fmt.Printf("[%s] %s\n", errPair.Tag, errPair.Error)
				}
			},
			RoundStartedCallback: func(round int, outboundsLen int) {
				println(fmt.Sprintf("round %d/%d", round+1, ltSettings.Rounds))
				printerChan = make(chan testers.LatencyTestResult, outboundsLen)
				printer = utils.NewStatsPrinter(outboundsLen, printerChan)
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

	testResults, err := runner.RunLatencyTests(ctx, configs, config)
	if err != nil {
		return nil, nil, fmt.Errorf("latency tests failed: %w", err)
	}

	// Filter configs to match successful results
	validConfigs := make([]parsers.ProxyConfig, 0, len(testResults.Results))
	for _, result := range testResults.Results {
		if result.Error == nil {
			for _, p := range configs {
				if p.Config != nil && p.Config.Tag == result.Tag {
					validConfigs = append(validConfigs, p)
					break
				}
			}
		}
	}

	return testResults.Results, validConfigs, nil
}
