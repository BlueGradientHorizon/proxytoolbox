package registry

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
)

type WorkerInfo struct {
	Name    string
	Version string
	Path    string
}

type workerInfoJSON struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Registry struct {
	entries map[string][]WorkerInfo
}

func NewRegistry() *Registry {
	return &Registry{entries: make(map[string][]WorkerInfo)}
}

// Discover scans the given directories for worker executables and probes them with --info.
// Only considers files whose name contains "worker".
func (r *Registry) Discover(paths ...string) error {
	for _, dir := range paths {
		dirEntry, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range dirEntry {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			// Only probe files that look like worker programs.
			if !strings.Contains(strings.ToLower(name), "worker") {
				continue
			}
			isExe, err := isExecutable(e)
			if err != nil {
				continue
			}
			if !isExe {
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

func (r *Registry) Get(name string) []WorkerInfo { return r.entries[name] }
func (r *Registry) All() map[string][]WorkerInfo { return r.entries }

func probe(path string) (*WorkerInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--info")
	// Detach stdin so the probed program never blocks waiting for console input.
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var ci workerInfoJSON
	if err := json.Unmarshal(out, &ci); err != nil {
		return nil, err
	}
	if ci.Name == "" {
		return nil, fmt.Errorf("worker %s returned empty name", path)
	}
	return &WorkerInfo{Name: ci.Name, Version: ci.Version, Path: path}, nil
}

func isExecutable(entry os.DirEntry) (bool, error) {
	if runtime.GOOS == "windows" {
		return strings.HasSuffix(strings.ToLower(entry.Name()), ".exe"), nil
	}
	i, err := entry.Info()
	if err != nil {
		return false, err
	}
	return i.Mode()&0111 != 0, nil
}
