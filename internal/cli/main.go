package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/presets"
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
		TestURL:     presets.Google204,
	}
	runSpeedTestFlag := true
	stSettings := speedTestSettings{
		Concurrency: 0,
		Rounds:      1,
		Timeout:     10 * time.Second,
		Mode:        runner.SpeedTestModeDownload,
		TestLimit:   0,
		TargetBytes: 1024,
		Provider:    presets.CloudflareProvider,
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

	testRunner, err := createTestRunner(workerPath, workerDebug, workerLogFile)
	if err != nil {
		fmt.Printf("Failed to create test runner: %v\n", err)
		os.Exit(1)
	}
	defer testRunner.Close()

	fmt.Println("Validating all configs...")
	validConfigs, _, err := validateConfigs(ctx, testRunner, configs, validErrFile)
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

		_, validTags, err := validateConfigs(ctx, testRunner, batchConfigs, validErrFile)
		if err != nil {
			fmt.Printf("Batch validation error: %v\n", err)
			continue
		}

		if len(validTags) == 0 {
			fmt.Println("No valid configurations in this batch.")
			continue
		}

		latencyResults, _, err := runLatencyTest(ctx, validTags, ltSettings, testRunner)
		if err != nil {
			fmt.Printf("Latency test error: %v\n", err)
			continue
		}

		allLatencyResults = append(allLatencyResults, latencyResults...)
	}

	if len(allLatencyResults) == 0 {
		fmt.Println("No good results")
		os.Exit(1)
	}

	sortLatencyResults(allLatencyResults)

	if err := writeResultsToFile(ltResultsFile, NewLatencyResultWriters(allLatencyResults), validConfigs); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	latencyPassedConfigs := filterPassedConfigs(validConfigs, allLatencyResults)

	if runSpeedTestFlag && len(latencyPassedConfigs) > 0 {
		handleSpeedTests(ctx, testRunner, latencyPassedConfigs, allLatencyResults, stSettings, validErrFile, stResultsFile)
	}

	fmt.Println("Shutting down...")
}
