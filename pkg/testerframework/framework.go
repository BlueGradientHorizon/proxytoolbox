package testerframework

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
	"syscall"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/pkg/ipcprotocol"
)

// CoreTester is the ONLY interface a new core has to implement.
type CoreTester interface {
	Info() ipcprotocol.CoreInfo
	Validate(ctx context.Context, configs []*core.OutboundConfig, sendResult func(ipcprotocol.Response)) error
	TestLatency(ctx context.Context, configs []*core.OutboundConfig, settings ipcprotocol.LatencySettings, sendResult func(ipcprotocol.Response)) error
	TestSpeed(ctx context.Context, configs []*core.OutboundConfig, settings ipcprotocol.SpeedSettings, sendResult func(ipcprotocol.Response)) error
}

// Run parses --info / --run and blocks forever serving TCP requests.
func Run(tester CoreTester) {
	var infoFlag, runFlag bool
	flag.BoolVar(&infoFlag, "info", false, "Print core info as JSON and exit")
	flag.BoolVar(&runFlag, "run", false, "Run tester server")
	flag.Parse()

	if infoFlag {
		b, _ := json.Marshal(tester.Info())
		fmt.Println(string(b))
		os.Exit(0)
	}

	if !runFlag {
		fmt.Fprintln(os.Stderr, "Usage: tester --info | --run")
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

	handle(conn, tester)
}

func handle(conn net.Conn, tester CoreTester) {
	bw := bufio.NewWriter(conn)
	dec := json.NewDecoder(conn)

	write := func(r ipcprotocol.Response) {
		b, _ := json.Marshal(r)
		fmt.Fprintf(bw, "%s\n", b)
		bw.Flush()
	}

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
		var req ipcprotocol.Request
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return
			}
			write(ipcprotocol.Response{Type: "error", Error: err.Error()})
			return
		}

		var err error
		switch req.Type {
		case "validate":
			err = tester.Validate(ctx, toCoreConfigs(req.Configs), write)
		case "test":
			switch req.TestType {
			case ipcprotocol.LatencyTest:
				var s ipcprotocol.LatencySettings
				if err = json.Unmarshal(req.Settings, &s); err == nil {
					err = tester.TestLatency(ctx, toCoreConfigs(req.Configs), s, write)
				}
			case ipcprotocol.SpeedTest:
				var s ipcprotocol.SpeedSettings
				if err = json.Unmarshal(req.Settings, &s); err == nil {
					err = tester.TestSpeed(ctx, toCoreConfigs(req.Configs), s, write)
				}
			default:
				err = fmt.Errorf("unknown test type: %s", req.TestType)
			}
		default:
			err = fmt.Errorf("unknown request type: %s", req.Type)
		}

		if err != nil {
			write(ipcprotocol.Response{Type: "error", Error: err.Error()})
		}
		write(ipcprotocol.Response{Type: "done"})
	}
}

func toCoreConfigs(raw []*ipcprotocol.RawConfig) []*core.OutboundConfig {
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
