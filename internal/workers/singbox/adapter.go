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
		return nil, errors.New("nil config")
	}

	outbound := &option.Outbound{
		Tag:  config.Tag,
		Type: config.Type,
	}

	switch settings := config.Settings.(type) {
	case core.VLESSSettings:
		outbound.Options = &option.VLESSOutboundOptions{
			ServerOptions: option.ServerOptions{
				Server:     config.Server,
				ServerPort: config.Port,
			},
			UUID: settings.UUID,
			Flow: settings.Flow,
		}

	case core.TrojanSettings:
		outbound.Options = &option.TrojanOutboundOptions{
			ServerOptions: option.ServerOptions{
				Server:     config.Server,
				ServerPort: config.Port,
			},
			Password: settings.Password,
		}

	case core.VMessSettings:
		outbound.Options = &option.VMessOutboundOptions{
			ServerOptions: option.ServerOptions{
				Server:     config.Server,
				ServerPort: config.Port,
			},
			UUID:     settings.UUID,
			Security: settings.Security,
			AlterId:  settings.AlterID,
		}

	case core.ShadowsocksSettings:
		outbound.Options = &option.ShadowsocksOutboundOptions{
			ServerOptions: option.ServerOptions{
				Server:     config.Server,
				ServerPort: config.Port,
			},
			Method:   settings.Method,
			Password: settings.Password,
		}

	case core.Hysteria2Settings:
		s, ok := config.Settings.(core.Hysteria2Settings)
		if !ok {
			return nil, fmt.Errorf("not a hysteria2 settings instance")
		}
		var obfs *option.Hysteria2Obfs
		if s.Obfs != nil {
			obfs = &option.Hysteria2Obfs{
				Type:     s.Obfs.Type,
				Password: s.Obfs.Password,
			}
		}
		outbound.Options = &option.Hysteria2OutboundOptions{
			ServerOptions: option.ServerOptions{
				Server:     config.Server,
				ServerPort: config.Port,
			},
			Password: settings.Password,
			Obfs:     obfs,
		}

	default:
		return nil, fmt.Errorf("unsupported protocol %s", config.Type)
	}

	if err := a.buildOptionOutbound(config, outbound); err != nil {
		return nil, err
	}

	return outbound, nil
}

func (a *Adapter) buildOptionOutbound(config *core.OutboundConfig, outbound *option.Outbound) error {
	var transport *option.V2RayTransportOptions
	if config.Transport != nil {
		transport = &option.V2RayTransportOptions{}
		switch config.Transport.Type {
		case "tcp", "":
			transport = nil
		case "http":
			transport.Type = C.V2RayTransportTypeHTTP
			if config.Transport.HTTP != nil {
				transport.HTTPOptions = option.V2RayHTTPOptions{
					Host:   config.Transport.HTTP.Host,
					Path:   config.Transport.HTTP.Path,
					Method: config.Transport.HTTP.Method,
				}
			}
		case "ws":
			transport.Type = C.V2RayTransportTypeWebsocket
			if config.Transport.WebSocket != nil {
				transport.WebsocketOptions = option.V2RayWebsocketOptions{
					Path: config.Transport.WebSocket.Path,
				}
				if config.Transport.WebSocket.Host != "" {
					transport.WebsocketOptions.Headers = badoption.HTTPHeader{
						"Host": badoption.Listable[string]{config.Transport.WebSocket.Host},
					}
				}
			}
		case "quic":
			transport.Type = C.V2RayTransportTypeQUIC
			transport.QUICOptions = option.V2RayQUICOptions{}
		case "grpc":
			transport.Type = C.V2RayTransportTypeGRPC
			if config.Transport.GRPC != nil {
				transport.GRPCOptions = option.V2RayGRPCOptions{
					ServiceName: config.Transport.GRPC.ServiceName,
				}
			}
		case "httpupgrade":
			transport.Type = C.V2RayTransportTypeHTTPUpgrade
			if config.Transport.HTTPUpgrade != nil {
				transport.HTTPUpgradeOptions = option.V2RayHTTPUpgradeOptions{
					Host: config.Transport.HTTPUpgrade.Host,
					Path: config.Transport.HTTPUpgrade.Path,
				}
			}
		default:
			return fmt.Errorf("unsupported transport type %s", config.Transport.Type)
		}
	}

	var tls *option.OutboundTLSOptions
	if config.TLS != nil && config.TLS.Enabled {
		tls = &option.OutboundTLSOptions{
			Enabled:    config.TLS.Enabled,
			ServerName: config.TLS.ServerName,
			Insecure:   config.TLS.Insecure,
		}

		if len(config.TLS.ALPN) > 0 {
			tls.ALPN = badoption.Listable[string]{}
			for _, alpn := range config.TLS.ALPN {
				tls.ALPN = append(tls.ALPN, alpn)
			}
		}

		if config.TLS.Fingerprint != "" {
			tls.UTLS = &option.OutboundUTLSOptions{
				Enabled:     true,
				Fingerprint: config.TLS.Fingerprint,
			}
		}

		if config.TLS.Reality != nil {
			tls.Reality = &option.OutboundRealityOptions{
				Enabled:   true,
				PublicKey: config.TLS.Reality.PublicKey,
				ShortID:   config.TLS.Reality.ShortID,
			}
			if tls.UTLS == nil {
				tls.UTLS = &option.OutboundUTLSOptions{
					Enabled:     true,
					Fingerprint: "chrome",
				}
			}
		}

		if config.TLS.ECH != nil {
			tls.ECH = &option.OutboundECHOptions{
				Enabled: true,
				Config:  config.TLS.ECH.Config,
			}
		}
	}

	switch opts := outbound.Options.(type) {
	case *option.VLESSOutboundOptions:
		opts.TLS = tls
		opts.Transport = transport
	case *option.TrojanOutboundOptions:
		opts.TLS = tls
		opts.Transport = transport
	case *option.VMessOutboundOptions:
		opts.TLS = tls
		opts.Transport = transport
	case *option.ShadowsocksOutboundOptions:
	case *option.Hysteria2OutboundOptions:
		opts.TLS = tls

	default:
		return fmt.Errorf("unsupported protocol %s", config.Type)
	}

	return nil
}
