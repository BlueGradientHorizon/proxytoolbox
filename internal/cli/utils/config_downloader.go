package utils

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func DownloadConfigs(inputFile string, outputFile string, timeout time.Duration) {
	if _, err := os.Stat(outputFile); err == nil {
		fmt.Printf("Output file '%s' exists. Redownload? y/n: ", outputFile)
		reader := bufio.NewReader(os.Stdin)
		ans, _ := reader.ReadString('\n')
		ans = strings.ToLower(strings.TrimSpace(ans))

		if ans == "" {
			fmt.Println("Assume no.")
			return
		}
		if strings.HasPrefix(ans, "n") {
			return
		}
	}

	links, err := readLines(inputFile)
	if err != nil {
		fmt.Printf("Error: Input file '%s' not found.\n", inputFile)
		fmt.Printf("Please create '%s' and list one config link per line.\n", inputFile)
		os.Exit(1)
	}

	err = os.WriteFile(outputFile, []byte(""), 0644)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		return
	}
	fmt.Printf("Prepared '%s' for writing.\n---\n", outputFile)

	client := &http.Client{
		Timeout: timeout,
	}

	downloadSuccessCount := 0
	allConfigsCount := 0

	outF, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("    -> Error opening output file for append: %v\n", err)
		return
	}

	for _, url := range links {
		fmt.Printf("Processing: %s\n", url)

		resp, err := client.Get(url)
		if err != nil {
			fmt.Printf("    -> Error downloading %s. Skipping.\n", url)
			continue
		}

		const maxSubSize = 64 * 1024 * 1024
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxSubSize))
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			fmt.Printf("    -> Error reading response from %s. Skipping.\n", url)
			continue
		}

		content := string(body)

		encodings := []*base64.Encoding{
			base64.StdEncoding, base64.RawStdEncoding,
			base64.URLEncoding, base64.RawURLEncoding,
		}

		for _, enc := range encodings {
			if decoded, err := enc.DecodeString(content); err == nil {
				content = string(decoded)
				break
			}
		}

		lines := strings.Split(content, "\n")
		configCount := 0

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
				continue
			}
			// TODO REMOVE
			// if s := strings.Split(line, "://"); (s[0] != "hy2") && (s[0] != "hysteria2") {
			// 	continue
			// }
			if strings.Contains(line, "://") {
				configCount++
				_, err := outF.WriteString(line + "\n")
				if err != nil {
					fmt.Printf("Error writing to file: %s\n", outputFile)
					return
				}
			}
		}

		allConfigsCount += configCount
		downloadSuccessCount++
		fmt.Printf("    -> Successfully downloaded. Found %d potential configs.\n", configCount)
	}

	fmt.Println("---")
	fmt.Printf("Successfully concatenated %d subscriptions. Found configs: %d.\n", downloadSuccessCount, allConfigsCount)
	err = outF.Close()
	if err != nil {
		fmt.Printf("Error when closing file: %s\n", outputFile)
		return
	}
	fmt.Printf("Final configurations saved to: %s\n", outputFile)
	fmt.Println("---")
}

// Helper function to read non-empty, non-comment lines
func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}
