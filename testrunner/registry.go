package testrunner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/pkg/ipcprotocol"
)

type TesterInfo struct {
	Name    string
	Version string
	Path    string
}

type Registry struct {
	entries map[string][]TesterInfo
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[string][]TesterInfo)}
}

// Discover scans the given directories for tester executables and probes them with --info.
// Only considers files whose name contains "tester".
func (r *Registry) Discover(paths ...string) error {
	for _, dir := range paths {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			// Only probe files that look like tester programs.
			if !strings.Contains(strings.ToLower(name), "tester") {
				continue
			}
			if !isExecutable(name) {
				continue
			}
			p := filepath.Join(dir, name)
			info, err := probe(p)
			if err != nil {
				continue
			}
			r.entries[info.Name] = append(r.entries[info.Name], *info)
		}
	}
	return nil
}

func (r *Registry) Get(name string) []TesterInfo { return r.entries[name] }
func (r *Registry) All() map[string][]TesterInfo { return r.entries }

func probe(path string) (*TesterInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--info")
	// Detach stdin so the probed program never blocks waiting for console input.
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var ci ipcprotocol.CoreInfo
	if err := json.Unmarshal(out, &ci); err != nil {
		return nil, err
	}
	if ci.Name == "" {
		return nil, fmt.Errorf("tester %s returned empty name", path)
	}
	return &TesterInfo{Name: ci.Name, Version: ci.Version, Path: path}, nil
}

func isExecutable(name string) bool {
	if runtime.GOOS == "windows" {
		return strings.HasSuffix(strings.ToLower(name), ".exe")
	}
	// On non-Windows we let exec.Command fail later if the file is not executable.
	return true
}
