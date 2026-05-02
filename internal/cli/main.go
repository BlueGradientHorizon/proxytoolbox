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

func parseFlags() bool {
	var workerDebug bool
	flag.BoolVar(&workerDebug, "worker-debug", false, "Print worker stdout and stderr")
	flag.Parse()
	return workerDebug
}

func discoverWorker() string {
	reg := registry.NewRegistry()
	reg.Discover("./bin")

	workersMap := reg.All()
	if len(workersMap) == 0 {
		return ""
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
	return workerPath
}

func handleConfigDownload(outputFile, inputFile string) error {
	overwrite := promptOverwrite(outputFile)
	if !overwrite {
		return nil
	}

	settings := utils.DownloadSettings{
		InputFilePath:   inputFile,
		OutputFilePath:  outputFile,
		Timeout:         10 * time.Second,
		MaxSubSizeBytes: 32 * 1024 * 1024,
		OnError: func(msg string) {
			fmt.Print(msg)
		},
		OnDLStart: func(url string) {
			fmt.Printf("Processing: %s\n", url)
		},
		OnDLSuccess: func(url string, configCount int) {
			fmt.Printf("    -> Successfully downloaded. Found %d potential configs.\n", configCount)
		},
		OnDLFailure: func(url string, reason string) {
			fmt.Printf("    -> Error (%s) %s. Skipping.\n", reason, url)
		},
		OnSummary: func(successCount, totalConfigs int, outputFile string) {
			fmt.Println("---")
			fmt.Printf("Successfully concatenated %d subscriptions. Found configs: %d.\n", successCount, totalConfigs)
			fmt.Printf("Final configurations saved to: %s\n", outputFile)
			fmt.Println("---")
		},
	}

	if err := utils.DownloadConfigs(settings); err != nil {
		return err
	}
	fmt.Printf("Configurations saved to: %s\n", outputFile)
	return nil
}

func promptOverwrite(outputFile string) bool {
	if _, err := os.Stat(outputFile); err == nil {
		fmt.Printf("Output file '%s' exists. Redownload? y/n: ", outputFile)
		reader := bufio.NewReader(os.Stdin)
		ans, _ := reader.ReadString('\n')
		ans = strings.ToLower(strings.TrimSpace(ans))

		if ans == "" {
			fmt.Println("Assume no.")
			return false
		}
		if strings.HasPrefix(ans, "n") {
			return false
		}
		if strings.HasPrefix(ans, "y") {
			return true
		}
		return false
	}
	return true
}

func loadAndParseConfigs(outputFile, parseErrFile string) []parsers.ProxyConfig {
	fmt.Printf("Attempting to load configurations from file: %s\n", outputFile)

	data, err := os.ReadFile(outputFile)
	if err != nil {
		fmt.Printf("File %s not found\n", outputFile)
		return nil
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

	parseF, err := os.Create(parseErrFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create %s: %v\n", parseErrFile, err)
		os.Exit(1)
	}
	defer parseF.Close()

	parsingErrorsMap := make(map[string]int)
	var configs []parsers.ProxyConfig
	for _, connUri := range configsUris {
		p, err := parsers.ParseConfig(connUri)
		if err != nil {
			parsingErrorsMap[err.Error()]++
			parseF.WriteString(connUri + "\n" + err.Error() + "\n")
			continue
		}
		configs = append(configs, *p)
	}

	println("parsing errors:")
	parsingErrors := 0
	for errStr, count := range parsingErrorsMap {
		fmt.Println(count, "x", errStr)
		parsingErrors += count
	}
	println("parsing errors total:", parsingErrors)

	return configs
}

func createTestRunner(workerPath string, workerDebug bool, workerLogFile string) (*runner.TestRunner, error) {
	var workerLogPath string
	if workerDebug {
		workerLogPath = workerLogFile
	}
	return runner.NewTestRunner(runner.RunnerSettings{
		WorkerPath:    workerPath,
		WorkerLogPath: workerLogPath,
	})
}

func validateConfigs(ctx context.Context, testRunner *runner.TestRunner, configs []parsers.ProxyConfig, validErrFile string) ([]parsers.ProxyConfig, []string) {
	taggedConfigs, validationErrors, err := testRunner.Validate(ctx, configs)
	if err != nil {
		fmt.Printf("Validation error: %v\n", err)
		os.Exit(1)
	}

	validF, err := os.Create(validErrFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create %s: %v\n", validErrFile, err)
		os.Exit(1)
	}
	defer validF.Close()

	validationErrorsMap := make(map[string]int)
	for _, errPair := range validationErrors {
		validationErrorsMap[errPair.Error]++
		validF.WriteString(errPair.Tag + "\n" + errPair.Error + "\n")
	}

	println("validation errors:")
	validationErrsTotal := 0
	for errStr, count := range validationErrorsMap {
		fmt.Println(count, "x", errStr)
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

	return validConfigs, validTags
}

// Writes successful latency test results to out.txt
func writeResultsToFile(filename string, sortedResults []runner.LatencyTestResult, configs []parsers.ProxyConfig) error {
	success := 0
	f, err := os.Create(filename)
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

const (
	inputFile     = "link_list.txt"
	outputFile    = "configs.txt"
	parseErrFile  = "parseErr.txt"
	validErrFile  = "validErr.txt"
	resultsFile   = "out.txt"
	workerLogFile = "worker.log"
)

func main() {
	workerDebug := parseFlags()

	ltSettings := LatencyTestSettings{
		Concurrency: 0,
		Timeout:     7 * time.Second,
		Rounds:      3,
	}
	runSpeedTestFlag := true
	stSettings := SpeedTestSettings{
		Concurrency: 1,
		Rounds:      1,
		Timeout:     10 * time.Second,
		Mode:        runner.SpeedTestModeDownload,
		TestLimit:   10,
		TargetBytes: 10 * 1024 * 1024,
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

	validConfigs, validTags := validateConfigs(ctx, testRunner, configs, validErrFile)
	if len(validTags) == 0 {
		fmt.Println("No valid configurations after validation.")
		return
	}

	latencyResults, successfulTags, err := runLatencyTest(ctx, validTags, ltSettings, testRunner)
	if err != nil {
		fmt.Printf("Latency test error: %v\n", err)
		os.Exit(1)
	}

	if len(latencyResults) == 0 {
		fmt.Println("No good results")
		os.Exit(1)
	}

	if err := writeResultsToFile(resultsFile, latencyResults, validConfigs); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}

	if runSpeedTestFlag {
		if _, _, err := runSpeedTest(ctx, successfulTags, stSettings, testRunner); err != nil {
			fmt.Printf("Speed test error: %v\n", err)
		}
	}

	fmt.Println("Shutting down...")
}
