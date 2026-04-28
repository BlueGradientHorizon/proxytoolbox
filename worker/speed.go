package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SpeedTestResult contains the result of a speed test for a single proxy.
type SpeedTestResult struct {
	Tag   string
	Speed float64
	Proxy ProxyInfo
	Error error
}

// SpeedTestMode indicates whether to test download or upload speed.
type SpeedTestMode int

const (
	SpeedTestModeDownload SpeedTestMode = iota
	SpeedTestModeUpload
)

// SpeedTestProvider defines how to construct speed test requests.
type SpeedTestProvider struct {
	GetURL        func(mode SpeedTestMode, targetBytes int64) string
	ModifyRequest func(req *http.Request, mode SpeedTestMode, targetBytes int64)
}

// SpeedTestSettings configures the speed test behavior.
type SpeedTestSettings struct {
	Mode        SpeedTestMode
	Provider    SpeedTestProvider
	Timeout     time.Duration
	TargetBytes int64
	Concurrency int
}

// NewDownloadTestSettings creates default download speed test settings.
func NewDownloadTestSettings() SpeedTestSettings {
	return SpeedTestSettings{
		Mode:        SpeedTestModeDownload,
		Timeout:     20 * time.Second,
		TargetBytes: 10 * 1024 * 1024,
	}
}

// NewUploadTestSettings creates default upload speed test settings.
func NewUploadTestSettings() SpeedTestSettings {
	return SpeedTestSettings{
		Mode:        SpeedTestModeUpload,
		Timeout:     20 * time.Second,
		TargetBytes: 10 * 1024 * 1024,
	}
}

// SpeedTest performs speed testing on multiple proxies in parallel.
type SpeedTest struct {
	ctx      context.Context
	settings SpeedTestSettings
	items    []speedTestItem
}

type speedTestItem struct {
	proxy  ProxyInfo
	client *http.Client
}

// NewSpeedTest creates a new speed test with the given proxies.
// Each proxy is represented by a ProxyInfo and a DialerFunc that establishes connections.
func NewSpeedTest(
	ctx context.Context,
	sett SpeedTestSettings,
	proxies []ProxyInfo,
	dialers []DialerFunc,
	tlsConfigProvider TLSConfigProvider,
) (*SpeedTest, error) {
	if sett.Provider.GetURL == nil {
		return nil, errors.New("NewSpeedTest: provider's GetURL is nil")
	}

	if sett.Provider.ModifyRequest == nil {
		sett.Provider.ModifyRequest = func(r *http.Request, m SpeedTestMode, b int64) {}
	}

	if len(proxies) != len(dialers) {
		return nil, errors.New("NewSpeedTest: proxies and dialers length mismatch")
	}

	items := make([]speedTestItem, len(proxies))
	for i := range proxies {
		items[i] = speedTestItem{
			proxy:  proxies[i],
			client: newTestClient(ctx, dialers[i], tlsConfigProvider),
		}
	}

	return &SpeedTest{
		ctx:      ctx,
		settings: sett,
		items:    items,
	}, nil
}

// Run executes the speed test for all proxies in parallel.
// Results are sent to all provided result channels.
// Returns a function that waits for all goroutines to complete.
func (t *SpeedTest) Run(resChans ...chan<- SpeedTestResult) func() {
	return runParallel(t.ctx, t.settings.Timeout, len(t.items), t.settings.Concurrency, func(ctx context.Context, i int) SpeedTestResult {
		item := t.items[i]
		defer item.client.CloseIdleConnections()

		val, err := t.runTest(ctx, item)
		if err != nil {
			val = -1
		}
		return SpeedTestResult{
			Tag:   item.proxy.Tag,
			Speed: val,
			Proxy: item.proxy,
			Error: err,
		}
	}, resChans...)
}

func (t *SpeedTest) runTest(ctx context.Context, item speedTestItem) (float64, error) {
	var method string
	var body io.Reader

	switch t.settings.Mode {
	case SpeedTestModeDownload:
		method = http.MethodGet
	case SpeedTestModeUpload:
		method = http.MethodPost
		body = io.LimitReader(zeroReader{}, t.settings.TargetBytes)
	}

	finalURL := t.settings.Provider.GetURL(t.settings.Mode, t.settings.TargetBytes)

	req, err := http.NewRequestWithContext(ctx, method, finalURL, body)
	if err != nil {
		return 0, err
	}

	t.settings.Provider.ModifyRequest(req, t.settings.Mode, t.settings.TargetBytes)

	start := time.Now()
	resp, err := item.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return -1, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var bytesProcessed int64
	if t.settings.Mode == SpeedTestModeDownload {
		bytesProcessed, err = io.CopyN(io.Discard, resp.Body, t.settings.TargetBytes)
		if err != nil {
			return 0, err
		}
	} else {
		bytesProcessed = t.settings.TargetBytes
	}

	elapsed := time.Since(start).Seconds()
	if elapsed <= 0 {
		return 0, nil
	}
	return float64(bytesProcessed) / elapsed, nil
}

type zeroReader struct{}

func (z zeroReader) Read(p []byte) (n int, err error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
