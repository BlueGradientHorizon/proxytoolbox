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
func (tr *TestRunner) Validate(ctx context.Context, configs []parsers.ProxyConfig, taggerFunc func(string, int) string) ([]parsers.ProxyConfig, []*ValidationError, error) {
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

	var validationErrors []*ValidationError

	validateReq := &worker.PBRequest{
		Type:    worker.PBRequestType_REQUEST_VALIDATE,
		Configs: toPBOutboundConfigs(extractConfigs(configsCopy)),
	}

	err = proc.SendRequest(ctx, validateReq, func(r *worker.PBResponse) {
		if r.GetType() == worker.PBResponseType_RESPONSE_VALIDATION {
			validationErrors = r.GetValidationErrors()
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
		buildTestReq: func(currentTags []string, c *LatencyTestRunnerSettings) *worker.PBRequest {
			testURL := c.TestURL
			if testURL == "" {
				testURL = presets.Google204
			}

			req, _ := http.NewRequest(http.MethodHead, testURL, nil)
			if c.ModifyRequest != nil {
				c.ModifyRequest(req)
			}
			rawReq, _ := httputil.DumpRequest(req, false)

			return &worker.PBRequest{
				Type:     worker.PBRequestType_REQUEST_TEST,
				TestType: worker.PBTestType_TEST_LATENCY,
				Tags:     worker.StringsToPBTags(currentTags),
				Settings: &worker.PBRequest_LatencySettings{
					LatencySettings: worker.LatencyTestSettingsToPB(worker.LatencyTestSettings{
						TestURL:     testURL,
						RawRequest:  rawReq,
						Timeout:     c.Timeout,
						Concurrency: base.Concurrency,
					}),
				},
			}
		},
		convert: func(r *worker.PBResponse) LatencyTestResult {
			var err error
			if r.GetError() != "" {
				err = fmt.Errorf("%s", r.GetError())
			}
			return LatencyTestResult{Tag: worker.PBBytesToString(r.GetTag()), Delay: r.GetLatencyMs(), Error: err}
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
		buildTestReq: func(currentTags []string, c *SpeedTestRunnerSettings) *worker.PBRequest {
			testURL := c.Provider.GetURL(c.Mode, c.TargetBytes)

			method := http.MethodGet
			if c.Mode == worker.SpeedTestModeUpload {
				method = http.MethodPost
			}

			req, _ := http.NewRequest(method, testURL, nil)
			if c.Provider.ModifyRequest != nil {
				c.Provider.ModifyRequest(req, c.Mode, c.TargetBytes)
			}

			rawReq, _ := httputil.DumpRequest(req, false)

			return &worker.PBRequest{
				Type:     worker.PBRequestType_REQUEST_TEST,
				TestType: worker.PBTestType_TEST_SPEED,
				Tags:     worker.StringsToPBTags(currentTags),
				Settings: &worker.PBRequest_SpeedSettings{
					SpeedSettings: worker.SpeedTestSettingsToPB(worker.SpeedTestSettings{
						Mode:        c.Mode,
						Timeout:     c.Timeout,
						TargetBytes: c.TargetBytes,
						Concurrency: base.Concurrency,
						TestURL:     testURL,
						RawRequest:  rawReq,
					}),
				},
			}
		},
		convert: func(r *worker.PBResponse) SpeedTestResult {
			var err error
			if r.GetError() != "" {
				err = fmt.Errorf("%s", r.GetError())
			}
			return SpeedTestResult{Tag: worker.PBBytesToString(r.GetTag()), Speed: r.GetSpeed(), Error: err}
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
	buildTestReq func([]string, TSettings) *worker.PBRequest
	convert      func(*worker.PBResponse) TResult
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

		err = proc.SendRequest(itr.ctx, req, func(r *worker.PBResponse) {
			switch r.GetType() {
			case worker.PBResponseType_RESPONSE_RESULT:
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
