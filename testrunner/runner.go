package testrunner

import (
	"context"
	"fmt"
	"net"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
	"github.com/sagernet/sing/common/metadata"
)

// TestRunner manages proxy core lifecycle and test execution.
// It provides both high-level convenience methods that handle everything
// automatically and low-level APIs for fine-grained control.
//
// The TestRunner follows a layered design:
//   - High-level API: RunLatencyTests, RunSpeedTests (automatic lifecycle)
//   - Low-level API: CreateCore, StartCore, StopCore, GetOutbounds (manual control)
//
// Resource management:
//   - When AutoCleanup is enabled, high-level methods automatically clean up resources
//   - When using low-level APIs, call Close() manually to clean up resources
//   - Always use defer to ensure cleanup occurs even on error or panic
type TestRunner struct {
	// coreManager manages the lifecycle of the proxy core
	coreManager CoreManager

	// instance is the running core instance (nil until CreateCore is called)
	instance CoreInstance

	// ctx is the context for the test runner lifecycle
	ctx context.Context

	// cancel cancels the test runner context
	cancel context.CancelFunc

	// config stores the test runner configuration
	config TestRunnerConfig
}

// NewTestRunner creates a new test runner with the specified configuration.
// It retrieves the appropriate core manager based on the configured core type.
//
// Parameters:
//   - config: Configuration for the test runner including core type and options
//
// Returns:
//   - *TestRunner: The created test runner instance
//   - error: Error if the core type is not supported
//
// Example:
//
//	runner, err := NewTestRunner(TestRunnerConfig{
//	    CoreType: SingBoxCore,
//	    LogLevel: "panic",
//	    AutoCleanup: true,
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer runner.Close()
func NewTestRunner(config TestRunnerConfig) (*TestRunner, error) {
	// Set default values
	if config.LogLevel == "" {
		config.LogLevel = "panic"
	}
	// AutoCleanup defaults to true if not explicitly set
	// Note: In Go, bool zero value is false, so we can't distinguish between
	// "not set" and "explicitly set to false". For now, we'll assume the caller
	// sets it explicitly if they want to control it.

	// Get the core manager
	coreManager, err := GetCoreManager(config.CoreType, config.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to get core manager: %w", err)
	}

	// Create a context for the test runner lifecycle
	ctx, cancel := context.WithCancel(context.Background())

	return &TestRunner{
		coreManager: coreManager,
		instance:    nil, // Will be set when CreateCore is called
		ctx:         ctx,
		cancel:      cancel,
		config:      config,
	}, nil
}

// Close cleans up resources used by the test runner.
// It stops the core instance if it's running and cancels the context.
// This method is safe to call multiple times.
//
// Returns:
//   - error: Error if cleanup fails (logged but not critical)
//
// Example:
//
//	runner, _ := NewTestRunner(config)
//	defer runner.Close()
func (tr *TestRunner) Close() error {
	// Cancel the context first to signal shutdown
	if tr.cancel != nil {
		tr.cancel()
	}

	// Stop the core instance if it exists
	if tr.instance != nil {
		if err := tr.instance.Stop(); err != nil {
			return fmt.Errorf("failed to stop core instance: %w", err)
		}
		tr.instance = nil
	}

	return nil
}

// CreateCore creates a core instance without starting it.
// This low-level method allows manual control over the lifecycle.
// It extracts OutboundConfig from ProxyProfiles, validates them,
// and stores the created instance in the TestRunner.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - profiles: Slice of proxy profiles containing configurations
//
// Returns:
//   - map[string]int: Validation errors mapped to occurrence counts
//   - error: Error if core creation fails completely
//
// Example:
//
//	validationErrors, err := runner.CreateCore(ctx, profiles)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if len(validationErrors) > 0 {
//	    log.Printf("Validation errors: %v", validationErrors)
//	}
func (tr *TestRunner) CreateCore(ctx context.Context, profiles []parsers.ProxyProfile) (map[string]int, error) {
	// Extract OutboundConfig from ProxyProfiles
	configs := make([]*core.OutboundConfig, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Config != nil {
			configs = append(configs, profile.Config)
		}
	}

	// Check if we have any configurations to create
	if len(configs) == 0 {
		return nil, fmt.Errorf("no valid configurations found in profiles")
	}

	// Call coreManager.Create to create the instance
	instance, validationErrors, err := tr.coreManager.Create(ctx, configs)
	if err != nil {
		return validationErrors, fmt.Errorf("failed to create core: %w", err)
	}

	// Store the created instance in TestRunner
	tr.instance = instance

	return validationErrors, nil
}

// StartCore starts the previously created core instance.
// This low-level method allows manual control over the lifecycle.
// The core must be created with CreateCore before calling this method.
//
// Returns:
//   - error: Error if startup fails or if no instance exists
//
// Example:
//
//	validationErrors, err := runner.CreateCore(ctx, profiles)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if err := runner.StartCore(); err != nil {
//	    log.Fatal(err)
//	}
func (tr *TestRunner) StartCore() error {
	if tr.instance == nil {
		return fmt.Errorf("cannot start core: no instance created (call CreateCore first)")
	}

	if err := tr.instance.Start(); err != nil {
		return fmt.Errorf("failed to start core instance: %w", err)
	}

	return nil
}

// StopCore stops the core instance and cleans up resources.
// This low-level method allows manual control over the lifecycle.
// It's safe to call this method multiple times or when no instance exists.
//
// Returns:
//   - error: Error if shutdown fails (logged but not critical)
//
// Example:
//
//	if err := runner.StopCore(); err != nil {
//	    log.Printf("Warning: failed to stop core: %v", err)
//	}
func (tr *TestRunner) StopCore() error {
	if tr.instance == nil {
		return nil // No instance to stop
	}

	if err := tr.instance.Stop(); err != nil {
		return fmt.Errorf("failed to stop core instance: %w", err)
	}

	// Clear the instance reference after stopping
	tr.instance = nil

	return nil
}

// GetCoreInstance returns the underlying core instance for direct access.
// This low-level method allows advanced users to access core-specific
// functionality that isn't exposed through the TestRunner interface.
//
// Returns:
//   - CoreInstance: The running core instance, or nil if no instance exists
//
// Example:
//
//	instance := runner.GetCoreInstance()
//	if instance != nil {
//	    outbounds := instance.GetOutbounds()
//	    // Use outbounds directly with testers
//	}
func (tr *TestRunner) GetCoreInstance() CoreInstance {
	return tr.instance
}

// GetOutbounds returns all outbounds from the running core instance.
// This is a convenience method that calls instance.GetOutbounds().
// It returns nil if no instance exists or if the instance hasn't been started.
//
// Returns:
//   - []core.Outbound: Slice of outbound interfaces, or nil if no instance exists
//
// Example:
//
//	outbounds := runner.GetOutbounds()
//	if outbounds == nil {
//	    log.Fatal("No core instance running")
//	}
//	// Use outbounds with testers
func (tr *TestRunner) GetOutbounds() []core.Outbound {
	if tr.instance == nil {
		return nil
	}
	return tr.instance.GetOutbounds()
}

// testRunner is a generic interface for running tests
type testRunner[T any] interface {
	Run(resChans ...chan<- T) func()
}

// runTestRound is a generic helper that executes a single round of tests on the provided outbounds.
// It handles the common logic for both latency and speed tests.
func runTestRound[T any](
	ctx context.Context,
	outbounds []core.Outbound,
	concurrency int,
	progressCallback func(T),
	createTest func(ctx context.Context, proxies []testers.ProxyInfo, dialers []testers.DialerFunc) (testRunner[T], error),
) ([]T, error) {
	if len(outbounds) == 0 {
		return nil, fmt.Errorf("no outbounds provided for testing")
	}

	// Convert outbounds to ProxyInfo and DialerFunc
	proxies := make([]testers.ProxyInfo, 0, len(outbounds))
	dialers := make([]testers.DialerFunc, 0, len(outbounds))

	for _, outbound := range outbounds {
		// Convert to ProxyInfo
		proxyInfo := testers.ProxyInfo{
			Tag:  outbound.Tag(),
			Type: outbound.Type(),
		}
		proxies = append(proxies, proxyInfo)

		// Create DialerFunc
		dialerFunc := func(outbound core.Outbound) testers.DialerFunc {
			return func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Parse the address into a Socksaddr
				socksAddr := metadata.ParseSocksaddr(addr)
				return outbound.DialContext(ctx, network, socksAddr)
			}
		}(outbound) // Capture outbound in closure
		dialers = append(dialers, dialerFunc)
	}

	// Determine effective concurrency
	effectiveConcurrency := concurrency
	if effectiveConcurrency <= 0 {
		effectiveConcurrency = len(outbounds) // Unlimited concurrency
	}

	// Create result channel
	results := make([]T, 0, len(outbounds))

	// Process outbounds in batches based on concurrency limit
	for i := 0; i < len(outbounds); i += effectiveConcurrency {
		// Determine batch size
		end := i + effectiveConcurrency
		if end > len(outbounds) {
			end = len(outbounds)
		}

		// Create batch slices
		batchProxies := proxies[i:end]
		batchDialers := dialers[i:end]

		// Create test for this batch
		test, err := createTest(ctx, batchProxies, batchDialers)
		if err != nil {
			return nil, fmt.Errorf("failed to create test for batch: %w", err)
		}

		// Create batch result channel
		batchResultChan := make(chan T, len(batchProxies))

		// Create progress channel if callback is provided
		var progressChan chan T
		var progressDone chan struct{}
		if progressCallback != nil {
			progressChan = make(chan T, len(batchProxies))
			progressDone = make(chan struct{})

			// Start goroutine to handle progress callbacks
			go func() {
				defer close(progressDone)
				for result := range progressChan {
					progressCallback(result)
				}
			}()
		}

		// Run test with progress callback and get wait function
		var wait func()
		if progressChan != nil {
			wait = test.Run(batchResultChan, progressChan)
		} else {
			wait = test.Run(batchResultChan)
		}

		// Collect batch results
		for range len(batchProxies) {
			result := <-batchResultChan
			results = append(results, result)
		}

		// Wait for all goroutines to complete before closing channels
		wait()

		// Close batch channels
		close(batchResultChan)

		// If progress channel exists, close it after all results are collected
		if progressChan != nil {
			close(progressChan)
			<-progressDone // Wait for progress handler to finish
		}
	}

	return results, nil
}

// RunLatencyTestRound executes a single round of latency tests on the provided outbounds.
// This low-level method allows custom logic between rounds (like in main.go).
// It converts outbounds to ProxyInfo and DialerFunc, creates a LatencyTest,
// runs the test with the progress callback, and collects results.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - outbounds: Slice of outbounds to test
//   - settings: Latency test settings (timeout, test URL)
//   - concurrency: Maximum number of concurrent tests (0 or negative for unlimited)
//   - progressCallback: Optional callback for real-time progress updates
//
// Returns:
//   - []testers.LatencyTestResult: Results from the latency test
//   - error: Error if test creation or execution fails
//
// Example:
//
//	outbounds := runner.GetOutbounds()
//	settings := testers.NewLatencyTestSettings()
//	settings.Timeout = 10 * time.Second
//	results, err := runner.RunLatencyTestRound(ctx, outbounds, settings, 10, func(r testers.LatencyTestResult) {
//	    fmt.Printf("Tested %s: %dms\n", r.Tag, r.Delay)
//	})
func (tr *TestRunner) RunLatencyTestRound(
	ctx context.Context,
	outbounds []core.Outbound,
	settings testers.LatencyTestSettings,
	concurrency int,
	progressCallback func(testers.LatencyTestResult),
) ([]testers.LatencyTestResult, error) {
	return runTestRound(ctx, outbounds, concurrency, progressCallback,
		func(ctx context.Context, proxies []testers.ProxyInfo, dialers []testers.DialerFunc) (testRunner[testers.LatencyTestResult], error) {
			return testers.NewLatencyTest(ctx, settings, proxies, dialers, nil)
		})
}

// RunSpeedTestRound executes speed tests on the provided outbounds.
// This low-level method allows custom logic for speed testing.
// It converts outbounds to ProxyInfo and DialerFunc, creates a SpeedTest,
// runs the test with the progress callback, and collects results.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - outbounds: Slice of outbounds to test
//   - settings: Speed test settings (timeout, target bytes, mode)
//   - concurrency: Maximum number of concurrent tests (0 or negative for unlimited)
//   - progressCallback: Optional callback for real-time progress updates
//
// Returns:
//   - []testers.SpeedTestResult: Results from the speed test
//   - error: Error if test creation or execution fails
//
// Example:
//
//	outbounds := runner.GetOutbounds()
//	settings := testers.NewDownloadTestSettings()
//	settings.Timeout = 20 * time.Second
//	settings.TargetBytes = 10 * 1024 * 1024
//	results, err := runner.RunSpeedTestRound(ctx, outbounds, settings, 10, func(r testers.SpeedTestResult) {
//	    fmt.Printf("Tested %s: %.2f MB/s\n", r.Tag, r.Speed/1024/1024)
//	})
func (tr *TestRunner) RunSpeedTestRound(
	ctx context.Context,
	outbounds []core.Outbound,
	settings testers.SpeedTestSettings,
	concurrency int,
	progressCallback func(testers.SpeedTestResult),
) ([]testers.SpeedTestResult, error) {
	return runTestRound(ctx, outbounds, concurrency, progressCallback,
		func(ctx context.Context, proxies []testers.ProxyInfo, dialers []testers.DialerFunc) (testRunner[testers.SpeedTestResult], error) {
			return testers.NewSpeedTest(ctx, settings, proxies, dialers, nil)
		})
}

// testConfig is a generic interface for test configuration
type testConfig interface {
	getBaseConfig() *BaseTestRunnerConfig
}

func (c *LatencyTestRunnerConfig) getBaseConfig() *BaseTestRunnerConfig {
	return &c.BaseTestRunnerConfig
}

func (c *SpeedTestRunnerConfig) getBaseConfig() *BaseTestRunnerConfig {
	return &c.BaseTestRunnerConfig
}

// runTests is a generic helper that executes tests with automatic lifecycle management.
// It handles core creation, startup, multi-round testing with filtering, and cleanup.
//
// Type Parameters:
//   - TResult: The result type for individual test results (e.g., LatencyTestResult, SpeedTestResult)
//   - TConfig: The configuration type that implements testConfig interface
//   - TSettings: The settings type passed to the test runner (e.g., LatencyTestSettings, SpeedTestSettings)
//
// Parameters:
//   - tr: The TestRunner instance that manages the core lifecycle
//   - ctx: Context for cancellation and timeout control
//   - profiles: Slice of proxy profiles to test
//   - config: Test configuration containing timeout, rounds, callbacks, etc.
//   - createSettings: Function that converts config to test-specific settings
//   - runRound: Function that executes a single test round (e.g., RunLatencyTestRound, RunSpeedTestRound)
//   - isSuccess: Function that determines if a test result is successful
//   - getTag: Function that extracts the tag/identifier from a test result
//   - aggregate: Function that aggregates results into the final result type
//   - emptyResults: Function that creates an empty result set when no outbounds exist
//
// Returns:
//   - any: Aggregated test results (caller must type assert to specific result type)
//   - error: Error if core creation, startup, or testing fails
//
// The function follows this workflow:
//  1. Sets unique tags on profiles
//  2. Creates and starts the core instance
//  3. Executes multiple test rounds with optional filtering
//  4. Handles context cancellation gracefully
//  5. Cleans up resources automatically via defer
func runTests[TResult any, TConfig testConfig, TSettings any](
	tr *TestRunner,
	ctx context.Context,
	profiles []parsers.ProxyProfile,
	config TConfig,
	createSettings func(TConfig) TSettings,
	runRound func(context.Context, []core.Outbound, TSettings, int, func(TResult)) ([]TResult, error),
	isSuccess func(TResult) bool,
	getTag func(TResult) string,
	aggregate func([]TResult, map[string]int, bool) any,
	emptyResults func(map[string]int) any,
) (any, error) {
	baseConfig := config.getBaseConfig()

	// Set unique tags
	for i := range profiles {
		profiles[i].Config.Tag = fmt.Sprintf("outbound-%d", i)
	}

	// Create core with provided profiles
	validationErrors, err := tr.CreateCore(ctx, profiles)
	if err != nil {
		return nil, fmt.Errorf("failed to create core: %w", err)
	}

	// Call CoreCreatedCallback if provided
	if baseConfig.CoreCreatedCallback != nil {
		baseConfig.CoreCreatedCallback(validationErrors)
	}

	// Use defer to ensure cleanup on return (handles success, failure, and cancellation)
	defer func() {
		if tr.config.AutoCleanup {
			if stopErr := tr.StopCore(); stopErr != nil {
				// Log the error but don't override the return error
				_ = stopErr
			}
		}
	}()

	// Start core instance
	if err := tr.StartCore(); err != nil {
		return nil, fmt.Errorf("failed to start core: %w", err)
	}

	// Check if context is already cancelled before starting tests
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Get outbounds from the core instance
	outbounds := tr.GetOutbounds()
	if len(outbounds) == 0 {
		return emptyResults(validationErrors), nil
	}

	// Create test settings from config
	settings := createSettings(config)

	// Run the configured number of rounds
	var finalResults []TResult
	currentOutbounds := outbounds

	for round := 0; round < baseConfig.Rounds; round++ {
		// Check for context cancellation before each round
		select {
		case <-ctx.Done():
			// Return partial results if cancelled mid-execution
			return aggregate(finalResults, validationErrors, baseConfig.SortResults), ctx.Err()
		default:
		}

		// Call RoundStartedCallback if provided
		if baseConfig.RoundStartedCallback != nil {
			baseConfig.RoundStartedCallback(round, len(currentOutbounds))
		}

		// Run a single round of tests
		var progressCallback func(TResult)
		if baseConfig.ProgressCallback != nil {
			progressCallback, _ = baseConfig.ProgressCallback.(func(TResult))
		}
		roundResults, err := runRound(ctx, currentOutbounds, settings, baseConfig.Concurrency, progressCallback)
		if err != nil {
			return nil, fmt.Errorf("failed to run test round %d: %w", round+1, err)
		}

		// Store results from this round
		finalResults = roundResults

		// Call RoundEndedCallback if provided
		if baseConfig.RoundEndedCallback != nil {
			baseConfig.RoundEndedCallback(round)
		}

		// If this is not the last round and filtering is enabled, filter for next round
		if round < baseConfig.Rounds-1 && baseConfig.FilterFailed {
			// Filter successful results for next round
			successfulTags := make(map[string]bool)
			for _, result := range roundResults {
				if isSuccess(result) {
					successfulTags[getTag(result)] = true
				}
			}

			// If no proxies passed, stop testing
			if len(successfulTags) == 0 {
				break
			}

			// Rebuild outbound list from successful results
			nextOutbounds := make([]core.Outbound, 0, len(successfulTags))
			for _, outbound := range currentOutbounds {
				if successfulTags[outbound.Tag()] {
					nextOutbounds = append(nextOutbounds, outbound)
				}
			}
			currentOutbounds = nextOutbounds
		}
	}

	// Aggregate and return results
	return aggregate(finalResults, validationErrors, baseConfig.SortResults), nil
}

// RunLatencyTests executes latency tests with automatic lifecycle management.
// This is the high-level API that handles core creation, startup, testing,
// and cleanup automatically. It's the simplest way to run latency tests.
//
// The method:
//  1. Creates a core instance with the provided profiles
//  2. Starts the core instance
//  3. Runs latency tests (potentially multiple rounds with filtering)
//  4. Cleans up resources automatically (via defer)
//  5. Handles context cancellation gracefully
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - profiles: Slice of proxy profiles to test
//   - config: Configuration for latency test execution
//
// Returns:
//   - *LatencyTestResults: Aggregated results from all test rounds
//   - error: Error if core creation, startup, or testing fails
//
// Example:
//
//	runner, _ := NewTestRunner(TestRunnerConfig{CoreType: SingBoxCore})
//	defer runner.Close()
//
//	config := LatencyTestConfig{
//	    Timeout: 10 * time.Second,
//	    Rounds: 3,
//	    FilterFailed: true,
//	    SortResults: true,
//	    ProgressCallback: func(r testers.LatencyTestResult) {
//	        fmt.Printf("Tested %s: %dms\n", r.Tag, r.Delay)
//	    },
//	}
//
//	results, err := runner.RunLatencyTests(ctx, profiles, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Successful tests: %d/%d\n", results.SuccessCount, len(profiles))
func (tr *TestRunner) RunLatencyTests(
	ctx context.Context,
	profiles []parsers.ProxyProfile,
	config LatencyTestRunnerConfig,
) (*LatencyTestResults, error) {
	result, err := runTests(
		tr, ctx, profiles, &config,
		// createSettings
		func(c *LatencyTestRunnerConfig) testers.LatencyTestSettings {
			testURL := c.TestURL
			if testURL == "" {
				testURL = testers.Google204
			}
			return testers.LatencyTestSettings{
				TestURL: testURL,
				Timeout: c.Timeout,
			}
		},
		// runRound
		tr.RunLatencyTestRound,
		// isSuccess
		func(r testers.LatencyTestResult) bool {
			return r.Error == nil && r.Delay > 0
		},
		// getTag
		func(r testers.LatencyTestResult) string {
			return r.Tag
		},
		// aggregate
		func(results []testers.LatencyTestResult, validationErrors map[string]int, sortResults bool) any {
			return aggregateLatencyResults(results, validationErrors, sortResults)
		},
		// emptyResults
		func(validationErrors map[string]int) any {
			return &LatencyTestResults{
				BaseTestResults: BaseTestResults{
					SuccessCount:     0,
					FailureCount:     0,
					ValidationErrors: validationErrors,
				},
				Results: []testers.LatencyTestResult{},
			}
		},
	)
	if err != nil {
		return nil, err
	}
	return result.(*LatencyTestResults), nil
}

// RunSpeedTests executes speed tests with automatic lifecycle management.
// This is the high-level API that handles core creation, startup, testing,
// and cleanup automatically. It's the simplest way to run speed tests.
//
// The method:
//  1. Creates a core instance with the provided profiles
//  2. Starts the core instance
//  3. Runs speed tests on all proxies
//  4. Cleans up resources automatically (via defer)
//  5. Handles context cancellation gracefully
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - profiles: Slice of proxy profiles to test
//   - config: Configuration for speed test execution
//
// Returns:
//   - *SpeedTestResults: Aggregated results from the speed test
//   - error: Error if core creation, startup, or testing fails
//
// Example:
//
//	runner, _ := NewTestRunner(TestRunnerConfig{CoreType: SingBoxCore})
//	defer runner.Close()
//
//	config := SpeedTestConfig{
//	    Timeout: 20 * time.Second,
//	    TargetBytes: 10 * 1024 * 1024,
//	    Direction: Download,
//	    ProgressCallback: func(r testers.SpeedTestResult) {
//	        fmt.Printf("Tested %s: %.2f MB/s\n", r.Tag, r.Speed/1024/1024)
//	    },
//	}
//
//	results, err := runner.RunSpeedTests(ctx, profiles, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Successful tests: %d/%d\n", results.SuccessCount, len(profiles))
func (tr *TestRunner) RunSpeedTests(
	ctx context.Context,
	profiles []parsers.ProxyProfile,
	config SpeedTestRunnerConfig,
) (*SpeedTestResults, error) {
	result, err := runTests(
		tr, ctx, profiles, &config,
		// createSettings
		func(c *SpeedTestRunnerConfig) testers.SpeedTestSettings {
			return testers.SpeedTestSettings{
				Mode:        c.Mode,
				Provider:    c.Provider,
				Timeout:     c.Timeout,
				TargetBytes: c.TargetBytes,
			}
		},
		// runRound
		tr.RunSpeedTestRound,
		// isSuccess
		func(r testers.SpeedTestResult) bool {
			return r.Error == nil && r.Speed > 0
		},
		// getTag
		func(r testers.SpeedTestResult) string {
			return r.Tag
		},
		// aggregate
		func(results []testers.SpeedTestResult, validationErrors map[string]int, sortResults bool) any {
			return aggregateSpeedResults(results, validationErrors, sortResults)
		},
		// emptyResults
		func(validationErrors map[string]int) any {
			return &SpeedTestResults{
				BaseTestResults: BaseTestResults{
					SuccessCount:     0,
					FailureCount:     len(validationErrors),
					ValidationErrors: validationErrors,
				},
				Results: []testers.SpeedTestResult{},
			}
		},
	)
	if err != nil {
		return nil, err
	}
	return result.(*SpeedTestResults), nil
}

// sortTestResults is a generic helper that sorts test results based on a comparison function.
// It uses bubble sort to place successful results before failed ones, and sorts successful
// results according to the provided comparison function.
func sortTestResults[T any](results []T, isSuccess func(T) bool, shouldSwap func(T, T) bool) {
	if len(results) == 0 {
		return
	}

	for i := 0; i < len(results)-1; i++ {
		for j := 0; j < len(results)-i-1; j++ {
			result1 := results[j]
			result2 := results[j+1]

			success1 := isSuccess(result1)
			success2 := isSuccess(result2)

			// Both successful: use custom comparison
			if success1 && success2 {
				if shouldSwap(result1, result2) {
					results[j], results[j+1] = results[j+1], results[j]
				}
			} else if !success1 && success2 {
				// result1 failed, result2 succeeded: swap to put successful first
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}

// aggregateSpeedResults aggregates speed test results and optionally sorts them.
// This helper function is used by RunSpeedTests to prepare the final results.
func aggregateSpeedResults(results []testers.SpeedTestResult, validationErrors map[string]int, sortResults bool) *SpeedTestResults {
	successCount := 0
	failureCount := 0

	for _, result := range results {
		if result.Error == nil && result.Speed > 0 {
			successCount++
		} else {
			failureCount++
		}
	}

	// Sort results by speed if configured (descending order - fastest first)
	if sortResults {
		sortTestResults(results,
			func(r testers.SpeedTestResult) bool { return r.Speed > 0 },
			func(r1, r2 testers.SpeedTestResult) bool { return r1.Speed < r2.Speed })
	}

	return &SpeedTestResults{
		BaseTestResults: BaseTestResults{
			SuccessCount:     successCount,
			FailureCount:     failureCount,
			ValidationErrors: validationErrors,
		},
		Results: results,
	}
}

// aggregateLatencyResults aggregates test results and optionally sorts them.
// This helper function is used by RunLatencyTests to prepare the final results.
func aggregateLatencyResults(results []testers.LatencyTestResult, validationErrors map[string]int, sortResults bool) *LatencyTestResults {
	successCount := 0
	failureCount := 0

	for _, result := range results {
		if result.Error == nil && result.Delay > 0 {
			successCount++
		} else {
			failureCount++
		}
	}

	// Sort results by latency if configured (ascending order - fastest first)
	if sortResults {
		sortTestResults(results,
			func(r testers.LatencyTestResult) bool { return r.Delay > 0 },
			func(r1, r2 testers.LatencyTestResult) bool { return r1.Delay > r2.Delay })
	}

	return &LatencyTestResults{
		BaseTestResults: BaseTestResults{
			SuccessCount:     successCount,
			FailureCount:     failureCount,
			ValidationErrors: validationErrors,
		},
		Results: results,
	}
}
