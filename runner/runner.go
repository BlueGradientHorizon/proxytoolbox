package runner

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"sync"

	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/presets"
	"github.com/bluegradienthorizon/proxytoolbox/worker"
)

// TestRunner manages worker process lifecycle and test execution via IPC.
type TestRunner struct {
	workerPath  string
	workerDebug bool
	proc        *WorkerProcess
	mu          sync.Mutex
	testMu      sync.Mutex
}

// NewTestRunner creates a new test runner with the specified configuration.
func NewTestRunner(runnerSettings RunnerSettings) (*TestRunner, error) {
	if runnerSettings.WorkerPath == "" {
		return nil, fmt.Errorf("worker path is required")
	}
	return &TestRunner{workerPath: runnerSettings.WorkerPath, workerDebug: runnerSettings.WorkerDebug}, nil
}

func (tr *TestRunner) ensureProc() (*WorkerProcess, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.proc != nil {
		return tr.proc, nil
	}
	tr.proc = &WorkerProcess{path: tr.workerPath, debug: tr.workerDebug}
	if err := tr.proc.Start(); err != nil {
		tr.proc = nil
		return nil, fmt.Errorf("start worker: %w", err)
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
	var progressCb func(LatencyTestResult)
	if base.ProgressCallback != nil {
		progressCb, _ = base.ProgressCallback.(func(LatencyTestResult))
	}

	res, err := runIPCTests(
		tr, ctx, configs, &ltRunnerSettings,
		func(currentConfigs []parsers.ProxyConfig, c *LatencyTestRunnerSettings) worker.Request {
			testURL := c.TestURL
			if testURL == "" {
				testURL = presets.Google204
			}
			tags := make([]string, len(currentConfigs))
			for i, p := range currentConfigs {
				tags[i] = p.Config.Tag
			}
			return worker.Request{
				Type:     worker.RequestTypeTest,
				TestType: worker.TestTypeLatency,
				Tags:     tags,
				Settings: mustMarshal(worker.LatencySettings{
					TimeoutMs:   int(c.Timeout.Milliseconds()),
					TestURL:     testURL,
					Concurrency: base.Concurrency,
				}),
			}
		},
		func(r worker.Response) LatencyTestResult {
			var err error
			if r.Error != "" {
				err = fmt.Errorf("%s", r.Error)
			}
			return LatencyTestResult{Tag: r.Tag, Delay: r.LatencyMs, Error: err}
		},
		progressCb,
		func(r LatencyTestResult) bool { return r.Error == nil && r.Delay > 0 },
		func(r LatencyTestResult) string { return r.Tag },
		func(rs []LatencyTestResult, ve []ValidationError, sort bool) any {
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
	var progressCb func(SpeedTestResult)
	if base.ProgressCallback != nil {
		progressCb, _ = base.ProgressCallback.(func(SpeedTestResult))
	}

	mode := "download"
	if stRunnerSettings.Mode == SpeedTestModeUpload {
		mode = "upload"
	}

	res, err := runIPCTests(
		tr, ctx, configs, &stRunnerSettings,
		func(currentConfigs []parsers.ProxyConfig, c *SpeedTestRunnerSettings) worker.Request {
			tags := make([]string, len(currentConfigs))
			for i, p := range currentConfigs {
				tags[i] = p.Config.Tag
			}

			wMode := worker.SpeedTestModeDownload
			if c.Mode == SpeedTestModeUpload {
				wMode = worker.SpeedTestModeUpload
			}

			// 1. Generate the URL from provider
			testURL := c.Provider.GetURL(wMode, c.TargetBytes)

			// Determine HTTP method
			method := http.MethodGet
			if c.Mode == SpeedTestModeUpload {
				method = http.MethodPost
			}

			// 2. Create a temporary request and apply ModifyRequest logic
			req, _ := http.NewRequest(method, testURL, nil)
			if c.Provider.ModifyRequest != nil {
				c.Provider.ModifyRequest(req, wMode, c.TargetBytes)
			}

			// 3. Serialize the request to wire format (excluding body)
			rawReq, _ := httputil.DumpRequest(req, false)

			return worker.Request{
				Type:     worker.RequestTypeTest,
				TestType: worker.TestTypeSpeed,
				Tags:     tags,
				Settings: mustMarshal(worker.SpeedSettings{
					Mode:        mode,
					TimeoutMs:   int(c.Timeout.Milliseconds()),
					TargetBytes: c.TargetBytes,
					Concurrency: base.Concurrency,
					TestURL:     testURL,
					RawRequest:  rawReq,
				}),
			}
		},
		func(r worker.Response) SpeedTestResult {
			var err error
			if r.Error != "" {
				err = fmt.Errorf("%s", r.Error)
			}
			return SpeedTestResult{Tag: r.Tag, Speed: r.Speed, Error: err}
		},
		progressCb,
		func(r SpeedTestResult) bool { return r.Error == nil && r.Speed > 0 },
		func(r SpeedTestResult) string { return r.Tag },
		func(rs []SpeedTestResult, ve []ValidationError, sort bool) any {
			return aggregateSpeedResults(rs, ve, sort)
		},
	)
	if err != nil {
		return nil, err
	}
	return res.(*SpeedTestResults), nil
}

func runIPCTests[TResult any, TSettings testSettings](
	tr *TestRunner,
	ctx context.Context,
	configs []parsers.ProxyConfig,
	settings TSettings,
	buildTestReq func([]parsers.ProxyConfig, TSettings) worker.Request,
	convert func(worker.Response) TResult,
	onProgress func(TResult),
	isSuccess func(TResult) bool,
	getTag func(TResult) string,
	aggregate func([]TResult, []ValidationError, bool) any,
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
	var validationErrors []ValidationError

	validateReq := worker.Request{
		Type:    worker.RequestTypeValidate,
		Configs: toRawConfigs(extractConfigs(configs)),
	}

	err = proc.SendRequest(ctx, validateReq, func(r worker.Response) {
		if r.Type == worker.ResponseTypeValidation {
			validationErrors = make([]ValidationError, len(r.ValidationErrors))
			for j, ve := range r.ValidationErrors {
				validationErrors[j] = ValidationError{Tag: ve.Tag, Error: ve.Error}
			}
			if base.CoreCreatedCallback != nil {
				base.CoreCreatedCallback(validationErrors)
			}
		}
	})
	if err != nil {
		if err != ErrWorkerBusy {
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

		err = proc.SendRequest(ctx, req, func(r worker.Response) {
			switch r.Type {
			case worker.ResponseTypeResult:
				res := convert(r)
				roundResults = append(roundResults, res)
				if onProgress != nil {
					onProgress(res)
				}
			}
		})

		if err != nil {
			if err != ErrWorkerBusy {
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
