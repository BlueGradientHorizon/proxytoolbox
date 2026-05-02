package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/xtls/xray-core/infra/conf"
)

type Adapter struct{}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) ConvertOutbound(config *core.OutboundConfig) (any, error) {
	outboundConf := &conf.OutboundDetourConfig{
		Tag: config.Tag,
	}

	switch s := config.Settings.(type) {
	case core.VLESSSettings:
		outboundConf.Protocol = "vless"
		settings := map[string]any{
			"address":    config.Server,
			"port":       config.Port,
			"id":         s.UUID,
			"flow":       s.Flow,
			"encryption": "none", // TODO query param should be used instead
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.VMessSettings:
		outboundConf.Protocol = "vmess"
		settings := map[string]any{
			"address":  config.Server,
			"port":     config.Port,
			"id":       s.UUID,
			"security": s.Security,
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.TrojanSettings:
		outboundConf.Protocol = "trojan"
		settings := map[string]any{
			"address":  config.Server,
			"port":     config.Port,
			"password": s.Password,
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.ShadowsocksSettings:
		outboundConf.Protocol = "shadowsocks"
		cipher := a.mapShadowsocksCipher(s.Method)
		settings := map[string]any{
			"address":  config.Server,
			"port":     config.Port,
			"password": s.Password,
			"cipher":   cipher,
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.Hysteria2Settings:
		outboundConf.Protocol = "hysteria"
		settings := map[string]any{
			"address": config.Server,
			"port":    config.Port,
			"auth":    s.Password,
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.WireguardSettings:
		outboundConf.Protocol = "wireguard"
		settings := map[string]any{
			"isClient":  true,
			"secretKey": s.SecretKey,
			"address":   s.Address,
			"peers":     a.convertWireguardPeersJSON(s.Peers),
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.SocksSettings:
		outboundConf.Protocol = "socks"
		settings := map[string]any{
			"address":  config.Server,
			"port":     config.Port,
			"username": s.User,
			"password": s.Pass,
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.HTTPSettings:
		outboundConf.Protocol = "http"
		settings := map[string]any{
			"address":  config.Server,
			"port":     config.Port,
			"username": s.User,
			"password": s.Pass,
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.VLiteSettings:
		return nil, fmt.Errorf("vlite is not supported by xray-core")

	default:
		return nil, fmt.Errorf("unsupported protocol")
	}

	// Build stream configuration using conf structs
	streamConfig, err := a.buildStreamConfig(config)
	if err != nil {
		return nil, err
	}
	outboundConf.StreamSetting = streamConfig

	// Build the complete outbound configuration
	return outboundConf.Build()
}

// FIX: Add cipher mapping for shadowsocks
func (a *Adapter) mapShadowsocksCipher(method string) string {
	switch strings.ToLower(method) {
	case "aes-128-gcm", "aes_128_gcm":
		return "aes-128-gcm"
	case "aes-256-gcm", "aes_256_gcm":
		return "aes-256-gcm"
	case "chacha20-ietf-poly1305", "chacha20-poly1305", "chacha20-ietf":
		return "chacha20-ietf-poly1305"
	case "xchacha20-ietf-poly1305", "xchacha20-poly1305":
		return "xchacha20-ietf-poly1305"
	case "none":
		return "none"
	default:
		// Default to aes-256-gcm for unknown ciphers
		return "aes-256-gcm"
	}
}

func (a *Adapter) convertWireguardPeersJSON(peers []core.WireguardPeer) []map[string]any {
	result := make([]map[string]any, len(peers))
	for i, p := range peers {
		result[i] = map[string]any{
			"publicKey": p.PublicKey,
			"endpoint":  p.Endpoint,
		}
	}
	return result
}

func (a *Adapter) buildStreamConfig(config *core.OutboundConfig) (*conf.StreamConfig, error) {
	streamConfig := &conf.StreamConfig{}

	// Set network protocol
	if config.Transport != nil {
		streamConfig.Network = (*conf.TransportProtocol)(&config.Transport.Type)
	}

	// Build transport settings
	transportConfig, err := a.buildTransportConfig(config.Transport)
	if err != nil {
		return nil, err
	}

	// Set transport-specific settings
	switch {
	case config.Transport != nil && config.Transport.Type == "websocket":
		streamConfig.WSSettings = transportConfig.(*conf.WebSocketConfig)
	case config.Transport != nil && config.Transport.Type == "grpc":
		streamConfig.GRPCSettings = transportConfig.(*conf.GRPCConfig)
	case config.Transport != nil && config.Transport.Type == "httpupgrade":
		streamConfig.HTTPUPGRADESettings = transportConfig.(*conf.HttpUpgradeConfig)
	case config.Transport != nil && (config.Transport.Type == "splithttp" || config.Transport.Type == "xhttp"):
		streamConfig.SplitHTTPSettings = transportConfig.(*conf.SplitHTTPConfig)
	case config.Transport != nil && (config.Transport.Type == "kcp" || config.Transport.Type == "mkcp"):
		streamConfig.KCPSettings = transportConfig.(*conf.KCPConfig)
	case config.Transport != nil && config.Transport.Type == "hysteria":
		streamConfig.HysteriaSettings = transportConfig.(*conf.HysteriaConfig)
	}

	// Build security settings using conf structs for proper REALITY initialization
	if config.TLS != nil && config.TLS.Enabled {
		if config.TLS.Reality != nil {
			streamConfig.Security = "reality"
			streamConfig.REALITYSettings = a.buildRealityConfig(config.TLS)
		} else {
			streamConfig.Security = "tls"
			streamConfig.TLSSettings = a.buildTLSConfig(config.TLS)
		}
	}

	return streamConfig, nil
}

func (a *Adapter) buildTransportConfig(transport *core.TransportConfig) (conf.Buildable, error) {
	if transport == nil {
		return &conf.TCPConfig{}, nil
	}

	switch transport.Type {
	case "tcp", "raw", "":
		return &conf.TCPConfig{}, nil

	case "ws", "websocket":
		cfg := &conf.WebSocketConfig{}
		if transport.WebSocket != nil {
			cfg.Path = transport.WebSocket.Path
			cfg.Host = transport.WebSocket.Host
		}
		return cfg, nil

	case "grpc":
		cfg := &conf.GRPCConfig{}
		if transport.GRPC != nil {
			cfg.ServiceName = transport.GRPC.ServiceName
		}
		return cfg, nil

	case "httpupgrade":
		cfg := &conf.HttpUpgradeConfig{}
		if transport.HTTPUpgrade != nil {
			cfg.Path = transport.HTTPUpgrade.Path
			cfg.Host = transport.HTTPUpgrade.Host
		}
		return cfg, nil

	case "splithttp", "xhttp":
		cfg := &conf.SplitHTTPConfig{}
		if transport.SplitHTTP != nil {
			cfg.Host = transport.SplitHTTP.Host
			cfg.Path = transport.SplitHTTP.Path
		}
		if transport.XHTTP != nil {
			cfg.Host = transport.XHTTP.Host
			cfg.Path = transport.XHTTP.Path
			cfg.Mode = transport.XHTTP.Mode
		}
		return cfg, nil

	case "kcp", "mkcp":
		return &conf.KCPConfig{}, nil

	case "hysteria":
		cfg := &conf.HysteriaConfig{
			Version: 2,
		}
		return cfg, nil

	default:
		return nil, fmt.Errorf("unsupported transport type %s", transport.Type)
	}
}

func (a *Adapter) buildRealityConfig(tlsConfig *core.TLSConfig) *conf.REALITYConfig {
	cfg := &conf.REALITYConfig{
		ServerName:  tlsConfig.ServerName,
		PublicKey:   tlsConfig.Reality.PublicKey,
		ShortId:     tlsConfig.Reality.ShortID,
		Fingerprint: tlsConfig.Fingerprint,
		SpiderX:     tlsConfig.Reality.SpiderX,
	}
	return cfg
}

func (a *Adapter) buildTLSConfig(tlsConfig *core.TLSConfig) *conf.TLSConfig {
	cfg := &conf.TLSConfig{
		ServerName:    tlsConfig.ServerName,
		AllowInsecure: tlsConfig.Insecure,
		Fingerprint:   tlsConfig.Fingerprint,
	}
	if len(tlsConfig.ALPN) > 0 {
		cfg.ALPN = (*conf.StringList)(&tlsConfig.ALPN)
	}
	if tlsConfig.ECH != nil && len(tlsConfig.ECH.Config) > 0 {
		cfg.ECHConfigList = tlsConfig.ECH.Config[0]
	}
	return cfg
}
