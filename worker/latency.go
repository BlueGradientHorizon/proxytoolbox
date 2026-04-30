package worker

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// ProxyInfo contains minimal information about a proxy for testing.
// This is intentionally generic to avoid coupling to any specific proxy core.
type ProxyInfo struct {
	Tag  string // Unique identifier for the proxy
	Type string // Protocol type (vless, trojan, vmess, etc.)
}

// LatencyTestResult contains the result of a latency test for a single proxy.
type LatencyTestResult struct {
	Tag   string
	Delay int64
	Error error
}

// LatencyTestSettings configures the latency test behavior.
type LatencyTestSettings struct {
	TestURL     string        `json:"test_url,omitempty"`
	RawRequest  []byte        `json:"raw_request"`
	Timeout     time.Duration `json:"timeout"`
	Concurrency int           `json:"concurrency"`
}

// LatencyTest performs latency testing on multiple proxies in parallel.
type LatencyTest struct {
	ctx      context.Context
	settings LatencyTestSettings
	items    []latencyTestItem
}

type latencyTestItem struct {
	proxy  ProxyInfo
	client *http.Client
	start  *time.Time
}

// NewLatencyTest creates a new latency test with the given proxies.
// Each proxy is represented by a ProxyInfo and a DialerFunc that establishes connections.
func NewLatencyTest(
	ctx context.Context,
	sett LatencyTestSettings,
	proxies []ProxyInfo,
	dialers []DialerFunc,
	tlsConfigProvider TLSConfigProvider,
) (*LatencyTest, error) {
	if sett.TestURL == "" && len(sett.RawRequest) == 0 {
		return nil, errors.New("LatencyTest: TestURL or RawRequest is empty")
	}

	if len(proxies) != len(dialers) {
		return nil, errors.New("LatencyTest: proxies and dialers length mismatch")
	}

	items := make([]latencyTestItem, len(proxies))
	for i := range proxies {
		var startTime time.Time

		// Wrap the dialer to capture start time
		timedDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
			startTime = time.Now()
			return dialers[i](ctx, network, addr)
		}

		items[i] = latencyTestItem{
			proxy:  proxies[i],
			client: newTestClient(ctx, timedDialer, tlsConfigProvider),
			start:  &startTime,
		}
	}

	return &LatencyTest{
		ctx:      ctx,
		settings: sett,
		items:    items,
	}, nil
}

// Run executes the latency test for all proxies in parallel.
// Results are sent to all provided result channels.
// Returns a function that waits for all goroutines to complete.
func (t *LatencyTest) Run(resChans ...chan<- LatencyTestResult) func() {
	buf := bufio.NewReader(bytes.NewReader(t.settings.RawRequest))
	req, err := http.ReadRequest(buf)
	if err != nil {
		err = fmt.Errorf("failed to parse raw request: %w", err)
		for i := range t.items {
			for _, c := range resChans {
				if c != nil {
					select {
					case c <- LatencyTestResult{
						Tag:   t.items[i].proxy.Tag,
						Delay: -1,
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

	return runParallel(t.ctx, t.settings.Timeout, len(t.items), t.settings.Concurrency, func(ctx context.Context, i int) LatencyTestResult {
		item := t.items[i]
		defer item.client.CloseIdleConnections()

		val, err := t.runTest(ctx, item, baseReq)
		if err != nil {
			val = -1
		}
		return LatencyTestResult{
			Tag:   item.proxy.Tag,
			Delay: val,
			Error: err,
		}
	}, resChans...)
}

func (t *LatencyTest) runTest(ctx context.Context, item latencyTestItem, baseReq *http.Request) (int64, error) {
	req := baseReq.WithContext(ctx)

	resp, err := item.client.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return -1, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return int64(time.Since(*item.start) / time.Millisecond), nil
}
