package testrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/pkg/ipcprotocol"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
)

// TestRunner manages tester process lifecycle and test execution via IPC.
type TestRunner struct {
	testerPath  string
	testerDebug bool
	proc        *TesterProcess
	mu          sync.Mutex
	testMu      sync.Mutex
}

// NewTestRunner creates a new test runner with the specified configuration.
func NewTestRunner(testerSettings TesterSettings) (*TestRunner, error) {
	if testerSettings.TesterPath == "" {
		return nil, fmt.Errorf("tester path is required")
	}
	return &TestRunner{testerPath: testerSettings.TesterPath, testerDebug: testerSettings.TesterDebug}, nil
}

func (tr *TestRunner) ensureProc() (*TesterProcess, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.proc != nil {
		return tr.proc, nil
	}
	tr.proc = &TesterProcess{path: tr.testerPath, debug: tr.testerDebug}
	if err := tr.proc.Start(); err != nil {
		tr.proc = nil
		return nil, fmt.Errorf("start tester: %w", err)
	}
	return tr.proc, nil
}

func (tr *TestRunner) invalidateProc() {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.proc != nil {
		tr.proc.Close()
		tr.proc = nil
	}
}

// Close cleans up resources used by the test runner.
func (tr *TestRunner) Close() error {
	tr.invalidateProc()
	return nil
}

// RunLatencyTests executes latency tests with automatic lifecycle management.
func (tr *TestRunner) RunLatencyTests(ctx context.Context, configs []parsers.ProxyConfig, ltRunnerSettings LatencyTestRunnerSettings) (*LatencyTestResults, error) {
	base := ltRunnerSettings.getBaseSettings()
	var progressCb func(testers.LatencyTestResult)
	if base.ProgressCallback != nil {
		progressCb, _ = base.ProgressCallback.(func(testers.LatencyTestResult))
	}

	res, err := runIPCTests(
		tr, ctx, configs, &ltRunnerSettings,
		func(currentConfigs []parsers.ProxyConfig, c *LatencyTestRunnerSettings) ipcprotocol.Request {
			testURL := c.TestURL
			if testURL == "" {
				testURL = testers.Google204
			}
			tags := make([]string, len(currentConfigs))
			for i, p := range currentConfigs {
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
		func(rs []testers.LatencyTestResult, ve []ipcprotocol.ValidationError, sort bool) any {
			return aggregateLatencyResults(rs, ve, sort)
		},
	)
	if err != nil {
		return nil, err
	}
	return res.(*LatencyTestResults), nil
}

// RunSpeedTests executes speed tests with automatic lifecycle management.
func (tr *TestRunner) RunSpeedTests(ctx context.Context, configs []parsers.ProxyConfig, stRunnerSettings SpeedTestRunnerSettings) (*SpeedTestResults, error) {
	base := stRunnerSettings.getBaseSettings()
	var progressCb func(testers.SpeedTestResult)
	if base.ProgressCallback != nil {
		progressCb, _ = base.ProgressCallback.(func(testers.SpeedTestResult))
	}

	mode := "download"
	if stRunnerSettings.Mode == testers.Upload {
		mode = "upload"
	}

	res, err := runIPCTests(
		tr, ctx, configs, &stRunnerSettings,
		func(currentConfigs []parsers.ProxyConfig, c *SpeedTestRunnerSettings) ipcprotocol.Request {
			tags := make([]string, len(currentConfigs))
			for i, p := range currentConfigs {
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
		func(rs []testers.SpeedTestResult, ve []ipcprotocol.ValidationError, sort bool) any {
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

func runIPCTests[TResult any, TSettings testSettings](
	tr *TestRunner,
	ctx context.Context,
	configs []parsers.ProxyConfig,
	settings TSettings,
	buildTestReq func([]parsers.ProxyConfig, TSettings) ipcprotocol.Request,
	convert func(ipcprotocol.Response) TResult,
	onProgress func(TResult),
	isSuccess func(TResult) bool,
	getTag func(TResult) string,
	aggregate func([]TResult, []ipcprotocol.ValidationError, bool) any,
) (any, error) {
	base := settings.getBaseSettings()

	for i := range configs {
		configs[i].Config.Tag = fmt.Sprintf("outbound-%d", i)
	}

	tr.testMu.Lock()
	defer tr.testMu.Unlock()

	proc, err := tr.ensureProc()
	if err != nil {
		return nil, err
	}

	// --- Validation phase ---
	var validationErrors []ipcprotocol.ValidationError

	validateReq := ipcprotocol.Request{
		Type:    ipcprotocol.RequestTypeValidate,
		Configs: toRawConfigs(extractConfigs(configs)),
	}

	err = proc.SendRequest(ctx, validateReq, func(r ipcprotocol.Response) {
		if r.Type == ipcprotocol.ResponseTypeValidation {
			validationErrors = r.ValidationErrors
			if base.CoreCreatedCallback != nil {
				base.CoreCreatedCallback(validationErrors)
			}
		}
	})
	if err != nil {
		if err != ErrTesterBusy {
			tr.invalidateProc()
		}
		return nil, fmt.Errorf("validation: %w", err)
	}

	currentConfigs := configs

	// --- Test phase ---
	var final []TResult

	for round := 0; round < base.Rounds; round++ {
		select {
		case <-ctx.Done():
			return aggregate(final, validationErrors, base.SortResults), ctx.Err()
		default:
		}

		if base.RoundStartedCallback != nil {
			base.RoundStartedCallback(round, len(currentConfigs))
		}

		req := buildTestReq(currentConfigs, settings)
		var roundResults []TResult

		err = proc.SendRequest(ctx, req, func(r ipcprotocol.Response) {
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
			if err != ErrTesterBusy {
				tr.invalidateProc()
			}
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
			next := make([]parsers.ProxyConfig, 0, len(good))
			for _, p := range currentConfigs {
				if good[p.Config.Tag] {
					next = append(next, p)
				}
			}
			currentConfigs = next
		}
	}

	return aggregate(final, validationErrors, base.SortResults), nil
}

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

func extractConfigs(configs []parsers.ProxyConfig) []*core.OutboundConfig {
	out := make([]*core.OutboundConfig, 0, len(configs))
	for _, p := range configs {
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

type testSettings interface {
	getBaseSettings() *BaseTestRunnerSettings
}

func (c *LatencyTestRunnerSettings) getBaseSettings() *BaseTestRunnerSettings {
	return &c.BaseTestRunnerSettings
}

func (c *SpeedTestRunnerSettings) getBaseSettings() *BaseTestRunnerSettings {
	return &c.BaseTestRunnerSettings
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

func aggregateLatencyResults(results []testers.LatencyTestResult, validationErrors []ipcprotocol.ValidationError, sortResults bool) *LatencyTestResults {
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

func aggregateSpeedResults(results []testers.SpeedTestResult, validationErrors []ipcprotocol.ValidationError, sortResults bool) *SpeedTestResults {
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
