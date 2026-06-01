package worker

import (
	"bufio"
	"context"
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
	"google.golang.org/protobuf/proto"
)

// Worker is the ONLY interface a new worker has to implement.
type Worker interface {
	Info() PBCoreInfo
	Validate(ctx context.Context, configs []*core.OutboundConfig, sendResult func(*PBResponse)) error
	TestLatency(ctx context.Context, settings LatencyTestSettings, tags []string, sendResult func(*PBResponse)) error
	TestSpeed(ctx context.Context, settings SpeedTestSettings, tags []string, sendResult func(*PBResponse)) error
}

// CoreAdapter defines the minimal interface that concrete proxy cores must
// implement to plug into BaseWorker. It contains only core-specific hooks;
// all lifecycle, routing, and IPC logic is handled by BaseWorker.
type CoreAdapter interface {
	// Info returns core identification metadata.
	Info() PBCoreInfo
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
func (bw *BaseWorker) Info() PBCoreInfo {
	return bw.adapter.Info()
}

// Validate converts configurations, validates them individually and as a batch,
// streams validation errors via IPC, and stores the valid survivors.
func (bw *BaseWorker) Validate(ctx context.Context, configs []*core.OutboundConfig, sendResult func(*PBResponse)) error {
	var validationErrors []ValidationError
	var validConfigs []*core.OutboundConfig
	var validObjects []any

	for _, cfg := range configs {
		obj, err := recoverValueError(func() (any, error) {
			return bw.adapter.Convert(cfg)
		})
		if err != nil {
			validationErrors = append(validationErrors, ValidationError{
				Tag:   cfg.Tag,
				Error: "convert: " + cfg.Type + ": " + err.Error(),
			})
			continue
		}
		if err := recoverError(func() error {
			return bw.adapter.ValidateSingle(ctx, obj)
		}); err != nil {
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
		if err := recoverError(func() error {
			return bw.adapter.ValidateBatch(ctx, validObjects)
		}); err != nil {
			validationErrors = append(validationErrors, ValidationError{
				Error: err.Error(),
			})
		}
	}

	sendResult(&PBResponse{
		Type:             PBResponseType_RESPONSE_VALIDATION,
		ValidationErrors: ValidationErrorsToPB(validationErrors),
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
func (bw *BaseWorker) TestLatency(ctx context.Context, settings LatencyTestSettings, tags []string, sendResult func(*PBResponse)) error {
	return bw.runTest(ctx, settings, tags, sendResult)
}

// TestSpeed implements Worker.TestSpeed by delegating to runTest.
func (bw *BaseWorker) TestSpeed(ctx context.Context, settings SpeedTestSettings, tags []string, sendResult func(*PBResponse)) error {
	return bw.runTest(ctx, settings, tags, sendResult)
}

// runTest orchestrates the full test lifecycle: tag filtering, instance creation
// and startup, dialer extraction, test execution, result streaming, and
// asynchronous instance teardown.
func (bw *BaseWorker) runTest(ctx context.Context, settings any, tags []string, sendResult func(*PBResponse)) error {
	configs, objects, missing := bw.selectByTags(tags)

	for _, tag := range missing {
		sendResult(&PBResponse{Type: PBResponseType_RESPONSE_RESULT, Tag: pbB(tag), Error: "tag not found"})
	}

	if len(configs) == 0 {
		return nil
	}

	inst, err := recoverValueError(func() (any, error) {
		return bw.adapter.CreateInstance(ctx, objects)
	})
	if err != nil {
		for _, cfg := range configs {
			sendResult(&PBResponse{Type: PBResponseType_RESPONSE_RESULT, Tag: pbB(cfg.Tag), Error: err.Error()})
		}
		return nil
	}

	if err := recoverError(func() error {
		return bw.adapter.StartInstance(inst)
	}); err != nil {
		bw.adapter.CloseInstance(inst)
		return err
	}

	proxies, dialers, err := recoverDialers(func() ([]ProxyInfo, []DialerFunc, error) {
		return bw.adapter.ExtractDialers(inst)
	})
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
			resp := &PBResponse{Type: PBResponseType_RESPONSE_RESULT, Tag: pbB(r.Tag), LatencyMs: r.Delay}
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
			resp := &PBResponse{Type: PBResponseType_RESPONSE_RESULT, Tag: pbB(r.Tag), Speed: r.Speed}
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
	flag.BoolVar(&infoFlag, "info", false, "Print core info as protobuf and exit")
	flag.BoolVar(&runFlag, "run", false, "Run worker server")
	flag.Parse()

	if infoFlag {
		info := worker.Info()
		b, err := proto.Marshal(&info)
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal info: %v\n", err)
			os.Exit(1)
		}
		os.Stdout.Write(b)
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
		var req PBRequest
		if err := ReadPB(conn, &req); err != nil {
			if err == io.EOF {
				return
			}
			sw.Write(&PBResponse{Type: PBResponseType_RESPONSE_ERROR, Error: err.Error()})
			return
		}

		// Handle each request in its own goroutine so that the main read loop
		// can immediately detect connection loss (parent termination) via EOF
		// even while a long-running test is in progress.
		go func(req PBRequest) {
			if !sw.TryLock() {
				sw.Write(&PBResponse{Type: PBResponseType_RESPONSE_BUSY})
				sw.Write(&PBResponse{Type: PBResponseType_RESPONSE_DONE})
				return
			}
			defer sw.Unlock()

			var err error
			switch req.GetType() {
			case PBRequestType_REQUEST_VALIDATE:
				configs, deserializeErrs := toCoreConfigs(req.GetConfigs())
				sendResultWrapped := func(r *PBResponse) {
					if r.GetType() == PBResponseType_RESPONSE_VALIDATION {
						merged := append(deserializeErrs, ValidationErrorsFromPB(r.GetValidationErrors())...)
						r.ValidationErrors = ValidationErrorsToPB(merged)
					}
					sw.Write(r)
				}
				err = worker.Validate(ctx, configs, sendResultWrapped)
			case PBRequestType_REQUEST_TEST:
				switch req.GetTestType() {
				case PBTestType_TEST_LATENCY:
					if ls, ok := req.GetSettings().(*PBRequest_LatencySettings); ok && ls.LatencySettings != nil {
						err = worker.TestLatency(ctx, PBLatencyTestSettingsToGo(ls.LatencySettings), PBTagsToStrings(req.GetTags()), sw.Write)
					} else {
						err = fmt.Errorf("missing latency settings")
					}
				case PBTestType_TEST_SPEED:
					if ss, ok := req.GetSettings().(*PBRequest_SpeedSettings); ok && ss.SpeedSettings != nil {
						err = worker.TestSpeed(ctx, PBSpeedTestSettingsToGo(ss.SpeedSettings), PBTagsToStrings(req.GetTags()), sw.Write)
					} else {
						err = fmt.Errorf("missing speed settings")
					}
				default:
					err = fmt.Errorf("unknown test type: %v", req.GetTestType())
				}
			default:
				err = fmt.Errorf("unknown request type: %v", req.GetType())
			}

			if err != nil {
				sw.Write(&PBResponse{Type: PBResponseType_RESPONSE_ERROR, Error: err.Error()})
			}
			sw.Write(&PBResponse{Type: PBResponseType_RESPONSE_DONE})
		}(req)
	}
}

type sessionWriter struct {
	sessionMu sync.Mutex
	writeMu   sync.Mutex
	bw        *bufio.Writer
}

func (sw *sessionWriter) Write(r *PBResponse) {
	sw.writeMu.Lock()
	defer sw.writeMu.Unlock()
	WritePB(sw.bw, r)
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

func toCoreConfigs(raw []*PBOutboundConfig) ([]*core.OutboundConfig, []ValidationError) {
	out := make([]*core.OutboundConfig, 0, len(raw))
	var errs []ValidationError
	for _, rc := range raw {
		cfg, err := PBOutboundConfigToCore(rc)
		if err != nil {
			tag := ""
			if rc != nil {
				tag = pbS(rc.GetTag())
			}
			errs = append(errs, ValidationError{
				Tag:   tag,
				Error: "deserialize: " + err.Error(),
			})
			continue
		}
		out = append(out, cfg)
	}
	return out, errs
}
