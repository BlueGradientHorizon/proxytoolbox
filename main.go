package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
	"github.com/bluegradienthorizon/proxytoolbox/testrunner"
	"github.com/bluegradienthorizon/proxytoolbox/tools"
	"github.com/bluegradienthorizon/proxytoolbox/utils"
)

func main() {
	// Configure latency test parameters
	latencyParams := LatencyTestParams{
		Concurrency: 0,
		Timeout:     7 * time.Second,
		Rounds:      3,
	}

	runSpeedTestFlag := false // Set to true to run speed tests after latency tests

	speedParams := SpeedTestParams{
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
	tools.DownloadConfigs(inputFile, outputFile, 10*time.Second)

	fmt.Printf("Attempting to load configurations from file: %s\n", outputFile)

	var profiles []parsers.ProxyProfile
	data, err := os.ReadFile(outputFile)
	if err != nil {
		fmt.Printf("File %s not found\n", outputFile)
		return
	}

	var profilesConnUris []string

	content := strings.TrimSpace(string(data))
	for _, line := range strings.Split(content, "\n") {
		profilesConnUris = append(profilesConnUris, line)
	}

	fmt.Println("before dedup:", len(profilesConnUris))
	profilesConnUris = utils.DeduplicateConnUris(profilesConnUris)
	fmt.Println("after dedup:", len(profilesConnUris))

	parsingErrorsMap := make(map[string]int)
	for _, connUri := range profilesConnUris {
		p, err := parsers.ParseProfile(connUri)
		if err != nil {
			parsingErrorsMap[err.Error()]++
			continue
		}
		profiles = append(profiles, *p)
	}

	println("parsing errors:")
	parsingErrors := 0
	for err, count := range parsingErrorsMap {
		fmt.Println(count, "x", err)
		parsingErrors += count
	}
	println("parsing errors total:", parsingErrors)

	if len(profiles) == 0 {
		fmt.Println("! No valid configurations were loaded. Check your source or subscription content.")
		return
	}

	ctx := context.Background()

	latencyResults, taggedProfiles, ltErr := runLatencyTest(ctx, profiles, latencyParams, testerPath)
	if ltErr != nil {
		fmt.Printf("Latency test error: %v\n", ltErr)
		os.Exit(-1)
	}

	if len(latencyResults) == 0 {
		fmt.Println("No good results")
		os.Exit(-1)
	}

	// Write results to file
	writeResultsToFile(latencyResults, taggedProfiles)

	// Run speed tests if enabled
	if runSpeedTestFlag {
		// var speedResults []testers.SpeedTestResult
		var speedErr error
		_, taggedProfiles, speedErr = runSpeedTest(ctx, taggedProfiles, speedParams, testerPath)
		if speedErr != nil {
			fmt.Printf("Speed test error: %v\n", speedErr)
		}
	}

	fmt.Println("Shutting down...")
}

// writeResultsToFile writes successful latency test results to out.txt
func writeResultsToFile(sortedResults []testers.LatencyTestResult, profiles []parsers.ProxyProfile) {
	success := 0
	f, _ := os.Create("out.txt")
	w := bufio.NewWriter(f)
	defer f.Close()

	for _, r := range sortedResults {
		if r.Error == nil {
			success++
			i := slices.IndexFunc(profiles, func(p parsers.ProxyProfile) bool {
				return p.Config.Tag == r.Tag
			})
			if i == -1 {
				i = 0
			}
			w.WriteString(profiles[i].ConnURI + "\n")
		}
	}
	w.Flush()

	fmt.Printf("success %d\n", success)
}
