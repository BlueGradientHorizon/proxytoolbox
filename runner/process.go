package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/worker"
)

// ErrWorkerBusy is returned when the worker process is already handling a request.
var ErrWorkerBusy = errors.New("worker is busy")

// WorkerProcess wraps a single worker binary invocation.
type WorkerProcess struct {
	path    string
	logPath string
	cmd     *exec.Cmd
	conn    net.Conn
	bw      *bufio.Writer
	logFile *os.File
}

// Start executes the worker with --run, reads the TCP port from stdout, and connects.
func (tp *WorkerProcess) Start() error {
	tp.cmd = exec.Command(tp.path, "--run")
	stdout, err := tp.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, _ := tp.cmd.StderrPipe()

	if err := tp.cmd.Start(); err != nil {
		return err
	}

	rd := bufio.NewReader(stdout)
	line, err := rd.ReadString('\n')
	if err != nil {
		tp.kill()
		return fmt.Errorf("worker exited before ready: %w", err)
	}
	line = strings.TrimSpace(line)
	const prefix = "PORT "
	if !strings.HasPrefix(line, prefix) {
		tp.kill()
		return fmt.Errorf("unexpected worker output: %s", line)
	}
	port, err := strconv.Atoi(strings.TrimPrefix(line, prefix))
	if err != nil {
		tp.kill()
		return fmt.Errorf("invalid port: %w", err)
	}

	if tp.logPath != "" {
		logFile, err := os.OpenFile(tp.logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			tp.kill()
			return fmt.Errorf("cannot create log file: %w", err)
		}
		tp.logFile = logFile

		var logMu sync.Mutex
		logFunc := func(r io.Reader, pipeName string) {
			br, ok := r.(*bufio.Reader)
			if !ok {
				br = bufio.NewReader(r)
			}
			for {
				lineBytes, err := br.ReadBytes('\n')
				if len(lineBytes) > 0 {
					logMu.Lock()
					logFile.Write(lineBytes)
					logFile.Sync()
					logMu.Unlock()
				}
				if err != nil {
					if err != io.EOF && !errors.Is(err, os.ErrClosed) {
						logMu.Lock()
						fmt.Fprintf(logFile, "%s read error: %v\n", pipeName, err)
						logFile.Sync()
						logMu.Unlock()
					}
					break
				}
			}
		}
		go logFunc(rd, "stdout")
		go logFunc(stderr, "stderr")
	} else {
		go func() { io.Copy(io.Discard, rd) }()
		go func() { io.Copy(io.Discard, stderr) }()
	}

	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		tp.kill()
		return fmt.Errorf("cannot connect to worker: %w", err)
	}
	tp.conn = conn
	tp.bw = bufio.NewWriter(conn)
	return nil
}

// SendRequest sends the request and consumes all streamed responses until "done".
func (tp *WorkerProcess) SendRequest(ctx context.Context, req *worker.PBRequest, onResponse func(*worker.PBResponse)) error {
	if err := worker.WritePB(tp.bw, req); err != nil {
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

		type decodeResult struct {
			r   worker.PBResponse
			err error
		}
		ch := make(chan decodeResult, 1)
		go func() {
			var r worker.PBResponse
			err := worker.ReadPB(tp.conn, &r)
			ch <- decodeResult{r, err}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case res := <-ch:
			r := res.r
			if res.err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("decode error: %w", res.err)
			}
			switch r.GetType() {
			case worker.PBResponseType_RESPONSE_DONE:
				if testErr != nil {
					return testErr
				}
				return nil
			case worker.PBResponseType_RESPONSE_ERROR:
				testErr = fmt.Errorf("tester error: %s", r.GetError())
			case worker.PBResponseType_RESPONSE_BUSY:
				testErr = ErrWorkerBusy
			default:
				if onResponse != nil {
					onResponse(&r)
				}
			}
		}
	}
}

func (tp *WorkerProcess) Close() error {
	if tp.conn != nil {
		tp.conn.Close()
		tp.conn = nil
	}
	tp.kill()
	if tp.logFile != nil {
		tp.logFile.Close()
		tp.logFile = nil
	}
	return nil
}

func (tp *WorkerProcess) kill() {
	if tp.cmd != nil && tp.cmd.Process != nil {
		tp.cmd.Process.Kill()
		tp.cmd.Wait()
		tp.cmd = nil
	}
}
