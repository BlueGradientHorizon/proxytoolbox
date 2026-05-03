package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/runner"
)

const (
	inputFile     = "link_list.txt"
	outputFile    = "configs.txt"
	parseErrFile  = "parseErr.txt"
	validErrFile  = "validErr.txt"
	ltResultsFile = "lt-out.txt"
	stResultsFile = "st-out.txt"
	workerLogFile = "worker.log"
)

func main() {
	workerDebug := parseFlags()

	ltSettings := latencyTestSettings{
		Concurrency: 0,
		Timeout:     7 * time.Second,
		Rounds:      3,
	}
	runSpeedTestFlag := true
	stSettings := speedTestSettings{
		Concurrency: 0,
		Rounds:      1,
		Timeout:     10 * time.Second,
		Mode:        runner.SpeedTestModeDownload,
		TestLimit:   0,
		TargetBytes: 1024,
	}

	workerPath := discoverWorker()
	if workerPath == "" {
		fmt.Println("No worker programs found.")
		return
	}

	if err := handleConfigDownload(outputFile, inputFile); err != nil {
		fmt.Printf("Error downloading configs: %v\n", err)
		os.Exit(1)
	}

	configs := loadAndParseConfigs(outputFile, parseErrFile)
	if len(configs) == 0 {
		fmt.Println("! No valid configurations were loaded. Check your source or subscription content.")
		return
	}

	ctx := context.Background()

	for i := range configs {
		if configs[i].Config != nil && configs[i].Config.Tag == "" {
			configs[i].Config.Tag = fmt.Sprintf("outbound-%d", i)
		}
	}

	fmt.Println("Validating all configs...")
	preValidateRunner, err := createTestRunner(workerPath, workerDebug, workerLogFile)
	if err != nil {
		fmt.Printf("Failed to create test runner for validation: %v\n", err)
		os.Exit(1)
	}

	validConfigs, _, err := validateConfigs(ctx, preValidateRunner, configs, validErrFile)
	preValidateRunner.Close()
	if err != nil {
		fmt.Printf("Validation error: %v\n", err)
		os.Exit(1)
	}

	if len(validConfigs) == 0 {
		fmt.Println("No valid configurations after validation.")
		return
	}

	fmt.Printf("Valid configs: %d\n", len(validConfigs))

	const batchSize = 5000

	var allLatencyResults []runner.LatencyTestResult

	for batchStart := 0; batchStart < len(validConfigs); batchStart += batchSize {
		batchEnd := min(batchStart+batchSize, len(validConfigs))
		batchConfigs := validConfigs[batchStart:batchEnd]

		fmt.Printf("Processing batch %d-%d (%d configs)\n", batchStart, batchEnd, len(batchConfigs))

		testRunner, err := createTestRunner(workerPath, workerDebug, workerLogFile)
		if err != nil {
			fmt.Printf("Failed to create test runner: %v\n", err)
			continue
		}

		_, validTags, err := validateConfigs(ctx, testRunner, batchConfigs, validErrFile)
		if err != nil {
			fmt.Printf("Batch validation error: %v\n", err)
			testRunner.Close()
			continue
		}

		if len(validTags) == 0 {
			fmt.Println("No valid configurations in this batch.")
			testRunner.Close()
			continue
		}

		latencyResults, _, err := runLatencyTest(ctx, validTags, ltSettings, testRunner)
		if err != nil {
			fmt.Printf("Latency test error: %v\n", err)
			testRunner.Close()
			continue
		}

		if len(latencyResults) == 0 {
			fmt.Println("No good results in this batch.")
			testRunner.Close()
			continue
		}

		allLatencyResults = append(allLatencyResults, latencyResults...)
		testRunner.Close()
	}

	if len(allLatencyResults) == 0 {
		fmt.Println("No good results")
		os.Exit(1)
	}

	sort.Slice(allLatencyResults, func(i, j int) bool {
		s1 := allLatencyResults[i].Error == nil
		s2 := allLatencyResults[j].Error == nil
		if s1 && s2 {
			return allLatencyResults[i].Delay < allLatencyResults[j].Delay
		}
		return s1 && !s2
	})

	if err := writeResultsToFile(ltResultsFile, NewLatencyResultWriters(allLatencyResults), validConfigs); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	if runSpeedTestFlag && len(validConfigs) > 0 {
		testRunner, err := createTestRunner(workerPath, workerDebug, workerLogFile)
		if err != nil {
			fmt.Printf("Failed to create test runner for speed test: %v\n", err)
		} else {
			_, speedValidTags, err := validateConfigs(ctx, testRunner, validConfigs, validErrFile)
			if err != nil {
				fmt.Printf("Speed test validation error: %v\n", err)
			} else if len(speedValidTags) > 0 {
				speedResults, _, err := runSpeedTest(ctx, speedValidTags, stSettings, testRunner)
				if err != nil {
					fmt.Printf("Speed test error: %v\n", err)
				} else if err := writeResultsToFile(stResultsFile, NewSpeedResultWriters(speedResults), validConfigs); err != nil {
					fmt.Fprintf(os.Stderr, "%v\n", err)
				}
			}
			testRunner.Close()
		}
	}

	fmt.Println("Shutting down...")
}
