package runner

import (
	"encoding/json"

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

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
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
	for i := 0; i < len(results)-1; i++ {
		for j := 0; j < len(results)-i-1; j++ {
			r1 := results[j]
			r2 := results[j+1]
			s1 := isSuccess(r1)
			s2 := isSuccess(r2)
			if s1 && s2 {
				if shouldSwap(r1, r2) {
					results[j], results[j+1] = results[j+1], results[j]
				}
			} else if !s1 && s2 {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}
}

func aggregateLatencyResults(results []LatencyTestResult, sortResults bool) *LatencyTestResults {
	successCount := 0
	failureCount := 0
	for _, r := range results {
		if r.Error == nil && r.Delay > 0 {
			successCount++
		} else {
			failureCount++
		}
	}
	if sortResults {
		sortTestResults(results,
			func(r LatencyTestResult) bool { return r.Delay > 0 },
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
		if r.Error == nil && r.Speed > 0 {
			successCount++
		} else {
			failureCount++
		}
	}
	if sortResults {
		sortTestResults(results,
			func(r SpeedTestResult) bool { return r.Speed > 0 },
			func(r1, r2 SpeedTestResult) bool { return r1.Speed < r2.Speed })
	}
	return &SpeedTestResults{
		BaseTestResults: BaseTestResults{
			SuccessCount: successCount, FailureCount: failureCount,
		},
		Results: results,
	}
}
