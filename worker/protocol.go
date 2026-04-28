package worker

import (
	"encoding/json"
	"fmt"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

type TestType string

const (
	LatencyTest TestType = "latency"
	SpeedTest   TestType = "speed"
)

type RequestType string

const (
	RequestTypeValidate RequestType = "validate"
	RequestTypeTest     RequestType = "test"
)

type ResponseType string

const (
	ResponseTypeValidation ResponseType = "validation"
	ResponseTypeResult     ResponseType = "result"
	ResponseTypeError      ResponseType = "error"
	ResponseTypeBusy       ResponseType = "busy"
	ResponseTypeDone       ResponseType = "done"
)

// Request is the generic IPC request that replaces TestRequest.
// Type can be "validate" or "test".
// For "test", TestType and Settings must be set.
type Request struct {
	Type     RequestType     `json:"type"`
	TestType TestType        `json:"test_type,omitempty"`
	Configs  []*RawConfig    `json:"configs"`
	Tags     []string        `json:"tags,omitempty"`
	Settings json.RawMessage `json:"settings,omitempty"`
}

// RawConfig mirrors core.OutboundConfig but keeps Settings as RawMessage
// so the worker can unmarshal it into the correct concrete type.
type RawConfig struct {
	Tag       string                `json:"tag"`
	Type      string                `json:"type"`
	Server    string                `json:"server"`
	Port      uint16                `json:"port"`
	Settings  json.RawMessage       `json:"settings"`
	TLS       *core.TLSConfig       `json:"tls,omitempty"`
	Transport *core.TransportConfig `json:"transport,omitempty"`
}

func (rc *RawConfig) ToCore() (*core.OutboundConfig, error) {
	cfg := &core.OutboundConfig{
		Tag:       rc.Tag,
		Type:      rc.Type,
		Server:    rc.Server,
		Port:      rc.Port,
		TLS:       rc.TLS,
		Transport: rc.Transport,
	}
	switch rc.Type {
	case "vless":
		var s core.VLESSSettings
		if err := json.Unmarshal(rc.Settings, &s); err != nil {
			return nil, err
		}
		cfg.Settings = s
	case "trojan":
		var s core.TrojanSettings
		if err := json.Unmarshal(rc.Settings, &s); err != nil {
			return nil, err
		}
		cfg.Settings = s
	case "vmess":
		var s core.VMessSettings
		if err := json.Unmarshal(rc.Settings, &s); err != nil {
			return nil, err
		}
		cfg.Settings = s
	case "shadowsocks", "ss":
		var s core.ShadowsocksSettings
		if err := json.Unmarshal(rc.Settings, &s); err != nil {
			return nil, err
		}
		cfg.Settings = s
	case "hysteria2", "hy2":
		var s core.Hysteria2Settings
		if err := json.Unmarshal(rc.Settings, &s); err != nil {
			return nil, err
		}
		cfg.Settings = s
	default:
		return nil, fmt.Errorf("unknown type %s", rc.Type)
	}
	return cfg, nil
}

type ValidationError struct {
	Tag   string `json:"tag"`
	Error string `json:"error"`
}

// Response is streamed back to the library (one JSON value per line).
type Response struct {
	Type             ResponseType      `json:"type"`
	ValidationErrors []ValidationError `json:"validation_errors,omitempty"`
	Tag              string            `json:"tag,omitempty"`
	Error            string            `json:"error,omitempty"`
	LatencyMs        int64             `json:"latency_ms,omitempty"`
	Speed            float64           `json:"speed,omitempty"`
}

type CoreInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type LatencySettings struct {
	TimeoutMs   int    `json:"timeout_ms"`
	TestURL     string `json:"test_url"`
	Concurrency int    `json:"concurrency"`
}

type SpeedSettings struct {
	Mode        string `json:"mode"` // "download" | "upload"
	TimeoutMs   int    `json:"timeout_ms"`
	TargetBytes int64  `json:"target_bytes"`
	Concurrency int    `json:"concurrency"`
}
