package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
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
	var workers []registry.WorkerInfo
	for _, list := range workersMap {
		for _, info := range list {
			fmt.Printf("- %s (%s) at %s\n", info.Name, info.Version, info.Path)
			workers = append(workers, info)
		}
	}

	if len(workers) == 1 {
		return workers[0].Path
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("Select worker (1-%d): ", len(workers))
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input.")
			continue
		}
		input = strings.TrimSpace(input)
		num, err := strconv.Atoi(input)
		if err != nil || num < 1 || num > len(workers) {
			fmt.Println("Invalid selection. Please enter a valid number.")
			continue
		}
		return workers[num-1].Path
	}
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

func validateConfigs(ctx context.Context, testRunner *runner.TestRunner, configs []parsers.ProxyConfig, validErrFile string) ([]parsers.ProxyConfig, []string, error) {
	taggedConfigs, validationErrors, err := testRunner.Validate(ctx, configs)
	if err != nil {
		return nil, nil, fmt.Errorf("validation error: %w", err)
	}

	if len(validationErrors) > 0 {
		validF, err := os.OpenFile(validErrFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create %s: %w", validErrFile, err)
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
	} else {
		println("no validation errors")
	}

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

	return validConfigs, validTags, nil
}

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
