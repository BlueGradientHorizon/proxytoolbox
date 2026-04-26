package testrunner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/pkg/ipcprotocol"
)

// TesterProcess wraps a single tester binary invocation.
type TesterProcess struct {
	path string
	cmd  *exec.Cmd
	conn net.Conn
	dec  *json.Decoder
	bw   *bufio.Writer
}

// Start executes the tester with --run, reads the TCP port from stdout, and connects.
func (tp *TesterProcess) Start() error {
	tp.cmd = exec.Command(tp.path, "--run")
	stdout, err := tp.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, _ := tp.cmd.StderrPipe()
	go func() { io.Copy(io.Discard, stderr) }()

	if err := tp.cmd.Start(); err != nil {
		return err
	}

	rd := bufio.NewReader(stdout)
	line, err := rd.ReadString('\n')
	if err != nil {
		tp.kill()
		return fmt.Errorf("tester exited before ready: %w", err)
	}
	line = strings.TrimSpace(line)
	const prefix = "PORT "
	if !strings.HasPrefix(line, prefix) {
		tp.kill()
		return fmt.Errorf("unexpected tester output: %s", line)
	}
	port, err := strconv.Atoi(strings.TrimPrefix(line, prefix))
	if err != nil {
		tp.kill()
		return fmt.Errorf("invalid port: %w", err)
	}

	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		tp.kill()
		return fmt.Errorf("cannot connect to tester: %w", err)
	}
	tp.conn = conn
	tp.dec = json.NewDecoder(conn)
	tp.bw = bufio.NewWriter(conn)
	return nil
}

// SendRequest sends the request and consumes all streamed responses until "done".
func (tp *TesterProcess) SendRequest(ctx context.Context, req ipcprotocol.Request, onResponse func(ipcprotocol.Response)) error {
	b, _ := json.Marshal(req)
	if _, err := fmt.Fprintf(tp.bw, "%s\n", b); err != nil {
		return err
	}
	if err := tp.bw.Flush(); err != nil {
		return err
	}

	var testErr error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		var r ipcprotocol.Response
		if err := tp.dec.Decode(&r); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("decode error: %w", err)
		}
		switch r.Type {
		case ipcprotocol.ResponseTypeDone:
			if testErr != nil {
				return testErr
			}
			return nil
		case ipcprotocol.ResponseTypeError:
			// Do not return immediately; keep reading until "done"
			// so the next round does not read leftover messages.
			testErr = fmt.Errorf("tester error: %s", r.Error)
		default:
			if onResponse != nil {
				onResponse(r)
			}
		}
	}
}

func (tp *TesterProcess) Close() error {
	if tp.conn != nil {
		tp.conn.Close()
		tp.conn = nil
	}
	tp.kill()
	return nil
}

func (tp *TesterProcess) kill() {
	if tp.cmd != nil && tp.cmd.Process != nil {
		tp.cmd.Process.Kill()
		tp.cmd.Wait()
		tp.cmd = nil
	}
}
