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
	"github.com/bluegradienthorizon/proxytoolbox/worker"
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
		Mode:        worker.SpeedTestModeDownload,
		TestLimit:   5,
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
		configsUris = append(configsUris, line)
	}

	fmt.Println("before dedup:", len(configsUris))
	configsUris = utils.NaiveDeduplicateConfigsUris(configsUris)
	fmt.Println("after dedup:", len(configsUris))

	parsingErrorsMap := make(map[string]int)
	for _, connUri := range configsUris {
		p, err := parsers.ParseConfig(connUri)
		if err != nil {
			parsingErrorsMap[err.Error()]++
			continue
		}
		configs = append(configs, *p)
	}

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

	runner, err := runner.NewTestRunner(runner.RunnerSettings{
		WorkerPath:  workerPath,
		WorkerDebug: workerDebug,
	})
	if err != nil {
		fmt.Printf("Failed to create test runner: %v\n", err)
		os.Exit(1)
	}
	defer runner.Close()

	latencyResults, taggedConfigs, ltErr := runLatencyTest(ctx, configs, ltSettings, runner)
	if ltErr != nil {
		fmt.Printf("Latency test error: %v\n", ltErr)
		os.Exit(1)
	}

	if len(latencyResults) == 0 {
		fmt.Println("No good results")
		os.Exit(1)
	}

	// Write results to file
	writeResultsToFile(latencyResults, taggedConfigs)

	// Run speed tests if enabled
	if runSpeedTestFlag {
		var speedErr error
		_, taggedConfigs, speedErr = runSpeedTest(ctx, taggedConfigs, stSettings, runner)
		if speedErr != nil {
			fmt.Printf("Speed test error: %v\n", speedErr)
		}
	}

	fmt.Println("Shutting down...")
}

// Writes successful latency test results to out.txt
func writeResultsToFile(sortedResults []worker.LatencyTestResult, configs []parsers.ProxyConfig) {
	success := 0
	f, err := os.Create("out.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output: %v\n", err)
		return
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
				w.WriteString(uri + "\n")
			} else {
				println("result tag is missing!!! " + r.Tag)
			}
		}
	}
	w.Flush()

	fmt.Printf("success %d\n", success)
}
