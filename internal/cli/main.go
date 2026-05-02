package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/internal/cli/utils"
	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/registry"
	"github.com/bluegradienthorizon/proxytoolbox/runner"
)

func main() {
	var workerDebug bool
	flag.BoolVar(&workerDebug, "worker-debug", false, "Print worker stdout and stderr")
	flag.Parse()

	// Configure latency test parameters
	var ltSettings = LatencyTestSettings{
		Concurrency: 0,
		Timeout:     7 * time.Second,
		Rounds:      3,
	}

	// Set this to true if you want to perform speed test after latency test
	var runSpeedTestFlag = true

	// Configure speed test parameters
	var stSettings = SpeedTestSettings{
		Concurrency: 1,
		Rounds:      1,
		Timeout:     10 * time.Second,
		Mode:        runner.SpeedTestModeDownload,
		TestLimit:   10,
		TargetBytes: 10 * 1024 * 1024,
	}

	reg := registry.NewRegistry()
	// Scan only the directory where built worker binaries live.
	reg.Discover("./bin")

	workersMap := reg.All()
	if len(workersMap) == 0 {
		fmt.Println("No worker programs found.")
		return
	}

	fmt.Println("Found worker programs:")
	var workerPath string
	for _, list := range workersMap {
		for _, info := range list {
			fmt.Printf("- %s (%s) at %s\n", info.Name, info.Version, info.Path)
			if workerPath == "" {
				workerPath = info.Path
			}
		}
	}

	inputFile := "link_list.txt"
	outputFile := "configs.txt"
	utils.DownloadConfigs(inputFile, outputFile, 10*time.Second)

	fmt.Printf("Attempting to load configurations from file: %s\n", outputFile)

	var configs []parsers.ProxyConfig
	data, err := os.ReadFile(outputFile)
	if err != nil {
		fmt.Printf("File %s not found\n", outputFile)
		return
	}

	var configsUris []string

	content := strings.TrimSpace(string(data))
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		configsUris = append(configsUris, line)
	}

	fmt.Println("before dedup:", len(configsUris))
	configsUris = utils.NaiveDeduplicateConfigsUris(configsUris)
	fmt.Println("after dedup:", len(configsUris))

	parseF, err := os.Create("parseErr.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create parseErr.txt: %v\n", err)
		os.Exit(1)
	}
	parsingErrorsMap := make(map[string]int)
	for _, connUri := range configsUris {
		p, err := parsers.ParseConfig(connUri)
		if err != nil {
			parsingErrorsMap[err.Error()]++
			parseF.WriteString(connUri + "\n" + err.Error() + "\n")
			continue
		}
		configs = append(configs, *p)
	}
	parseF.Close()

	println("parsing errors:")
	parsingErrors := 0
	for err, count := range parsingErrorsMap {
		fmt.Println(count, "x", err)
		parsingErrors += count
	}
	println("parsing errors total:", parsingErrors)

	if len(configs) == 0 {
		fmt.Println("! No valid configurations were loaded. Check your source or subscription content.")
		return
	}

	ctx := context.Background()

	testRunner, err := runner.NewTestRunner(runner.RunnerSettings{
		WorkerPath:  workerPath,
		WorkerDebug: workerDebug,
	})
	if err != nil {
		fmt.Printf("Failed to create test runner: %v\n", err)
		os.Exit(1)
	}
	defer testRunner.Close()

	taggedConfigs, validationErrors, err := testRunner.Validate(ctx, configs)
	if err != nil {
		fmt.Printf("Validation error: %v\n", err)
		os.Exit(1)
	}

	validF, err := os.Create("validErr.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create validErr.txt: %v\n", err)
		os.Exit(1)
	}
	validationErrorsMap := make(map[string]int)
	for _, errPair := range validationErrors {
		validationErrorsMap[errPair.Error]++
		validF.WriteString(errPair.Tag + "\n" + errPair.Error + "\n")
	}
	validF.Close()

	println("validation errors:")
	validationErrsTotal := 0
	for err, count := range validationErrorsMap {
		fmt.Println(count, "x", err)
		validationErrsTotal += count
	}
	println("validation errors total:", validationErrsTotal)

	validConfigs := make([]parsers.ProxyConfig, 0, len(taggedConfigs))
	validTags := make([]string, 0, len(taggedConfigs))
	errMap := make(map[string]bool)
	for _, ve := range validationErrors {
		errMap[ve.Tag] = true
	}
	for _, c := range taggedConfigs {
		if c.Config != nil && !errMap[c.Config.Tag] {
			validConfigs = append(validConfigs, c)
			validTags = append(validTags, c.Config.Tag)
		}
	}

	if len(validTags) == 0 {
		fmt.Println("No valid configurations after validation.")
		return
	}

	latencyResults, successfulTags, ltErr := runLatencyTest(ctx, validTags, ltSettings, testRunner)
	if ltErr != nil {
		fmt.Printf("Latency test error: %v\n", ltErr)
		os.Exit(1)
	}

	if len(latencyResults) == 0 {
		fmt.Println("No good results")
		os.Exit(1)
	}

	// Write results to file
	if err := writeResultsToFile(latencyResults, validConfigs); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	// Run speed tests if enabled
	if runSpeedTestFlag {
		var speedErr error
		_, _, speedErr = runSpeedTest(ctx, successfulTags, stSettings, testRunner)
		if speedErr != nil {
			fmt.Printf("Speed test error: %v\n", speedErr)
		}
	}

	fmt.Println("Shutting down...")
}

// Writes successful latency test results to out.txt
func writeResultsToFile(sortedResults []runner.LatencyTestResult, configs []parsers.ProxyConfig) error {
	success := 0
	f, err := os.Create("out.txt")
	if err != nil {
		return fmt.Errorf("failed to create output: %w", err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)

	tagToURI := make(map[string]string, len(configs))
	for _, p := range configs {
		if p.Config != nil {
			tagToURI[p.Config.Tag] = p.ConnURI
		}
	}
	for _, r := range sortedResults {
		if r.Error == nil {
			success++
			if uri, ok := tagToURI[r.Tag]; ok {
				w.WriteString(uri)
				w.WriteString("\n")
			} else {
				println("result tag is missing!!! " + r.Tag)
			}
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush output: %w", err)
	}

	fmt.Printf("success %d\n", success)
	return nil
}
