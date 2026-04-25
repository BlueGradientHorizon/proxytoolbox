// Package singbox provides an adapter for converting core-agnostic configurations
// to sing-box-specific types. This adapter allows the library to use sing-box as
// a proxy core implementation.
package singbox

import (
	"errors"
	"fmt"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// Adapter converts generic configurations to sing-box types.
// It implements the core.ConfigConverter interface.
type Adapter struct{}

// NewAdapter creates a new sing-box adapter instance.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// ConvertOutbound converts a generic OutboundConfig to a sing-box option.Outbound.
// This method handles all supported protocol types: VLESS, Trojan, VMess, Shadowsocks, and Hysteria2.
// It preserves all configuration details including TLS and transport settings.
func (a *Adapter) ConvertOutbound(config *core.OutboundConfig) (*option.Outbound, error) {
	if config == nil {
		return nil, errors.New("ConvertOutbound: nil config")
	}

	outbound := &option.Outbound{
		Tag:  config.Tag,
		Type: config.Type,
	}

	// Convert protocol-specific settings
	switch settings := config.Settings.(type) {
	case core.VLESSSettings:
		vlessOptions := option.VLESSOutboundOptions{
			ServerOptions: option.ServerOptions{
				Server:     config.Server,
				ServerPort: config.Port,
			},
			UUID: settings.UUID,
			Flow: settings.Flow,
		}
		if config.TLS != nil {
			tls, err := a.convertTLS(config.TLS)
			if err != nil {
				return nil, err
			}
			vlessOptions.OutboundTLSOptionsContainer.TLS = tls
		}
		if config.Transport != nil {
			transport, err := a.convertTransport(config.Transport)
			if err != nil {
				return nil, err
			}
			vlessOptions.Transport = transport
		}
		outbound.Options = &vlessOptions

	case core.TrojanSettings:
		trojanOptions := option.TrojanOutboundOptions{
			ServerOptions: option.ServerOptions{
				Server:     config.Server,
				ServerPort: config.Port,
			},
			Password: settings.Password,
		}
		if config.TLS != nil {
			tls, err := a.convertTLS(config.TLS)
			if err != nil {
				return nil, err
			}
			trojanOptions.OutboundTLSOptionsContainer.TLS = tls
		}
		if config.Transport != nil {
			transport, err := a.convertTransport(config.Transport)
			if err != nil {
				return nil, err
			}
			trojanOptions.Transport = transport
		}
		outbound.Options = &trojanOptions

	case core.VMessSettings:
		vmessOptions := option.VMessOutboundOptions{
			ServerOptions: option.ServerOptions{
				Server:     config.Server,
				ServerPort: config.Port,
			},
			UUID:     settings.UUID,
			Security: settings.Security,
			AlterId:  settings.AlterID,
		}
		if config.TLS != nil {
			tls, err := a.convertTLS(config.TLS)
			if err != nil {
				return nil, err
			}
			vmessOptions.OutboundTLSOptionsContainer.TLS = tls
		}
		if config.Transport != nil {
			transport, err := a.convertTransport(config.Transport)
			if err != nil {
				return nil, err
			}
			vmessOptions.Transport = transport
		}
		outbound.Options = &vmessOptions

	case core.ShadowsocksSettings:
		ssOptions := option.ShadowsocksOutboundOptions{
			ServerOptions: option.ServerOptions{
				Server:     config.Server,
				ServerPort: config.Port,
			},
			Method:   settings.Method,
			Password: settings.Password,
		}
		outbound.Options = &ssOptions

	case core.Hysteria2Settings:
		hy2Options := option.Hysteria2OutboundOptions{
			ServerOptions: option.ServerOptions{
				Server:     config.Server,
				ServerPort: config.Port,
			},
			Password: settings.Password,
		}
		if settings.Obfs != nil {
			hy2Options.Obfs = &option.Hysteria2Obfs{
				Type:     settings.Obfs.Type,
				Password: settings.Obfs.Password,
			}
		}
		if config.TLS != nil {
			tls, err := a.convertTLS(config.TLS)
			if err != nil {
				return nil, err
			}
			hy2Options.OutboundTLSOptionsContainer.TLS = tls
		}
		outbound.Options = &hy2Options

	default:
		return nil, fmt.Errorf("ConvertOutbound: unsupported protocol settings type")
	}

	return outbound, nil
}

// convertTLS converts a generic TLSConfig to sing-box OutboundTLSOptions.
// It handles TLS, Reality, ECH, and uTLS fingerprinting configurations.
func (a *Adapter) convertTLS(config *core.TLSConfig) (*option.OutboundTLSOptions, error) {
	if config == nil || !config.Enabled {
		return nil, nil
	}

	tls := &option.OutboundTLSOptions{
		Enabled:    config.Enabled,
		ServerName: config.ServerName,
		Insecure:   config.Insecure,
	}

	// Convert ALPN list
	if len(config.ALPN) > 0 {
		tls.ALPN = badoption.Listable[string]{}
		for _, alpn := range config.ALPN {
			tls.ALPN = append(tls.ALPN, alpn)
		}
	}

	// Convert uTLS fingerprint
	if config.Fingerprint != "" {
		tls.UTLS = &option.OutboundUTLSOptions{
			Enabled:     true,
			Fingerprint: config.Fingerprint,
		}
	}

	// Convert Reality configuration
	if config.Reality != nil {
		tls.Reality = &option.OutboundRealityOptions{
			Enabled:   true,
			PublicKey: config.Reality.PublicKey,
			ShortID:   config.Reality.ShortID,
		}
		// Reality requires UTLS
		if tls.UTLS == nil {
			tls.UTLS = &option.OutboundUTLSOptions{
				Enabled:     true,
				Fingerprint: "chrome",
			}
		}
	}

	// Convert ECH configuration
	if config.ECH != nil {
		tls.ECH = &option.OutboundECHOptions{
			Enabled: true,
			Config:  config.ECH.Config,
		}
	}

	return tls, nil
}

// convertTransport converts a generic TransportConfig to sing-box V2RayTransportOptions.
// It handles all supported transport types: TCP, HTTP/2, WebSocket, QUIC, gRPC, and HTTP Upgrade.
func (a *Adapter) convertTransport(config *core.TransportConfig) (*option.V2RayTransportOptions, error) {
	if config == nil {
		return nil, nil
	}

	transport := &option.V2RayTransportOptions{}

	switch config.Type {
	case "tcp", "":
		// No transport options needed for raw TCP
		return nil, nil

	case "http":
		transport.Type = C.V2RayTransportTypeHTTP
		if config.HTTP != nil {
			transport.HTTPOptions = option.V2RayHTTPOptions{
				Host:   config.HTTP.Host,
				Path:   config.HTTP.Path,
				Method: config.HTTP.Method,
			}
		}

	case "ws":
		transport.Type = C.V2RayTransportTypeWebsocket
		if config.WebSocket != nil {
			transport.WebsocketOptions = option.V2RayWebsocketOptions{
				Path: config.WebSocket.Path,
			}
		}

	case "quic":
		transport.Type = C.V2RayTransportTypeQUIC
		transport.QUICOptions = option.V2RayQUICOptions{}

	case "grpc":
		transport.Type = C.V2RayTransportTypeGRPC
		if config.GRPC != nil {
			transport.GRPCOptions = option.V2RayGRPCOptions{
				ServiceName: config.GRPC.ServiceName,
			}
		}

	case "httpupgrade":
		transport.Type = C.V2RayTransportTypeHTTPUpgrade
		if config.HTTPUpgrade != nil {
			transport.HTTPUpgradeOptions = option.V2RayHTTPUpgradeOptions{
				Host: config.HTTPUpgrade.Host,
				Path: config.HTTPUpgrade.Path,
			}
		}

	default:
		return nil, fmt.Errorf("convertTransport: unsupported transport type %s", config.Type)
	}

	return transport, nil
}
