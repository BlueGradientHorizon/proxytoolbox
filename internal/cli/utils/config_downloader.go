package utils

import (
	"bufio"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func DownloadConfigs(inputFile string, outputFile string, timeout time.Duration/*, overwrite bool*/) error {
	/*if !overwrite {
		if _, err := os.Stat(outputFile); err == nil {
			return nil
		}
	}*/

	links, err := readLines(inputFile)
	if err != nil {
		return err
	}

	err = os.WriteFile(outputFile, []byte(""), 0644)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: timeout,
	}

	downloadSuccessCount := 0
	allConfigsCount := 0

	outF, err := os.OpenFile(outputFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer outF.Close()

	for _, url := range links {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}

		const maxSubSize = 64 * 1024 * 1024
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxSubSize))
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
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
			if strings.Contains(line, "://") {
				configCount++
				_, err := outF.WriteString(line + "\n")
				if err != nil {
					return err
				}
			}
		}

		allConfigsCount += configCount
		downloadSuccessCount++
	}

	return nil
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
