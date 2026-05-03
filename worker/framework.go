package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

// Worker is the ONLY interface a new worker has to implement.
type Worker interface {
	Info() CoreInfo
	Validate(ctx context.Context, configs []*core.OutboundConfig, sendResult func(Response)) error
	TestLatency(ctx context.Context, settings LatencyTestSettings, tags []string, sendResult func(Response)) error
	TestSpeed(ctx context.Context, settings SpeedTestSettings, tags []string, sendResult func(Response)) error
}

// CoreAdapter defines the minimal interface that concrete proxy cores must
// implement to plug into BaseWorker. It contains only core-specific hooks;
// all lifecycle, routing, and IPC logic is handled by BaseWorker.
type CoreAdapter interface {
	// Info returns core identification metadata.
	Info() CoreInfo
	// Convert transforms a generic OutboundConfig into a core-specific object.
	Convert(cfg *core.OutboundConfig) (any, error)
	// ValidateSingle checks a single converted config for validity.
	ValidateSingle(ctx context.Context, obj any) error
	// ValidateBatch checks a batch of converted configs for cross-config conflicts.
	ValidateBatch(ctx context.Context, objs []any) error
	// CreateInstance builds a core-specific proxy instance from converted configs.
	CreateInstance(ctx context.Context, converted []any) (any, error)
	// StartInstance starts the given instance.
	StartInstance(inst any) error
	// ExtractDialers extracts proxy metadata and dialer functions from a running instance.
	ExtractDialers(inst any) ([]ProxyInfo, []DialerFunc, error)
	// CloseInstance tears down the given instance.
	CloseInstance(inst any)
	// TLSProvider returns a TLS configuration provider for the core.
	TLSProvider(ctx context.Context) TLSConfigProvider
}

// BaseWorker provides a generic, reusable implementation of the Worker interface.
// It orchestrates config validation, tag filtering, test execution, and IPC
// result streaming. Concrete cores supply a CoreAdapter to handle core-specific
// operations.
type BaseWorker struct {
	adapter CoreAdapter
	mu      sync.Mutex
	configs []*core.OutboundConfig
	objects []any
}

// NewBaseWorker creates a new BaseWorker that delegates core-specific operations
// to the provided adapter.
func NewBaseWorker(adapter CoreAdapter) *BaseWorker {
	return &BaseWorker{adapter: adapter}
}

// Info returns core information from the adapter.
func (bw *BaseWorker) Info() CoreInfo {
	return bw.adapter.Info()
}

// Validate converts configurations, validates them individually and as a batch,
// streams validation errors via IPC, and stores the valid survivors.
func (bw *BaseWorker) Validate(ctx context.Context, configs []*core.OutboundConfig, sendResult func(Response)) error {
	var validationErrors []ValidationError
	var validConfigs []*core.OutboundConfig
	var validObjects []any

	for _, cfg := range configs {
		obj, err := bw.adapter.Convert(cfg)
		if err != nil {
			validationErrors = append(validationErrors, ValidationError{
				Tag:   cfg.Tag,
				Error: "convert: " + cfg.Type + ": " + err.Error(),
			})
			continue
		}
		if err := bw.adapter.ValidateSingle(ctx, obj); err != nil {
			validationErrors = append(validationErrors, ValidationError{
				Tag:   cfg.Tag,
				Error: "instantiate: " + cfg.Type + ": " + err.Error(),
			})
			continue
		}
		validConfigs = append(validConfigs, cfg)
		validObjects = append(validObjects, obj)
	}

	if len(validObjects) > 0 {
		if err := bw.adapter.ValidateBatch(ctx, validObjects); err != nil {
			validationErrors = append(validationErrors, ValidationError{
				Tag:   "",
				Error: err.Error(),
			})
		}
	}

	sendResult(Response{
		Type:             ResponseTypeValidation,
		ValidationErrors: validationErrors,
	})

	bw.mu.Lock()
	bw.configs = validConfigs
	bw.objects = validObjects
	bw.mu.Unlock()

	return nil
}

// selectByTags returns the configs and converted objects that match the requested
// tags. If tags is empty, all stored configs are returned. It also returns a list
// of requested tags that were not found.
func (bw *BaseWorker) selectByTags(tags []string) ([]*core.OutboundConfig, []any, []string) {
	bw.mu.Lock()
	defer bw.mu.Unlock()

	if len(tags) == 0 {
		configs := make([]*core.OutboundConfig, len(bw.configs))
		copy(configs, bw.configs)
		objects := make([]any, len(bw.objects))
		copy(objects, bw.objects)
		return configs, objects, nil
	}

	configMap := make(map[string]*core.OutboundConfig, len(bw.configs))
	objectMap := make(map[string]any, len(bw.configs))
	for i, cfg := range bw.configs {
		configMap[cfg.Tag] = cfg
		objectMap[cfg.Tag] = bw.objects[i]
	}

	var matchedConfigs []*core.OutboundConfig
	var matchedObjects []any
	var missing []string

	for _, tag := range tags {
		if cfg, ok := configMap[tag]; ok {
			matchedConfigs = append(matchedConfigs, cfg)
			matchedObjects = append(matchedObjects, objectMap[tag])
		} else {
			missing = append(missing, tag)
		}
	}

	return matchedConfigs, matchedObjects, missing
}

// TestLatency implements Worker.TestLatency by delegating to runTest.
func (bw *BaseWorker) TestLatency(ctx context.Context, settings LatencyTestSettings, tags []string, sendResult func(Response)) error {
	return bw.runTest(ctx, settings, tags, sendResult)
}

// TestSpeed implements Worker.TestSpeed by delegating to runTest.
func (bw *BaseWorker) TestSpeed(ctx context.Context, settings SpeedTestSettings, tags []string, sendResult func(Response)) error {
	return bw.runTest(ctx, settings, tags, sendResult)
}

// runTest orchestrates the full test lifecycle: tag filtering, instance creation
// and startup, dialer extraction, test execution, result streaming, and
// asynchronous instance teardown.
func (bw *BaseWorker) runTest(ctx context.Context, settings any, tags []string, sendResult func(Response)) error {
	configs, objects, missing := bw.selectByTags(tags)

	for _, tag := range missing {
		sendResult(Response{Type: ResponseTypeResult, Tag: tag, Error: "tag not found"})
	}

	if len(configs) == 0 {
		return nil
	}

	inst, err := bw.adapter.CreateInstance(ctx, objects)
	if err != nil {
		for _, cfg := range configs {
			sendResult(Response{Type: ResponseTypeResult, Tag: cfg.Tag, Error: err.Error()})
		}
		return nil
	}

	if err := bw.adapter.StartInstance(inst); err != nil {
		bw.adapter.CloseInstance(inst)
		return err
	}

	proxies, dialers, err := bw.adapter.ExtractDialers(inst)
	if err != nil {
		bw.adapter.CloseInstance(inst)
		return err
	}

	switch s := settings.(type) {
	case LatencyTestSettings:
		lt, err := NewLatencyTest(ctx, s, proxies, dialers, bw.adapter.TLSProvider(ctx))
		if err != nil {
			bw.adapter.CloseInstance(inst)
			return err
		}
		ch := make(chan LatencyTestResult, len(proxies))
		wait := lt.Run(ch)
		for range proxies {
			r := <-ch
			resp := Response{Type: ResponseTypeResult, Tag: r.Tag, LatencyMs: r.Delay}
			if r.Error != nil {
				resp.Error = r.Error.Error()
			}
			sendResult(resp)
		}
		wait()
	case SpeedTestSettings:
		st, err := NewSpeedTest(ctx, s, proxies, dialers, bw.adapter.TLSProvider(ctx))
		if err != nil {
			bw.adapter.CloseInstance(inst)
			return err
		}
		ch := make(chan SpeedTestResult, len(proxies))
		wait := st.Run(ch)
		for range proxies {
			r := <-ch
			resp := Response{Type: ResponseTypeResult, Tag: r.Tag, Speed: r.Speed}
			if r.Error != nil {
				resp.Error = r.Error.Error()
			}
			sendResult(resp)
		}
		wait()
	default:
		bw.adapter.CloseInstance(inst)
		return fmt.Errorf("unknown settings type: %T", settings)
	}

	// Closing an instance could take up to an astronomical 5 seconds when 7k+ configs loaded
	// So intentionally close it asynchronously so the "done" response isn't delayed.
	go func() {
		start := time.Now()
		bw.adapter.CloseInstance(inst)
		fmt.Printf("instance closing took %v\n", time.Since(start))
	}()
	return nil
}

// Run parses --info / --run and blocks forever serving TCP requests.
func Run(worker Worker) {
	var infoFlag, runFlag bool
	flag.BoolVar(&infoFlag, "info", false, "Print core info as JSON and exit")
	flag.BoolVar(&runFlag, "run", false, "Run worker server")
	flag.Parse()

	if infoFlag {
		b, _ := json.Marshal(worker.Info())
		fmt.Println(string(b))
		os.Exit(0)
	}

	if !runFlag {
		fmt.Fprintln(os.Stderr, "Usage: worker --info | --run")
		os.Exit(1)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	fmt.Printf("PORT %d\n", port)
	os.Stdout.Sync()

	conn, err := ln.Accept()
	if err != nil {
		fmt.Fprintf(os.Stderr, "accept: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	handle(conn, worker)
}

func handle(conn net.Conn, worker Worker) {
	bw := bufio.NewWriter(conn)
	dec := json.NewDecoder(conn)
	sw := &sessionWriter{bw: bw}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sig:
			cancel()
		case <-ctx.Done():
		}
	}()

	for {
		var req Request
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return
			}
			sw.Write(Response{Type: ResponseTypeError, Error: err.Error()})
			return
		}

		// Handle each request in its own goroutine so that the main read loop
		// can immediately detect connection loss (parent termination) via EOF
		// even while a long-running test is in progress.
		go func(req Request) {
			if !sw.TryLock() {
				sw.Write(Response{Type: ResponseTypeBusy})
				sw.Write(Response{Type: ResponseTypeDone})
				return
			}
			defer sw.Unlock()

			var err error
			switch req.Type {
			case RequestTypeValidate:
				configs, deserializeErrs := toCoreConfigs(req.Configs)
				sendResultWrapped := func(r Response) {
					if r.Type == ResponseTypeValidation {
						r.ValidationErrors = append(deserializeErrs, r.ValidationErrors...)
					}
					sw.Write(r)
				}
				err = worker.Validate(ctx, configs, sendResultWrapped)
			case RequestTypeTest:
				switch req.TestType {
				case TestTypeLatency:
					var s LatencyTestSettings
					if err = json.Unmarshal(req.Settings, &s); err == nil {
						err = worker.TestLatency(ctx, s, req.Tags, sw.Write)
					}
				case TestTypeSpeed:
					var s SpeedTestSettings
					if err = json.Unmarshal(req.Settings, &s); err == nil {
						err = worker.TestSpeed(ctx, s, req.Tags, sw.Write)
					}
				default:
					err = fmt.Errorf("unknown test type: %s", req.TestType)
				}
			default:
				err = fmt.Errorf("unknown request type: %s", req.Type)
			}

			if err != nil {
				sw.Write(Response{Type: ResponseTypeError, Error: err.Error()})
			}
			sw.Write(Response{Type: ResponseTypeDone})
		}(req)
	}
}

type sessionWriter struct {
	sessionMu sync.Mutex
	lineMu    sync.Mutex
	bw        *bufio.Writer
}

func (sw *sessionWriter) Write(r Response) {
	sw.lineMu.Lock()
	defer sw.lineMu.Unlock()
	b, _ := json.Marshal(r)
	fmt.Fprintf(sw.bw, "%s\n", b)
	sw.bw.Flush()
}

func (sw *sessionWriter) TryLock() bool {
	return sw.sessionMu.TryLock()
}

func (sw *sessionWriter) Lock() {
	sw.sessionMu.Lock()
}

func (sw *sessionWriter) Unlock() {
	sw.sessionMu.Unlock()
}

func toCoreConfigs(raw []*RawConfig) ([]*core.OutboundConfig, []ValidationError) {
	out := make([]*core.OutboundConfig, 0, len(raw))
	var errs []ValidationError
	for _, rc := range raw {
		cfg, err := rc.ToCore()
		if err != nil {
			errs = append(errs, ValidationError{
				Tag:   rc.Tag,
				Error: "deserialize: " + err.Error(),
			})
			continue
		}
		out = append(out, cfg)
	}
	return out, errs
}
