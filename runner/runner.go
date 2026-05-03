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
	workerPath    string
	workerLogPath string
	proc          *WorkerProcess
	mu            sync.Mutex
	testMu        sync.Mutex
}

// NewTestRunner creates a new test runner with the specified configuration.
func NewTestRunner(runnerSettings RunnerSettings) (*TestRunner, error) {
	if runnerSettings.WorkerPath == "" {
		return nil, fmt.Errorf("worker path is required")
	}
	return &TestRunner{workerPath: runnerSettings.WorkerPath, workerLogPath: runnerSettings.WorkerLogPath}, nil
}

func (tr *TestRunner) ensureProc() (*WorkerProcess, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.proc != nil {
		return tr.proc, nil
	}
	tr.proc = &WorkerProcess{path: tr.workerPath, logPath: tr.workerLogPath}
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

func DefaultConfigTaggerFunc(existingTag string, index int) string {
	if existingTag != "" {
		return existingTag
	}
	return fmt.Sprintf("outbound-%d", index)
}

// Validate instantiates the core objects for the given configs and returns validation errors.
// It does not mutate the given list but creates a copy with generated tags and returns it.
func (tr *TestRunner) Validate(ctx context.Context, configs []parsers.ProxyConfig, taggerFunc func(string, int) string) ([]parsers.ProxyConfig, []ValidationError, error) {
	configsCopy := make([]parsers.ProxyConfig, len(configs))
	for i, c := range configs {
		configsCopy[i] = c
		if c.Config != nil {
			cfgCopy := *c.Config
			cfgCopy.Tag = taggerFunc(cfgCopy.Tag, i)
			configsCopy[i].Config = &cfgCopy
		}
	}

	tr.testMu.Lock()
	defer tr.testMu.Unlock()

	proc, err := tr.ensureProc()
	if err != nil {
		return nil, nil, err
	}

	var validationErrors []ValidationError

	validateReq := worker.Request{
		Type:    worker.RequestTypeValidate,
		Configs: toRawConfigs(extractConfigs(configsCopy)),
	}

	err = proc.SendRequest(ctx, validateReq, func(r worker.Response) {
		if r.Type == worker.ResponseTypeValidation {
			validationErrors = r.ValidationErrors
		}
	})
	if err != nil {
		if err != ErrWorkerBusy {
			tr.invalidateProc()
		}
		return nil, nil, fmt.Errorf("validation: %w", err)
	}

	return configsCopy, validationErrors, nil
}

// RunLatencyTests executes latency tests with automatic lifecycle management.
func (tr *TestRunner) RunLatencyTests(ctx context.Context, tags []string, ltRunnerSettings LatencyTestRunnerSettings) (*LatencyTestResults, error) {
	base := ltRunnerSettings.getBaseSettings()
	var progressCb func(LatencyTestResult)
	if base.ProgressCallback != nil {
		var ok bool
		progressCb, ok = base.ProgressCallback.(func(LatencyTestResult))
		if !ok {
			return nil, fmt.Errorf("invalid ProgressCallback type: expected func(LatencyTestResult)")
		}
	}

	itr := &ipcTestRunner[LatencyTestResult, *LatencyTestRunnerSettings]{
		tr:       tr,
		ctx:      ctx,
		tags:     tags,
		settings: &ltRunnerSettings,
		buildTestReq: func(currentTags []string, c *LatencyTestRunnerSettings) worker.Request {
			testURL := c.TestURL
			if testURL == "" {
				testURL = presets.Google204
			}

			req, _ := http.NewRequest(http.MethodHead, testURL, nil)
			if c.ModifyRequest != nil {
				c.ModifyRequest(req)
			}
			rawReq, _ := httputil.DumpRequest(req, false)

			s, err := mustMarshal(worker.LatencyTestSettings{
				TestURL:     testURL,
				RawRequest:  rawReq,
				Timeout:     c.Timeout,
				Concurrency: base.Concurrency,
			})
			if err != nil {
				panic(fmt.Sprintf("marshal latency settings: %v", err))
			}

			return worker.Request{
				Type:     worker.RequestTypeTest,
				TestType: worker.TestTypeLatency,
				Tags:     currentTags,
				Settings: s,
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
		isSuccess:  func(r LatencyTestResult) bool { return r.Error == nil },
		getTag:     func(r LatencyTestResult) string { return r.Tag },
		aggregate: func(rs []LatencyTestResult, sort bool) any {
			return aggregateLatencyResults(rs, sort)
		},
	}
	res, err := itr.run()
	if err != nil {
		return nil, err
	}
	return res.(*LatencyTestResults), nil
}

// RunSpeedTests executes speed tests with automatic lifecycle management.
func (tr *TestRunner) RunSpeedTests(ctx context.Context, tags []string, stRunnerSettings SpeedTestRunnerSettings) (*SpeedTestResults, error) {
	base := stRunnerSettings.getBaseSettings()
	var progressCb func(SpeedTestResult)
	if base.ProgressCallback != nil {
		var ok bool
		progressCb, ok = base.ProgressCallback.(func(SpeedTestResult))
		if !ok {
			return nil, fmt.Errorf("invalid ProgressCallback type: expected func(SpeedTestResult)")
		}
	}

	itr := &ipcTestRunner[SpeedTestResult, *SpeedTestRunnerSettings]{
		tr:       tr,
		ctx:      ctx,
		tags:     tags,
		settings: &stRunnerSettings,
		buildTestReq: func(currentTags []string, c *SpeedTestRunnerSettings) worker.Request {
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

			s, err := mustMarshal(worker.SpeedTestSettings{
				Mode:        c.Mode,
				Timeout:     c.Timeout,
				TargetBytes: c.TargetBytes,
				Concurrency: base.Concurrency,
				TestURL:     testURL,
				RawRequest:  rawReq,
			})
			if err != nil {
				panic(fmt.Sprintf("marshal speed settings: %v", err))
			}

			return worker.Request{
				Type:     worker.RequestTypeTest,
				TestType: worker.TestTypeSpeed,
				Tags:     currentTags,
				Settings: s,
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
		isSuccess:  func(r SpeedTestResult) bool { return r.Error == nil },
		getTag:     func(r SpeedTestResult) string { return r.Tag },
		aggregate: func(rs []SpeedTestResult, sort bool) any {
			return aggregateSpeedResults(rs, sort)
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
	tags         []string
	settings     TSettings
	buildTestReq func([]string, TSettings) worker.Request
	convert      func(worker.Response) TResult
	onProgress   func(TResult)
	isSuccess    func(TResult) bool
	getTag       func(TResult) string
	aggregate    func([]TResult, bool) any
}

func (itr *ipcTestRunner[TResult, TSettings]) run() (any, error) {
	base := itr.settings.getBaseSettings()

	itr.tr.testMu.Lock()
	defer itr.tr.testMu.Unlock()

	proc, err := itr.tr.ensureProc()
	if err != nil {
		return nil, err
	}

	currentTags := itr.tags

	// --- Test phase ---
	var final []TResult

	for round := 0; round < base.Rounds; round++ {
		select {
		case <-itr.ctx.Done():
			return itr.aggregate(final, base.SortResults), itr.ctx.Err()
		default:
		}

		if base.RoundStartedCallback != nil {
			base.RoundStartedCallback(round, len(currentTags))
		}

		req := itr.buildTestReq(currentTags, itr.settings)
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
			next := make([]string, 0, len(good))
			for _, tag := range currentTags {
				if good[tag] {
					next = append(next, tag)
				}
			}
			currentTags = next
		}
	}

	return itr.aggregate(final, base.SortResults), nil
}
