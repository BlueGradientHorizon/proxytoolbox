package runner

import (
	"encoding/json"
	"sort"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/parsers"
	"github.com/bluegradienthorizon/proxytoolbox/worker"
)

func extractConfigs(configs []parsers.ProxyConfig) []*core.OutboundConfig {
	out := make([]*core.OutboundConfig, 0, len(configs))
	for _, p := range configs {
		if p.Config != nil {
			out = append(out, p.Config)
		}
	}
	return out
}

func toRawConfigs(configs []*core.OutboundConfig) []*worker.RawConfig {
	out := make([]*worker.RawConfig, 0, len(configs))
	for _, c := range configs {
		s, _ := json.Marshal(c.Settings)
		out = append(out, &worker.RawConfig{
			Tag: c.Tag, Type: c.Type, Server: c.Server, Port: c.Port,
			Settings: s, TLS: c.TLS, Transport: c.Transport,
		})
	}
	return out
}

func mustMarshal(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

type testSettings interface {
	getBaseSettings() *BaseTestRunnerSettings
}

func (c *LatencyTestRunnerSettings) getBaseSettings() *BaseTestRunnerSettings {
	return &c.BaseTestRunnerSettings
}

func (c *SpeedTestRunnerSettings) getBaseSettings() *BaseTestRunnerSettings {
	return &c.BaseTestRunnerSettings
}

func sortTestResults[T any](results []T, isSuccess func(T) bool, shouldSwap func(T, T) bool) {
	if len(results) == 0 {
		return
	}
	sort.Slice(results, func(i, j int) bool {
		s1 := isSuccess(results[i])
		s2 := isSuccess(results[j])
		if s1 && s2 {
			return shouldSwap(results[j], results[i])
		}
		return s1 && !s2
	})
}

func aggregateLatencyResults(results []LatencyTestResult, sortResults bool) *LatencyTestResults {
	successCount := 0
	failureCount := 0
	for _, r := range results {
		if r.Error == nil {
			successCount++
		} else {
			failureCount++
		}
	}
	if sortResults {
		sortTestResults(results,
			func(r LatencyTestResult) bool { return r.Error == nil },
			func(r1, r2 LatencyTestResult) bool { return r1.Delay > r2.Delay })
	}
	return &LatencyTestResults{
		BaseTestResults: BaseTestResults{
			SuccessCount: successCount, FailureCount: failureCount,
		},
		Results: results,
	}
}

func aggregateSpeedResults(results []SpeedTestResult, sortResults bool) *SpeedTestResults {
	successCount := 0
	failureCount := 0
	for _, r := range results {
		if r.Error == nil {
			successCount++
		} else {
			failureCount++
		}
	}
	if sortResults {
		sortTestResults(results,
			func(r SpeedTestResult) bool { return r.Error == nil },
			func(r1, r2 SpeedTestResult) bool { return r1.Speed < r2.Speed })
	}
	return &SpeedTestResults{
		BaseTestResults: BaseTestResults{
			SuccessCount: successCount, FailureCount: failureCount,
		},
		Results: results,
	}
}
