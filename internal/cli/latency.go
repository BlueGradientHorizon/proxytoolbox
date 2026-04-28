package main

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/internal/cli/utils"
	"github.com/bluegradienthorizon/proxytoolbox/measure"
	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/runner"
	"github.com/bluegradienthorizon/proxytoolbox/worker"
)

type LatencyTestSettings struct {
	Concurrency int
	Timeout     time.Duration
	Rounds      int
}

func runLatencyTest(ctx context.Context, configs []parsers.ProxyConfig, ltSettings LatencyTestSettings, testRunner *runner.TestRunner) ([]measure.LatencyTestResult, []parsers.ProxyConfig, error) {
	var printerChan chan measure.LatencyTestResult
	var printer *utils.StatsPrinter
	var printDone chan bool

	config := runner.LatencyTestRunnerSettings{
		BaseTestRunnerSettings: runner.BaseTestRunnerSettings{
			SortResults:  true,
			FilterFailed: true,
			Concurrency:  ltSettings.Concurrency,
			Timeout:      ltSettings.Timeout,
			Rounds:       ltSettings.Rounds,
			CoreCreatedCallback: func(validationErrors []worker.ValidationError) {
				println("validation errors:")
				for _, errPair := range validationErrors {
					fmt.Printf("[%s] %s\n", errPair.Tag, errPair.Error)
				}
			},
			RoundStartedCallback: func(round int, outboundsLen int) {
				println(fmt.Sprintf("round %d/%d", round+1, ltSettings.Rounds))
				printerChan = make(chan measure.LatencyTestResult, outboundsLen)
				printer = utils.NewStatsPrinter(outboundsLen, printerChan)
				printDone = make(chan bool)
				go printer.Start(printDone)
			},
			ProgressCallback: func(result measure.LatencyTestResult) {
				printerChan <- result
			},
			RoundEndedCallback: func(round int) {
				<-printDone
				close(printerChan)
			},
		},
		TestURL: measure.Google204,
	}

	testResults, err := testRunner.RunLatencyTests(ctx, configs, config)
	if err != nil {
		return nil, nil, fmt.Errorf("latency tests failed: %w", err)
	}

	// Filter configs to match successful results
	configMap := make(map[string]parsers.ProxyConfig, len(configs))
	for _, p := range configs {
		if p.Config != nil {
			configMap[p.Config.Tag] = p
		}
	}
	validConfigs := make([]parsers.ProxyConfig, 0, len(testResults.Results))
	for _, result := range testResults.Results {
		if result.Error == nil {
			if p, ok := configMap[result.Tag]; ok {
				validConfigs = append(validConfigs, p)
			}
		}
	}

	return testResults.Results, validConfigs, nil
}
