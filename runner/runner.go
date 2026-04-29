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

	itr := &ipcTestRunner[LatencyTestResult, *LatencyTestRunnerSettings]{
		tr:       tr,
		ctx:      ctx,
		configs:  configs,
		settings: &ltRunnerSettings,
		buildTestReq: func(currentConfigs []parsers.ProxyConfig, c *LatencyTestRunnerSettings) worker.Request {
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
		convert: func(r worker.Response) LatencyTestResult {
			var err error
			if r.Error != "" {
				err = fmt.Errorf("%s", r.Error)
			}
			return LatencyTestResult{Tag: r.Tag, Delay: r.LatencyMs, Error: err}
		},
		onProgress: progressCb,
		isSuccess:  func(r LatencyTestResult) bool { return r.Error == nil && r.Delay > 0 },
		getTag:     func(r LatencyTestResult) string { return r.Tag },
		aggregate: func(rs []LatencyTestResult, ve []ValidationError, sort bool) any {
			return aggregateLatencyResults(rs, ve, sort)
		},
	}
	res, err := itr.run()
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

	itr := &ipcTestRunner[SpeedTestResult, *SpeedTestRunnerSettings]{
		tr:       tr,
		ctx:      ctx,
		configs:  configs,
		settings: &stRunnerSettings,
		buildTestReq: func(currentConfigs[]parsers.ProxyConfig, c *SpeedTestRunnerSettings) worker.Request {
			tags := make([]string, len(currentConfigs))
			for i, p := range currentConfigs {
				tags[i] = p.Config.Tag
			}

			// 1. Generate the URL from provider
			testURL := c.Provider.GetURL(c.Mode, c.TargetBytes)

			// Determine HTTP method
			method := http.MethodGet
			if c.Mode == worker.SpeedTestModeUpload {
				method = http.MethodPost
			}

			// 2. Create a temporary request and apply ModifyRequest logic
			req, _ := http.NewRequest(method, testURL, nil)
			if c.Provider.ModifyRequest != nil {
				c.Provider.ModifyRequest(req, c.Mode, c.TargetBytes)
			}

			// 3. Serialize the request to wire format (excluding body)
			rawReq, _ := httputil.DumpRequest(req, false)

			return worker.Request{
				Type:     worker.RequestTypeTest,
				TestType: worker.TestTypeSpeed,
				Tags:     tags,
				Settings: mustMarshal(worker.SpeedSettings{
					Mode:        string(c.Mode),
					TimeoutMs:   int(c.Timeout.Milliseconds()),
					TargetBytes: c.TargetBytes,
					Concurrency: base.Concurrency,
					TestURL:     testURL,
					RawRequest:  rawReq,
				}),
			}
		},
		convert: func(r worker.Response) SpeedTestResult {
			var err error
			if r.Error != "" {
				err = fmt.Errorf("%s", r.Error)
			}
			return SpeedTestResult{Tag: r.Tag, Speed: r.Speed, Error: err}
		},
		onProgress: progressCb,
		isSuccess:  func(r SpeedTestResult) bool { return r.Error == nil && r.Speed > 0 },
		getTag:     func(r SpeedTestResult) string { return r.Tag },
		aggregate: func(rs []SpeedTestResult, ve []ValidationError, sort bool) any {
			return aggregateSpeedResults(rs, ve, sort)
		},
	}
	res, err := itr.run()
	if err != nil {
		return nil, err
	}
	return res.(*SpeedTestResults), nil
}

type ipcTestRunner[TResult any, TSettings testSettings] struct {
	tr           *TestRunner
	ctx          context.Context
	configs      []parsers.ProxyConfig
	settings     TSettings
	buildTestReq func([]parsers.ProxyConfig, TSettings) worker.Request
	convert      func(worker.Response) TResult
	onProgress   func(TResult)
	isSuccess    func(TResult) bool
	getTag       func(TResult) string
	aggregate    func([]TResult, []ValidationError, bool) any
}

func (itr *ipcTestRunner[TResult, TSettings]) run() (any, error) {
	base := itr.settings.getBaseSettings()

	for i := range itr.configs {
		itr.configs[i].Config.Tag = fmt.Sprintf("outbound-%d", i)
	}

	itr.tr.testMu.Lock()
	defer itr.tr.testMu.Unlock()

	proc, err := itr.tr.ensureProc()
	if err != nil {
		return nil, err
	}

	// --- Validation phase ---
	var validationErrors []ValidationError

	validateReq := worker.Request{
		Type:    worker.RequestTypeValidate,
		Configs: toRawConfigs(extractConfigs(itr.configs)),
	}

	err = proc.SendRequest(itr.ctx, validateReq, func(r worker.Response) {
		if r.Type == worker.ResponseTypeValidation {
			validationErrors = r.ValidationErrors
			if base.CoreCreatedCallback != nil {
				base.CoreCreatedCallback(validationErrors)
			}
		}
	})
	if err != nil {
		if err != ErrWorkerBusy {
			itr.tr.invalidateProc()
		}
		return nil, fmt.Errorf("validation: %w", err)
	}

	currentConfigs := itr.configs

	// --- Test phase ---
	var final []TResult

	for round := 0; round < base.Rounds; round++ {
		select {
		case <-itr.ctx.Done():
			return itr.aggregate(final, validationErrors, base.SortResults), itr.ctx.Err()
		default:
		}

		if base.RoundStartedCallback != nil {
			base.RoundStartedCallback(round, len(currentConfigs))
		}

		req := itr.buildTestReq(currentConfigs, itr.settings)
		var roundResults []TResult

		err = proc.SendRequest(itr.ctx, req, func(r worker.Response) {
			switch r.Type {
			case worker.ResponseTypeResult:
				res := itr.convert(r)
				roundResults = append(roundResults, res)
				if itr.onProgress != nil {
					itr.onProgress(res)
				}
			}
		})

		if err != nil {
			if err != ErrWorkerBusy {
				itr.tr.invalidateProc()
			}
			return nil, fmt.Errorf("round %d: %w", round+1, err)
		}

		final = roundResults

		if base.RoundEndedCallback != nil {
			base.RoundEndedCallback(round)
		}

		if round < base.Rounds-1 && base.FilterFailed {
			good := make(map[string]bool)
			for _, res := range roundResults {
				if itr.isSuccess(res) {
					good[itr.getTag(res)] = true
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

	return itr.aggregate(final, validationErrors, base.SortResults), nil
}
