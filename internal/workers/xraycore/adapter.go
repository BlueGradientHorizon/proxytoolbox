package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/xtls/xray-core/infra/conf"
)

type Adapter struct{}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) ConvertOutbound(config *core.OutboundConfig) (any, error) {
	if config == nil {
		return nil, errors.New("ConvertOutbound: nil config")
	}

	outboundConf := &conf.OutboundDetourConfig{
		Tag: config.Tag,
	}

	switch s := config.Settings.(type) {
	case core.VLESSSettings:
		outboundConf.Protocol = "vless"
		settings := &conf.VLessOutboundConfig{
			Address:    parseAddr(config.Server),
			Port:       uint16(config.Port),
			Id:         s.UUID,
			Flow:       s.Flow,
			Encryption: "none", // TODO query param should be used instead
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.VMessSettings:
		outboundConf.Protocol = "vmess"
		settings := &conf.VMessOutboundConfig{
			Address:  parseAddr(config.Server),
			Port:     uint16(config.Port),
			ID:       s.UUID,
			Security: s.Security,
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.TrojanSettings:
		outboundConf.Protocol = "trojan"
		settings := &conf.TrojanClientConfig{
			Address:  parseAddr(config.Server),
			Port:     uint16(config.Port),
			Password: s.Password,
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.ShadowsocksSettings:
		outboundConf.Protocol = "shadowsocks"
		settings := &conf.ShadowsocksClientConfig{
			Address:  parseAddr(config.Server),
			Port:     uint16(config.Port),
			Cipher:   s.Method,
			Password: s.Password,
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.Hysteria2Settings:
		outboundConf.Protocol = "hysteria"
		// i need to pass s.Password to HysteriaConfig in buildStreamConfig
		settings := &conf.HysteriaClientConfig{
			Version: 2,
			Address: parseAddr(config.Server),
			Port:    uint16(config.Port),
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.WireguardSettings:
		outboundConf.Protocol = "wireguard"
		peers := make([]*conf.WireGuardPeerConfig, len(s.Peers))
		for i, p := range s.Peers {
			peers[i] = &conf.WireGuardPeerConfig{
				PublicKey: p.PublicKey,
				Endpoint:  p.Endpoint,
			}
		}
		settings := &conf.WireGuardConfig{
			IsClient:  true,
			SecretKey: s.SecretKey,
			Address:   s.Address,
			Peers:     peers,
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.SocksSettings:
		outboundConf.Protocol = "socks"
		settings := &conf.SocksClientConfig{
			Address:  parseAddr(config.Server),
			Port:     uint16(config.Port),
			Username: s.User,
			Password: s.Pass,
		}
		settingsJSON, _ := json.Marshal(settings)
		outboundConf.Settings = (*json.RawMessage)(&settingsJSON)

	case core.HTTPSettings:
		outboundConf.Protocol = "http"
		settings := &conf.HTTPClientConfig{
			Address:  parseAddr(config.Server),
			Port:     uint16(config.Port),
			Username: s.User,
			Password: s.Pass,
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

func (a *Adapter) buildStreamConfig(config *core.OutboundConfig) (*conf.StreamConfig, error) {
	streamConfig := &conf.StreamConfig{}

	// Set network protocol
	if config.Transport != nil {
		streamConfig.Network = (*conf.TransportProtocol)(&config.Transport.Type)
	}

	// Build transport settings inline
	if config.Transport == nil {
		streamConfig.TCPSettings = &conf.TCPConfig{}
	} else {
		switch config.Transport.Type {
		case "tcp", "raw", "":
			streamConfig.TCPSettings = &conf.TCPConfig{}
		case "ws", "websocket":
			cfg := &conf.WebSocketConfig{}
			if config.Transport.WebSocket != nil {
				cfg.Path = config.Transport.WebSocket.Path
				cfg.Host = config.Transport.WebSocket.Host
			}
			streamConfig.WSSettings = cfg
		case "grpc":
			cfg := &conf.GRPCConfig{}
			if config.Transport.GRPC != nil {
				cfg.ServiceName = config.Transport.GRPC.ServiceName
			}
			streamConfig.GRPCSettings = cfg
		case "httpupgrade":
			cfg := &conf.HttpUpgradeConfig{}
			if config.Transport.HTTPUpgrade != nil {
				cfg.Path = config.Transport.HTTPUpgrade.Path
				cfg.Host = config.Transport.HTTPUpgrade.Host
			}
			streamConfig.HTTPUPGRADESettings = cfg
		case "splithttp", "xhttp":
			cfg := &conf.SplitHTTPConfig{}
			if config.Transport.SplitHTTP != nil {
				cfg.Host = config.Transport.SplitHTTP.Host
				cfg.Path = config.Transport.SplitHTTP.Path
			}
			if config.Transport.XHTTP != nil {
				cfg.Host = config.Transport.XHTTP.Host
				cfg.Path = config.Transport.XHTTP.Path
				cfg.Mode = config.Transport.XHTTP.Mode
			}
			streamConfig.SplitHTTPSettings = cfg
		case "kcp", "mkcp":
			streamConfig.KCPSettings = &conf.KCPConfig{}
		case "hysteria":
			s, ok := config.Settings.(core.Hysteria2Settings)
			if !ok {
				return nil, fmt.Errorf("not a hysteria2 settings instance")
			}
			cfg := &conf.HysteriaConfig{
				Version: 2,
				Auth:    s.Password,
			}
			streamConfig.HysteriaSettings = cfg
		default:
			return nil, fmt.Errorf("unsupported transport type %s", config.Transport.Type)
		}
	}

	// Build security settings using conf structs for proper REALITY initialization
	if config.TLS != nil && config.TLS.Enabled {
		if config.TLS.Reality != nil {
			streamConfig.Security = "reality"
			streamConfig.REALITYSettings = &conf.REALITYConfig{
				ServerName:  config.TLS.ServerName,
				PublicKey:   config.TLS.Reality.PublicKey,
				ShortId:     config.TLS.Reality.ShortID,
				Fingerprint: config.TLS.Fingerprint,
				SpiderX:     config.TLS.Reality.SpiderX,
			}
		} else {
			streamConfig.Security = "tls"
			streamConfig.TLSSettings = &conf.TLSConfig{
				ServerName:    config.TLS.ServerName,
				AllowInsecure: config.TLS.Insecure,
				Fingerprint:   config.TLS.Fingerprint,
			}
			if len(config.TLS.ALPN) > 0 {
				streamConfig.TLSSettings.ALPN = (*conf.StringList)(&config.TLS.ALPN)
			}
			if config.TLS.ECH != nil && len(config.TLS.ECH.Config) > 0 {
				streamConfig.TLSSettings.ECHConfigList = config.TLS.ECH.Config[0]
			}
		}
	}

	return streamConfig, nil
}
