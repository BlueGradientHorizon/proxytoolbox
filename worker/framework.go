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

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

// Worker is the ONLY interface a new worker has to implement.
type Worker interface {
	Info() CoreInfo
	Validate(ctx context.Context, configs []*core.OutboundConfig, sendResult func(Response)) error
	TestLatency(ctx context.Context, settings LatencySettings, tags []string, sendResult func(Response)) error
	TestSpeed(ctx context.Context, settings SpeedSettings, tags []string, sendResult func(Response)) error
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
				sw.Lock()
				sw.Write(Response{Type: ResponseTypeBusy})
				sw.Write(Response{Type: ResponseTypeDone})
				sw.Unlock()
				return
			}
			defer sw.Unlock()

			var err error
			switch req.Type {
			case RequestTypeValidate:
				err = worker.Validate(ctx, toCoreConfigs(req.Configs), sw.Write)
			case RequestTypeTest:
				switch req.TestType {
				case TestTypeLatency:
					var s LatencySettings
					if err = json.Unmarshal(req.Settings, &s); err == nil {
						err = worker.TestLatency(ctx, s, req.Tags, sw.Write)
					}
				case TestTypeSpeed:
					var s SpeedSettings
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

func toCoreConfigs(raw []*RawConfig) []*core.OutboundConfig {
	out := make([]*core.OutboundConfig, 0, len(raw))
	for _, rc := range raw {
		cfg, err := rc.ToCore()
		if err != nil {
			continue
		}
		out = append(out, cfg)
	}
	return out
}
