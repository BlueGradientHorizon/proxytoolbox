package runner

import (
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/worker"
)

// RunnerSettings configures test runner behavior and worker path.
type RunnerSettings struct {
	// WorkerPath is the absolute path to the worker binary.
	WorkerPath string

	// WorkerDebug enables printing of worker stdout and stderr.
	WorkerDebug bool
}

// ValidationError represents an error validating proxy configurations
type ValidationError = worker.ValidationError

// LatencyTestResult contains the result of a latency test for a single proxy.
type LatencyTestResult = worker.LatencyTestResult

// SpeedTestResult contains the result of a speed test for a single proxy.
type SpeedTestResult = worker.SpeedTestResult

// SpeedTestMode indicates whether to test download or upload speed.
type SpeedTestMode = worker.SpeedTestMode

const (
	SpeedTestModeDownload = worker.SpeedTestModeDownload
	SpeedTestModeUpload   = worker.SpeedTestModeUpload
)

// BaseTestRunnerSettings contains common configuration fields shared by all test types.
type BaseTestRunnerSettings struct {
	// Timeout for individual tests
	// If a test doesn't complete within this duration, it's marked as failed
	Timeout time.Duration

	// Rounds specifies how many test rounds to execute
	// Multiple rounds help identify consistently performing proxies
	Rounds int

	// Concurrency limits how many outbounds are tested simultaneously in a single round.
	// If set to 0 or negative, all outbounds are tested concurrently.
	Concurrency int

	// CoreCreatedCallback is called after core creation with validation errors
	// It allows initialization of progress tracking with accurate totals
	// Optional: can be nil if not needed
	CoreCreatedCallback func(validationErrors []ValidationError)

	// RoundStartedCallback is called at the start of each test round
	// It receives the current round number (0-indexed)
	// Optional: can be nil if not needed
	RoundStartedCallback func(round int, outboundsLen int)

	// RoundEndedCallback is called at the end of each test round
	// It receives the current round number (0-indexed)
	// Optional: can be nil if not needed
	RoundEndedCallback func(round int)

	// ProgressCallback receives real-time test updates as each proxy completes
	// Optional: can be nil if progress reporting is not needed
	// The actual type depends on the test type (LatencyTestResult or SpeedTestResult)
	ProgressCallback interface{}

	// FilterFailed removes failed proxies between rounds
	// When true, only successful proxies from round N proceed to round N+1
	FilterFailed bool

	// SortResults sorts results by performance metric (ascending order)
	SortResults bool
}

// LatencyTestRunnerSettings configures latency test execution parameters.
type LatencyTestRunnerSettings struct {
	BaseTestRunnerSettings

	// TestURL specifies the URL to test latency against
	TestURL string
}

// SpeedTestRunnerSettings configures speed test execution parameters.
type SpeedTestRunnerSettings struct {
	BaseTestRunnerSettings

	// TargetBytes specifies how many bytes to transfer during the test
	// Larger values provide more accurate measurements but take longer
	TargetBytes int64

	// Mode specifies the speed test mode (Download or Upload)
	Mode SpeedTestMode

	// Provider specifies which speed test provider to use
	Provider worker.SpeedTestProvider
}

// BaseTestResults contains common fields shared by all test result types.
type BaseTestResults struct {
	// SuccessCount is the number of successful tests
	SuccessCount int

	// FailureCount is the number of failed tests
	FailureCount int

	// ValidationErrors is a list of tag-error pairs for failed configurations
	// Collected during configuration validation before testing begins
	ValidationErrors []ValidationError
}

// LatencyTestResults contains aggregated results from latency testing.
// It provides both individual test results and summary statistics.
type LatencyTestResults struct {
	BaseTestResults

	// Results contains all test results from the final round
	// Includes both successful and failed tests depending on configuration
	Results []LatencyTestResult
}

// SpeedTestResults contains aggregated results from speed testing.
// It provides both individual test results and summary statistics.
type SpeedTestResults struct {
	BaseTestResults

	// Results contains all test results
	// Includes both successful and failed tests
	Results []SpeedTestResult
}
