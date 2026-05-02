package main

import (
	"context"
	net_std "net"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/internal/workers/utils"
	"github.com/bluegradienthorizon/proxytoolbox/worker"

	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/proxyman"
	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/serial"
	xraycore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/routing"
	"github.com/xtls/xray-core/transport/internet/tagged/taggedimpl"

	_ "github.com/xtls/xray-core/main/distro/all"
)

type xrayAdapter struct {
	proxies []worker.ProxyInfo
}

func (a *xrayAdapter) Info() worker.CoreInfo {
	return worker.CoreInfo{
		Name:    "xray-core",
		Version: utils.GetModuleVersion("github.com/xtls/xray-core"),
	}
}

func (a *xrayAdapter) Convert(config *core.OutboundConfig) (any, error) {
	return NewAdapter().ConvertOutbound(config)
}

func (a *xrayAdapter) ValidateSingle(ctx context.Context, obj any) error {
	ob := obj.(*xraycore.OutboundHandlerConfig)
	config := &xraycore.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
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
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
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
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
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
	worker.Run(worker.NewBaseWorker(&xrayAdapter{}))
}
