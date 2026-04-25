package main

import (
	"errors"
	"fmt"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

type Adapter struct{}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) ConvertOutbound(config *core.OutboundConfig) (*option.Outbound, error) {
	if config == nil {
		return nil, errors.New("ConvertOutbound: nil config")
	}

	outbound := &option.Outbound{
		Tag:  config.Tag,
		Type: config.Type,
	}

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

func (a *Adapter) convertTLS(config *core.TLSConfig) (*option.OutboundTLSOptions, error) {
	if config == nil || !config.Enabled {
		return nil, nil
	}

	tls := &option.OutboundTLSOptions{
		Enabled:    config.Enabled,
		ServerName: config.ServerName,
		Insecure:   config.Insecure,
	}

	if len(config.ALPN) > 0 {
		tls.ALPN = badoption.Listable[string]{}
		for _, alpn := range config.ALPN {
			tls.ALPN = append(tls.ALPN, alpn)
		}
	}

	if config.Fingerprint != "" {
		tls.UTLS = &option.OutboundUTLSOptions{
			Enabled:     true,
			Fingerprint: config.Fingerprint,
		}
	}

	if config.Reality != nil {
		tls.Reality = &option.OutboundRealityOptions{
			Enabled:   true,
			PublicKey: config.Reality.PublicKey,
			ShortID:   config.Reality.ShortID,
		}
		if tls.UTLS == nil {
			tls.UTLS = &option.OutboundUTLSOptions{
				Enabled:     true,
				Fingerprint: "chrome",
			}
		}
	}

	if config.ECH != nil {
		tls.ECH = &option.OutboundECHOptions{
			Enabled: true,
			Config:  config.ECH.Config,
		}
	}

	return tls, nil
}

func (a *Adapter) convertTransport(config *core.TransportConfig) (*option.V2RayTransportOptions, error) {
	if config == nil {
		return nil, nil
	}

	transport := &option.V2RayTransportOptions{}

	switch config.Type {
	case "tcp", "":
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
