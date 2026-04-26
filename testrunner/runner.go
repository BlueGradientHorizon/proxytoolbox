package testrunner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/pkg/ipcprotocol"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
)

// TestRunner manages tester process lifecycle and test execution via IPC.
type TestRunner struct {
	testerPath  string
	testerDebug bool
	autoCleanup bool
}

// NewTestRunner creates a new test runner with the specified configuration.
func NewTestRunner(config TestRunnerConfig) (*TestRunner, error) {
	if config.TesterPath == "" {
		return nil, fmt.Errorf("tester path is required")
	}
	return &TestRunner{testerPath: config.TesterPath, testerDebug: config.TesterDebug}, nil
}

// Close cleans up resources used by the test runner.
func (tr *TestRunner) Close() error { return nil }

// RunLatencyTests executes latency tests with automatic lifecycle management.
func (tr *TestRunner) RunLatencyTests(ctx context.Context, profiles []parsers.ProxyProfile, config LatencyTestRunnerConfig) (*LatencyTestResults, error) {
	base := config.getBaseConfig()
	var progressCb func(testers.LatencyTestResult)
	if base.ProgressCallback != nil {
		progressCb, _ = base.ProgressCallback.(func(testers.LatencyTestResult))
	}

	res, err := runIPCTests(
		tr, ctx, profiles, &config,
		func(currentProfiles []parsers.ProxyProfile, c *LatencyTestRunnerConfig) ipcprotocol.Request {
			testURL := c.TestURL
			if testURL == "" {
				testURL = testers.Google204
			}
			tags := make([]string, len(currentProfiles))
			for i, p := range currentProfiles {
				tags[i] = p.Config.Tag
			}
			return ipcprotocol.Request{
				Type:     ipcprotocol.RequestTypeTest,
				TestType: ipcprotocol.LatencyTest,
				Tags:     tags,
				Settings: mustMarshal(ipcprotocol.LatencySettings{
					TimeoutMs:   int(c.Timeout.Milliseconds()),
					TestURL:     testURL,
					Concurrency: base.Concurrency,
				}),
			}
		},
		func(r ipcprotocol.Response) testers.LatencyTestResult {
			var err error
			if r.Error != "" {
				err = fmt.Errorf("%s", r.Error)
			}
			return testers.LatencyTestResult{Tag: r.Tag, Delay: r.LatencyMs, Error: err}
		},
		progressCb,
		func(r testers.LatencyTestResult) bool { return r.Error == nil && r.Delay > 0 },
		func(r testers.LatencyTestResult) string { return r.Tag },
		func(rs []testers.LatencyTestResult, ve map[string]int, sort bool) any {
			return aggregateLatencyResults(rs, ve, sort)
		},
	)
	if err != nil {
		return nil, err
	}
	return res.(*LatencyTestResults), nil
}

// RunSpeedTests executes speed tests with automatic lifecycle management.
func (tr *TestRunner) RunSpeedTests(ctx context.Context, profiles []parsers.ProxyProfile, config SpeedTestRunnerConfig) (*SpeedTestResults, error) {
	base := config.getBaseConfig()
	var progressCb func(testers.SpeedTestResult)
	if base.ProgressCallback != nil {
		progressCb, _ = base.ProgressCallback.(func(testers.SpeedTestResult))
	}

	mode := "download"
	if config.Mode == testers.Upload {
		mode = "upload"
	}

	res, err := runIPCTests(
		tr, ctx, profiles, &config,
		func(currentProfiles []parsers.ProxyProfile, c *SpeedTestRunnerConfig) ipcprotocol.Request {
			tags := make([]string, len(currentProfiles))
			for i, p := range currentProfiles {
				tags[i] = p.Config.Tag
			}
			return ipcprotocol.Request{
				Type:     ipcprotocol.RequestTypeTest,
				TestType: ipcprotocol.SpeedTest,
				Tags:     tags,
				Settings: mustMarshal(ipcprotocol.SpeedSettings{
					Mode:        mode,
					TimeoutMs:   int(c.Timeout.Milliseconds()),
					TargetBytes: c.TargetBytes,
					Concurrency: base.Concurrency,
				}),
			}
		},
		func(r ipcprotocol.Response) testers.SpeedTestResult {
			var err error
			if r.Error != "" {
				err = fmt.Errorf("%s", r.Error)
			}
			return testers.SpeedTestResult{Tag: r.Tag, Speed: r.Speed, Error: err}
		},
		progressCb,
		func(r testers.SpeedTestResult) bool { return r.Error == nil && r.Speed > 0 },
		func(r testers.SpeedTestResult) string { return r.Tag },
		func(rs []testers.SpeedTestResult, ve map[string]int, sort bool) any {
			return aggregateSpeedResults(rs, ve, sort)
		},
	)
	if err != nil {
		return nil, err
	}
	return res.(*SpeedTestResults), nil
}

// ---------------------------------------------------------------------
// Generic IPC test runner
// ---------------------------------------------------------------------

func runIPCTests[TResult any, TConfig testConfig](
	tr *TestRunner,
	ctx context.Context,
	profiles []parsers.ProxyProfile,
	config TConfig,
	buildTestReq func([]parsers.ProxyProfile, TConfig) ipcprotocol.Request,
	convert func(ipcprotocol.Response) TResult,
	onProgress func(TResult),
	isSuccess func(TResult) bool,
	getTag func(TResult) string,
	aggregate func([]TResult, map[string]int, bool) any,
) (any, error) {
	base := config.getBaseConfig()

	for i := range profiles {
		profiles[i].Config.Tag = fmt.Sprintf("outbound-%d", i)
	}

	proc := &TesterProcess{path: tr.testerPath, debug: tr.testerDebug}
	if err := proc.Start(); err != nil {
		return nil, fmt.Errorf("start tester: %w", err)
	}
	defer proc.Close()

	// --- Validation phase ---
	var validationErrors map[string]int

	validateReq := ipcprotocol.Request{
		Type:    ipcprotocol.RequestTypeValidate,
		Configs: toRawConfigs(extractConfigs(profiles)),
	}

	err := proc.SendRequest(ctx, validateReq, func(r ipcprotocol.Response) {
		if r.Type == ipcprotocol.ResponseTypeValidation {
			validationErrors = r.ValidationErrors
			if base.CoreCreatedCallback != nil {
				base.CoreCreatedCallback(validationErrors)
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	currentProfiles := profiles

	// --- Test phase ---
	var final []TResult

	for round := 0; round < base.Rounds; round++ {
		select {
		case <-ctx.Done():
			return aggregate(final, validationErrors, base.SortResults), ctx.Err()
		default:
		}

		if base.RoundStartedCallback != nil {
			base.RoundStartedCallback(round, len(currentProfiles))
		}

		req := buildTestReq(currentProfiles, config)
		var roundResults []TResult

		err := proc.SendRequest(ctx, req, func(r ipcprotocol.Response) {
			switch r.Type {
			case ipcprotocol.ResponseTypeResult:
				res := convert(r)
				roundResults = append(roundResults, res)
				if onProgress != nil {
					onProgress(res)
				}
			}
		})

		if err != nil {
			return nil, fmt.Errorf("round %d: %w", round+1, err)
		}

		final = roundResults

		if base.RoundEndedCallback != nil {
			base.RoundEndedCallback(round)
		}

		if round < base.Rounds-1 && base.FilterFailed {
			good := make(map[string]bool)
			for _, r := range roundResults {
				if isSuccess(r) {
					good[getTag(r)] = true
				}
			}
			if len(good) == 0 {
				break
			}
			next := make([]parsers.ProxyProfile, 0, len(good))
			for _, p := range currentProfiles {
				if good[p.Config.Tag] {
					next = append(next, p)
				}
			}
			currentProfiles = next
		}
	}

	return aggregate(final, validationErrors, base.SortResults), nil
}

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

func extractConfigs(profiles []parsers.ProxyProfile) []*core.OutboundConfig {
	out := make([]*core.OutboundConfig, 0, len(profiles))
	for _, p := range profiles {
		if p.Config != nil {
			out = append(out, p.Config)
		}
	}
	return out
}

func toRawConfigs(configs []*core.OutboundConfig) []*ipcprotocol.RawConfig {
	out := make([]*ipcprotocol.RawConfig, 0, len(configs))
	for _, c := range configs {
		s, _ := json.Marshal(c.Settings)
		out = append(out, &ipcprotocol.RawConfig{
			Tag: c.Tag, Type: c.Type, Server: c.Server, Port: c.Port,
			Settings: s, TLS: c.TLS, Transport: c.Transport,
		})
	}
	return out
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}

type testConfig interface {
	getBaseConfig() *BaseTestRunnerConfig
}

func (c *LatencyTestRunnerConfig) getBaseConfig() *BaseTestRunnerConfig {
	return &c.BaseTestRunnerConfig
}

func (c *SpeedTestRunnerConfig) getBaseConfig() *BaseTestRunnerConfig {
	return &c.BaseTestRunnerConfig
}

func sortTestResults[T any](results []T, isSuccess func(T) bool, shouldSwap func(T, T) bool) {
	if len(results) == 0 {
		return
	}
	for i := 0; i < len(results)-1; i++ {
		for j := 0; j < len(results)-i-1; j++ {
			r1 := results[j]
			r2 := results[j+1]
			s1 := isSuccess(r1)
			s2 := isSuccess(r2)
			if s1 && s2 {
				if shouldSwap(r1, r2) {
					results[j], results[j+1] = results[j+1], results[j]
				}
			} else if !s1 && s2 {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}

func aggregateLatencyResults(results []testers.LatencyTestResult, validationErrors map[string]int, sortResults bool) *LatencyTestResults {
	successCount := 0
	failureCount := 0
	for _, r := range results {
		if r.Error == nil && r.Delay > 0 {
			successCount++
		} else {
			failureCount++
		}
	}
	if sortResults {
		sortTestResults(results,
			func(r testers.LatencyTestResult) bool { return r.Delay > 0 },
			func(r1, r2 testers.LatencyTestResult) bool { return r1.Delay > r2.Delay })
	}
	return &LatencyTestResults{
		BaseTestResults: BaseTestResults{
			SuccessCount: successCount, FailureCount: failureCount,
			ValidationErrors: validationErrors,
		},
		Results: results,
	}
}

func aggregateSpeedResults(results []testers.SpeedTestResult, validationErrors map[string]int, sortResults bool) *SpeedTestResults {
	successCount := 0
	failureCount := 0
	for _, r := range results {
		if r.Error == nil && r.Speed > 0 {
			successCount++
		} else {
			failureCount++
		}
	}
	if sortResults {
		sortTestResults(results,
			func(r testers.SpeedTestResult) bool { return r.Speed > 0 },
			func(r1, r2 testers.SpeedTestResult) bool { return r1.Speed < r2.Speed })
	}
	return &SpeedTestResults{
		BaseTestResults: BaseTestResults{
			SuccessCount: successCount, FailureCount: failureCount,
			ValidationErrors: validationErrors,
		},
		Results: results,
	}
}
