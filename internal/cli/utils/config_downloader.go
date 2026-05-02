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

type DownloadSettings struct {
	InputFilePath   string
	OutputFilePath  string
	Timeout         time.Duration
	MaxSubSizeBytes int64
	OnError         func(msg string)
	OnDLStart       func(url string)
	OnDLSuccess     func(url string, configCount int)
	OnDLFailure     func(url string, reason string)
	OnSummary       func(successCount, totalConfigs int, outputFile string)
}

func DownloadConfigs(s DownloadSettings) error {
	links, err := readLines(s.InputFilePath)
	if err != nil {
		if s.OnError != nil {
			s.OnError(fmt.Sprintf("input file '%s' not found", s.InputFilePath))
		}
		return err
	}

	err = os.WriteFile(s.OutputFilePath, []byte(""), 0644)
	if err != nil {
		if s.OnError != nil {
			s.OnError(fmt.Sprintf("error creating output file: %v", err))
		}
		return err
	}

	client := &http.Client{
		Timeout: s.Timeout,
	}

	downloadSuccessCount := 0
	allConfigsCount := 0

	outF, err := os.OpenFile(s.OutputFilePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		if s.OnError != nil {
			s.OnError(fmt.Sprintf("error opening output file: %v", err))
		}
		return err
	}

	for _, url := range links {
		if s.OnDLStart != nil {
			s.OnDLStart(url)
		}

		resp, err := client.Get(url)
		if err != nil {
			if s.OnDLFailure != nil {
				s.OnDLFailure(url, "downloading")
			}
			continue
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, s.MaxSubSizeBytes))
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			if s.OnDLFailure != nil {
				s.OnDLFailure(url, "reading response")
			}
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
					if s.OnError != nil {
						s.OnError(fmt.Sprintf("error writing to file: %s", s.OutputFilePath))
					}
					outF.Close()
					return err
				}
			}
		}

		allConfigsCount += configCount
		downloadSuccessCount++
		if s.OnDLSuccess != nil {
			s.OnDLSuccess(url, configCount)
		}
	}

	err = outF.Close()
	if err != nil {
		if s.OnError != nil {
			s.OnError(fmt.Sprintf("error closing file: %s", s.OutputFilePath))
		}
		return err
	}

	if s.OnSummary != nil {
		s.OnSummary(downloadSuccessCount, allConfigsCount, s.OutputFilePath)
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
