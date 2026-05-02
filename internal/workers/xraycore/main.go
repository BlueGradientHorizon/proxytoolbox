package main

import (
	"context"
	"fmt"
	net_std "net"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/internal/workers/utils"
	"github.com/bluegradienthorizon/proxytoolbox/worker"

	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/proxyman"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	xraycore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/routing"
	"github.com/xtls/xray-core/proxy/dokodemo"
	httpout "github.com/xtls/xray-core/proxy/http"
	"github.com/xtls/xray-core/proxy/hysteria"
	hyaccount "github.com/xtls/xray-core/proxy/hysteria/account"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/socks"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"
	vlessout "github.com/xtls/xray-core/proxy/vless/outbound"
	"github.com/xtls/xray-core/proxy/vmess"
	vmessout "github.com/xtls/xray-core/proxy/vmess/outbound"
	"github.com/xtls/xray-core/proxy/wireguard"
	"github.com/xtls/xray-core/transport/internet"
	hytransport "github.com/xtls/xray-core/transport/internet/hysteria" // NEW
	"github.com/xtls/xray-core/transport/internet/tagged/taggedimpl"
	"google.golang.org/protobuf/proto"

	_ "github.com/xtls/xray-core/main/distro/all"
)

type xrayAdapter struct {
	proxies []worker.ProxyInfo
}

func NewAdapter() *xrayAdapter {
	return &xrayAdapter{}
}

func (a *xrayAdapter) Info() worker.CoreInfo {
	return worker.CoreInfo{
		Name:    "xray-core",
		Version: utils.GetModuleVersion("github.com/xtls/xray-core"),
	}
}

func (a *xrayAdapter) Convert(config *core.OutboundConfig) (any, error) {
	ob := &xraycore.OutboundHandlerConfig{
		Tag: config.Tag,
	}

	var proxySettings proto.Message
	// streamConfig is built per-protocol; Hysteria2 overrides it entirely.
	streamConfig := &internet.StreamConfig{
		ProtocolName: "tcp",
	}

	switch s := config.Settings.(type) {
	case core.VLESSSettings:
		proxySettings = &vlessout.Config{
			Vnext: &protocol.ServerEndpoint{
				Address: xnet.NewIPOrDomain(xnet.ParseAddress(config.Server)),
				Port:    uint32(config.Port),
				User: &protocol.User{
					Account: serial.ToTypedMessage(&vless.Account{
						Id:   s.UUID,
						Flow: s.Flow,
					}),
				},
			},
		}
	case core.VMessSettings:
		secType := protocol.SecurityType_AUTO
		switch s.Security {
		case "aes-128-gcm":
			secType = protocol.SecurityType_AES128_GCM
		case "chacha20-poly1305":
			secType = protocol.SecurityType_CHACHA20_POLY1305
		case "none":
			secType = protocol.SecurityType_NONE
		case "zero":
			secType = protocol.SecurityType_ZERO
		}
		proxySettings = &vmessout.Config{
			Receiver: &protocol.ServerEndpoint{
				Address: xnet.NewIPOrDomain(xnet.ParseAddress(config.Server)),
				Port:    uint32(config.Port),
				User: &protocol.User{
					Account: serial.ToTypedMessage(&vmess.Account{
						Id: s.UUID,
						SecuritySettings: &protocol.SecurityConfig{
							Type: secType,
						},
					}),
				},
			},
		}
	case core.TrojanSettings:
		proxySettings = &trojan.ClientConfig{
			Server: &protocol.ServerEndpoint{
				Address: xnet.NewIPOrDomain(xnet.ParseAddress(config.Server)),
				Port:    uint32(config.Port),
				User: &protocol.User{
					Account: serial.ToTypedMessage(&trojan.Account{
						Password: s.Password,
					}),
				},
			},
		}
	case core.ShadowsocksSettings:
		proxySettings = &shadowsocks.ClientConfig{
			Server: &protocol.ServerEndpoint{
				Address: xnet.NewIPOrDomain(xnet.ParseAddress(config.Server)),
				Port:    uint32(config.Port),
				User: &protocol.User{
					Account: serial.ToTypedMessage(&shadowsocks.Account{
						Password:   s.Password,
						CipherType: ssCipher(s.Method),
					}),
				},
			},
		}
	case core.Hysteria2Settings:
		proxySettings = &hysteria.ClientConfig{
			Version: 2,
			Server: &protocol.ServerEndpoint{
				Address: xnet.NewIPOrDomain(xnet.ParseAddress(config.Server)),
				Port:    uint32(config.Port),
				User: &protocol.User{
					Account: serial.ToTypedMessage(&hyaccount.Account{
						Auth: s.Password,
					}),
				},
			},
		}
		// FIX: Hysteria2 requires its own stream-level transport config.
		// The proxy-level ClientConfig alone is not enough.
		streamConfig = &internet.StreamConfig{
			ProtocolName: "hysteria",
			TransportSettings: []*internet.TransportConfig{
				{
					ProtocolName: "hysteria",
					Settings: serial.ToTypedMessage(&hytransport.Config{
						Version:        2,
						Auth:           s.Password,
						UdpIdleTimeout: 60,
					}),
				},
			},
		}
	case core.WireguardSettings:
		peers := make([]*wireguard.PeerConfig, len(s.Peers))
		for i, p := range s.Peers {
			peers[i] = &wireguard.PeerConfig{
				PublicKey: p.PublicKey,
				Endpoint:  p.Endpoint,
			}
		}
		proxySettings = &wireguard.DeviceConfig{
			SecretKey: s.SecretKey,
			Endpoint:  s.Address,
			Peers:     peers,
			IsClient:  true,
			Mtu:       1420,
		}
	case core.SocksSettings:
		proxySettings = &socks.ClientConfig{
			Server: &protocol.ServerEndpoint{
				Address: xnet.NewIPOrDomain(xnet.ParseAddress(config.Server)),
				Port:    uint32(config.Port),
				User: &protocol.User{
					Account: serial.ToTypedMessage(&socks.Account{
						Username: s.User,
						Password: s.Pass,
					}),
				},
			},
		}
	case core.HTTPSettings:
		proxySettings = &httpout.ClientConfig{
			Server: &protocol.ServerEndpoint{
				Address: xnet.NewIPOrDomain(xnet.ParseAddress(config.Server)),
				Port:    uint32(config.Port),
				User: &protocol.User{
					Account: serial.ToTypedMessage(&httpout.Account{
						Username: s.User,
						Password: s.Pass,
					}),
				},
			},
		}
	case core.VLiteSettings:
		return nil, fmt.Errorf("vlite is not supported by xray-core")
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
	ob.ProxySettings = serial.ToTypedMessage(proxySettings)

	// Apply transport settings (skip for Hysteria2 which already set streamConfig above).
	if streamConfig.ProtocolName != "hysteria" {
		protName, transConfig, err := a.convertTransport(config.Transport)
		if err != nil {
			return nil, err
		}
		streamConfig.ProtocolName = protName
		if transConfig != nil {
			streamConfig.TransportSettings = []*internet.TransportConfig{transConfig}
		}
	}

	// Apply TLS/Reality security settings.
	secType, secSettings, err := a.convertTLS(config.TLS)
	if err != nil {
		return nil, err
	}
	if secType != "" {
		streamConfig.SecurityType = secType
		streamConfig.SecuritySettings = secSettings
	}

	ob.SenderSettings = serial.ToTypedMessage(&proxyman.SenderConfig{
		StreamSettings: streamConfig,
	})

	return ob, nil
}

func (a *xrayAdapter) ValidateSingle(ctx context.Context, obj any) error {
	ob := obj.(*xraycore.OutboundHandlerConfig)
	config := &xraycore.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&proxyman.InboundConfig{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
		},
		Inbound: []*xraycore.InboundHandlerConfig{
			{
				Tag: "dummy-in",
				ReceiverSettings: serial.ToTypedMessage(&proxyman.ReceiverConfig{
					PortList: &xnet.PortList{Range: []*xnet.PortRange{xnet.SinglePortRange(10000)}},
					Listen:   xnet.NewIPOrDomain(xnet.LocalHostIP),
				}),
				ProxySettings: serial.ToTypedMessage(&dokodemo.Config{
					Address:  xnet.NewIPOrDomain(xnet.LocalHostIP),
					Port:     uint32(0),
					Networks: []xnet.Network{xnet.Network_TCP},
				}),
			},
		},
		Outbound: []*xraycore.OutboundHandlerConfig{ob},
	}
	inst, err := xraycore.NewWithContext(ctx, config)
	if inst != nil {
		inst.Close()
	}
	return err
}

func (a *xrayAdapter) ValidateBatch(ctx context.Context, objs []any) error {
	config := &xraycore.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&proxyman.InboundConfig{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
		},
		Inbound: []*xraycore.InboundHandlerConfig{
			{
				Tag: "dummy-in",
				ReceiverSettings: serial.ToTypedMessage(&proxyman.ReceiverConfig{
					PortList: &xnet.PortList{Range: []*xnet.PortRange{xnet.SinglePortRange(10000)}},
					Listen:   xnet.NewIPOrDomain(xnet.LocalHostIP),
				}),
				ProxySettings: serial.ToTypedMessage(&dokodemo.Config{
					Address:  xnet.NewIPOrDomain(xnet.LocalHostIP),
					Port:     uint32(0),
					Networks: []xnet.Network{xnet.Network_TCP},
				}),
			},
		},
	}
	for _, obj := range objs {
		config.Outbound = append(config.Outbound, obj.(*xraycore.OutboundHandlerConfig))
	}
	inst, err := xraycore.NewWithContext(ctx, config)
	if inst != nil {
		inst.Close()
	}
	return err
}

func (a *xrayAdapter) CreateInstance(ctx context.Context, converted []any) (any, error) {
	config := &xraycore.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&proxyman.InboundConfig{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
		},
		Inbound: []*xraycore.InboundHandlerConfig{
			{
				Tag: "dummy-in",
				ReceiverSettings: serial.ToTypedMessage(&proxyman.ReceiverConfig{
					PortList: &xnet.PortList{Range: []*xnet.PortRange{xnet.SinglePortRange(10000)}},
					Listen:   xnet.NewIPOrDomain(xnet.LocalHostIP),
				}),
				ProxySettings: serial.ToTypedMessage(&dokodemo.Config{
					Address:  xnet.NewIPOrDomain(xnet.LocalHostIP),
					Port:     uint32(0),
					Networks: []xnet.Network{xnet.Network_TCP},
				}),
			},
		},
	}

	a.proxies = make([]worker.ProxyInfo, len(converted))
	for i, obj := range converted {
		ob := obj.(*xraycore.OutboundHandlerConfig)
		config.Outbound = append(config.Outbound, ob)

		typ := "unknown"
		if strings.Contains(ob.ProxySettings.Type, "vless") {
			typ = "vless"
		} else if strings.Contains(ob.ProxySettings.Type, "vmess") {
			typ = "vmess"
		} else if strings.Contains(ob.ProxySettings.Type, "trojan") {
			typ = "trojan"
		} else if strings.Contains(ob.ProxySettings.Type, "shadowsocks") {
			typ = "shadowsocks"
		} else if strings.Contains(ob.ProxySettings.Type, "hysteria") {
			typ = "hysteria2"
		} else if strings.Contains(ob.ProxySettings.Type, "wireguard") {
			typ = "wireguard"
		} else if strings.Contains(ob.ProxySettings.Type, "socks") {
			typ = "socks"
		} else if strings.Contains(ob.ProxySettings.Type, "http") {
			typ = "http"
		}
		a.proxies[i] = worker.ProxyInfo{
			Tag:  ob.Tag,
			Type: typ,
		}
	}

	return xraycore.NewWithContext(ctx, config)
}

func (a *xrayAdapter) StartInstance(inst any) error {
	return inst.(*xraycore.Instance).Start()
}

func (a *xrayAdapter) ExtractDialers(inst any) ([]worker.ProxyInfo, []worker.DialerFunc, error) {
	instance := inst.(*xraycore.Instance)
	disp := instance.GetFeature(routing.DispatcherType()).(routing.Dispatcher)

	dialers := make([]worker.DialerFunc, len(a.proxies))
	for i, proxy := range a.proxies {
		tag := proxy.Tag
		// FIX: DialTaggedOutbound requires the xray *Instance to be present in
		// the context (checked via core.FromContext). The caller's ctx doesn't
		// have it, so we inject it here using the exported XrayKey type.
		dialers[i] = func(ctx context.Context, network, addr string) (net_std.Conn, error) {
			dest, err := xnet.ParseDestination(network + ":" + addr)
			if err != nil {
				return nil, err
			}
			instanceCtx := context.WithValue(ctx, xraycore.XrayKey(1), instance)
			return taggedimpl.DialTaggedOutbound(instanceCtx, disp, dest, tag)
		}
	}
	return a.proxies, dialers, nil
}

func (a *xrayAdapter) CloseInstance(inst any) {
	if instance, ok := inst.(*xraycore.Instance); ok {
		instance.Close()
	}
}

func (a *xrayAdapter) TLSProvider(ctx context.Context) worker.TLSConfigProvider {
	return nil
}

func main() {
	worker.Run(worker.NewBaseWorker(NewAdapter()))
}
