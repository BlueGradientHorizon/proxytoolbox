package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/internal/cli/utils"
	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
	"github.com/bluegradienthorizon/proxytoolbox/testrunner"
)

func main() {
	var testerDebug bool
	flag.BoolVar(&testerDebug, "tester-debug", false, "Print tester stdout and stderr")
	flag.Parse()

	// Configure latency test parameters
	ltSettings := LatencyTestSettings{
		Concurrency: 0,
		Timeout:     7 * time.Second,
		Rounds:      3,
	}

	runSpeedTestFlag := false // Set to true to run speed tests after latency tests

	stSettings := SpeedTestSettings{
		Concurrency: 1,
		Rounds:      2,
		Timeout:     10 * time.Second,
		Mode:        testers.Download,
		TestLimit:   5,
		TargetBytes: 10 * 1024 * 1024,
	}

	reg := testrunner.NewRegistry()
	// Scan only the directory where built tester binaries live.
	reg.Discover("./bin")

	testersMap := reg.All()
	if len(testersMap) == 0 {
		fmt.Println("No tester programs found.")
		return
	}

	fmt.Println("Found tester programs:")
	var testerPath string
	for _, list := range testersMap {
		for _, info := range list {
			fmt.Printf("- %s (%s) at %s\n", info.Name, info.Version, info.Path)
			if testerPath == "" {
				testerPath = info.Path
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

	var configsConnUris []string

	content := strings.TrimSpace(string(data))
	for _, line := range strings.Split(content, "\n") {
		configsConnUris = append(configsConnUris, line)
	}

	fmt.Println("before dedup:", len(configsConnUris))
	configsConnUris = utils.DeduplicateConnUris(configsConnUris)
	fmt.Println("after dedup:", len(configsConnUris))

	parsingErrorsMap := make(map[string]int)
	for _, connUri := range configsConnUris {
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

	ltTesterSettings := testrunner.TesterSettings{
		TesterPath:  testerPath,
		TesterDebug: testerDebug,
	}

	latencyResults, taggedConfigs, ltErr := runLatencyTest(ctx, configs, ltSettings, ltTesterSettings)
	if ltErr != nil {
		fmt.Printf("Latency test error: %v\n", ltErr)
		os.Exit(-1)
	}

	if len(latencyResults) == 0 {
		fmt.Println("No good results")
		os.Exit(-1)
	}

	// Write results to file
	writeResultsToFile(latencyResults, taggedConfigs)

	stTesterSettings := testrunner.TesterSettings{
		TesterPath:  testerPath,
		TesterDebug: testerDebug,
	}

	// Run speed tests if enabled
	if runSpeedTestFlag {
		// var speedResults []testers.SpeedTestResult
		var speedErr error
		_, taggedConfigs, speedErr = runSpeedTest(ctx, taggedConfigs, stSettings, stTesterSettings)
		if speedErr != nil {
			fmt.Printf("Speed test error: %v\n", speedErr)
		}
	}

	fmt.Println("Shutting down...")
}

// writeResultsToFile writes successful latency test results to out.txt
func writeResultsToFile(sortedResults []testers.LatencyTestResult, configs []parsers.ProxyConfig) {
	success := 0
	f, _ := os.Create("out.txt")
	w := bufio.NewWriter(f)
	defer f.Close()

	for _, r := range sortedResults {
		if r.Error == nil {
			success++
			i := slices.IndexFunc(configs, func(p parsers.ProxyConfig) bool {
				return p.Config.Tag == r.Tag
			})
			if i == -1 {
				i = 0
			}
			w.WriteString(configs[i].ConnURI + "\n")
		}
	}
	w.Flush()

	fmt.Printf("success %d\n", success)
}
