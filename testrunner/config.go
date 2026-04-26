package testrunner

import (
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/testers"
)

// TestRunnerConfig configures test runner behavior and tester path.
type TestRunnerConfig struct {
	// TesterPath is the absolute path to the tester binary.
	TesterPath string

	// TesterDebug enables printing of tester stdout and stderr.
	TesterDebug bool
}

// BaseTestRunnerConfig contains common configuration fields shared by all test types.
type BaseTestRunnerConfig struct {
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
	CoreCreatedCallback func(validationErrors map[string]int)

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

// LatencyTestRunnerConfig configures latency test execution parameters.
type LatencyTestRunnerConfig struct {
	BaseTestRunnerConfig

	// TestURL specifies the URL to test latency against
	// Common values: testers.Google204, testers.Cloudflare, custom URLs
	TestURL string
}

// SpeedTestRunnerConfig configures speed test execution parameters.
type SpeedTestRunnerConfig struct {
	BaseTestRunnerConfig

	// TargetBytes specifies how many bytes to transfer during the test
	// Larger values provide more accurate measurements but take longer
	TargetBytes int64

	// Mode specifies the speed test mode (Download or Upload)
	Mode testers.SpeedTestMode

	// Provider specifies which speed test provider to use
	// Common values: testers.CloudflareProvider, testers.CustomProvider
	Provider testers.SpeedTestProvider
}
