package worker

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	TestURL     string
	RawRequest  []byte
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
	if sett.TestURL == "" && len(sett.RawRequest) == 0 {
		return nil, errors.New("NewSpeedTest: TestURL or RawRequest is empty")
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
	// Reconstruct the request once
	buf := bufio.NewReader(bytes.NewReader(t.settings.RawRequest))
	req, err := http.ReadRequest(buf)
	if err != nil {
		err = fmt.Errorf("failed to parse raw request: %w", err)
		for i := range t.items {
			for _, c := range resChans {
				if c != nil {
					select {
					case c <- SpeedTestResult{
						Tag:   t.items[i].proxy.Tag,
						Speed: -1,
						Proxy: t.items[i].proxy,
						Error: err,
					}:
					case <-t.ctx.Done():
						return func() {}
					}
				}
			}
		}
		return func() {}
	}
	req.RequestURI = ""
	req.URL, _ = url.Parse(t.settings.TestURL)
	baseReq := req

	return runParallel(t.ctx, t.settings.Timeout, len(t.items), t.settings.Concurrency, func(ctx context.Context, i int) SpeedTestResult {
		item := t.items[i]
		defer item.client.CloseIdleConnections()

		val, err := t.runTest(ctx, item, baseReq)
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

func (t *SpeedTest) runTest(ctx context.Context, item speedTestItem, baseReq *http.Request) (float64, error) {
	req := baseReq.WithContext(ctx)

	// Attach the generated body for uploads
	if t.settings.Mode == SpeedTestModeUpload {
		req.Body = io.NopCloser(io.LimitReader(zeroReader{}, t.settings.TargetBytes))
	}

	// Execute using the core-specific client
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
