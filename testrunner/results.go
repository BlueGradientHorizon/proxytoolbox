package testrunner

import (
	"github.com/bluegradienthorizon/proxytoolbox/testers"
)

// BaseTestResults contains common fields shared by all test result types.
type BaseTestResults struct {
	// SuccessCount is the number of successful tests
	SuccessCount int

	// FailureCount is the number of failed tests
	FailureCount int

	// ValidationErrors maps error messages to occurrence counts
	// Collected during configuration validation before testing begins
	ValidationErrors map[string]int
}

// LatencyTestResults contains aggregated results from latency testing.
// It provides both individual test results and summary statistics.
type LatencyTestResults struct {
	BaseTestResults

	// Results contains all test results from the final round
	// Includes both successful and failed tests depending on configuration
	Results []testers.LatencyTestResult
}

// SpeedTestResults contains aggregated results from speed testing.
// It provides both individual test results and summary statistics.
type SpeedTestResults struct {
	BaseTestResults

	// Results contains all test results
	// Includes both successful and failed tests
	Results []testers.SpeedTestResult
}
