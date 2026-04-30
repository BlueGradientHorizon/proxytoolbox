package main

import (
	"context"
	"fmt"
	"net"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/internal/workers/utils"
	"github.com/bluegradienthorizon/proxytoolbox/worker"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/metadata"
)

type singboxAdapter struct{}

func (a *singboxAdapter) Info() worker.CoreInfo {
	return worker.CoreInfo{
		Name:    "sing-box",
		Version: utils.GetModuleVersion("github.com/sagernet/sing-box"),
	}
}

func (a *singboxAdapter) Convert(cfg *core.OutboundConfig) (any, error) {
	out, err := NewAdapter().ConvertOutbound(cfg)
	if err != nil {
		return nil, err
	}
	return *out, nil
}

func (a *singboxAdapter) ValidateSingle(ctx context.Context, obj any) error {
	out := obj.(option.Outbound)
	inst, err := newBoxInstance(ctx, []option.Outbound{out})
	if err != nil {
		return err
	}
	inst.Close()
	return nil
}

func (a *singboxAdapter) ValidateBatch(ctx context.Context, objs []any) error {
	outbounds := make([]option.Outbound, len(objs))
	for i, obj := range objs {
		outbounds[i] = obj.(option.Outbound)
	}
	inst, err := newBoxInstance(ctx, outbounds)
	if inst != nil {
		inst.Close()
	}
	return err
}

func (a *singboxAdapter) CreateInstance(ctx context.Context, converted []any) (any, error) {
	outbounds := make([]option.Outbound, len(converted))
	for i, obj := range converted {
		outbounds[i] = obj.(option.Outbound)
	}
	return newBoxInstance(ctx, outbounds)
}

func (a *singboxAdapter) StartInstance(inst any) error {
	return inst.(*box.Box).Start()
}

func (a *singboxAdapter) ExtractDialers(inst any) ([]worker.ProxyInfo, []worker.DialerFunc, error) {
	b := inst.(*box.Box)
	sbOuts := b.Outbound().Outbounds()
	proxies := make([]worker.ProxyInfo, 0, len(sbOuts))
	dialers := make([]worker.DialerFunc, 0, len(sbOuts))

	for _, sbOut := range sbOuts {
		tag := sbOut.Tag()
		proxies = append(proxies, worker.ProxyInfo{Tag: tag, Type: sbOut.Type()})
		o := sbOut
		dialers = append(dialers, func(ctx context.Context, network, addr string) (net.Conn, error) {
			return o.DialContext(ctx, network, metadata.ParseSocksaddr(addr))
		})
	}

	return proxies, dialers, nil
}

func (a *singboxAdapter) CloseInstance(inst any) {
	if b, ok := inst.(*box.Box); ok {
		b.Close()
	}
}

func (a *singboxAdapter) TLSProvider(ctx context.Context) worker.TLSConfigProvider {
	return CreateTLSConfigProvider()
}

func newBoxInstance(ctx context.Context, outbounds []option.Outbound) (*box.Box, error) {
	if len(outbounds) == 0 {
		return nil, fmt.Errorf("no valid configs")
	}
	opts := option.Options{
		Log:       &option.LogOptions{Disabled: true},
		Outbounds: outbounds,
	}
	instanceCtx := include.Context(ctx)
	return box.New(box.Options{Context: instanceCtx, Options: opts})
}

func main() {
	worker.Run(worker.NewBaseWorker(&singboxAdapter{}))
}
