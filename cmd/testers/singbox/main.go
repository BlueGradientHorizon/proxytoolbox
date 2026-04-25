package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/pkg/ipcprotocol"
	"github.com/bluegradienthorizon/proxytoolbox/pkg/testerframework"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
	"github.com/bluegradienthorizon/proxytoolbox/utils"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/metadata"
)

type sbTester struct{}

func (t *sbTester) Info() ipcprotocol.CoreInfo {
	return ipcprotocol.CoreInfo{
		Name:    "sing-box",
		Version: utils.GetModuleVersion("github.com/sagernet/sing-box"),
	}
}

func (t *sbTester) Validate(ctx context.Context, configs []*core.OutboundConfig, sendResult func(ipcprotocol.Response)) error {
	instance, validationErrors, _, err := createBox(ctx, configs)
	if instance != nil {
		instance.Close()
	}

	var parts []string
	for errStr, count := range validationErrors {
		parts = append(parts, fmt.Sprintf("%d x %s", count, errStr))
	}
	if err != nil {
		parts = append(parts, err.Error())
	}

	sendResult(ipcprotocol.Response{
		Type:             "validation",
		ValidationErrors: validationErrors,
		Error:            strings.Join(parts, "; "),
	})
	return nil
}

func (t *sbTester) TestLatency(ctx context.Context, configs []*core.OutboundConfig, settings ipcprotocol.LatencySettings, sendResult func(ipcprotocol.Response)) error {
	instance, _, validConfigs, err := createBox(ctx, configs)

	validMap := make(map[string]struct{})
	for _, cfg := range validConfigs {
		validMap[cfg.Tag] = struct{}{}
	}

	for _, cfg := range configs {
		if _, ok := validMap[cfg.Tag]; !ok {
			sendResult(ipcprotocol.Response{Type: "result", Tag: cfg.Tag, Error: "validation failed"})
		}
	}

	if err != nil {
		if len(validConfigs) > 0 {
			for _, cfg := range validConfigs {
				sendResult(ipcprotocol.Response{Type: "result", Tag: cfg.Tag, Error: err.Error()})
			}
		}
		return nil
	}

	if len(validConfigs) == 0 {
		return nil
	}

	if err := instance.Start(); err != nil {
		return err
	}
	defer instance.Close()

	sbOuts := instance.Outbound().Outbounds()
	proxies := make([]testers.ProxyInfo, 0, len(sbOuts))
	dialers := make([]testers.DialerFunc, 0, len(sbOuts))

	for _, sbOut := range sbOuts {
		tag := sbOut.Tag()
		proxies = append(proxies, testers.ProxyInfo{Tag: tag, Type: sbOut.Type()})
		o := sbOut
		dialers = append(dialers, func(ctx context.Context, network, addr string) (net.Conn, error) {
			return o.DialContext(ctx, network, metadata.ParseSocksaddr(addr))
		})
	}

	timeout := time.Duration(settings.TimeoutMs) * time.Millisecond
	lt, err := testers.NewLatencyTest(ctx, testers.LatencyTestSettings{
		TestURL: settings.TestURL,
		Timeout: timeout,
	}, proxies, dialers, CreateTLSConfigProvider())
	if err != nil {
		return err
	}

	ch := make(chan testers.LatencyTestResult, len(proxies))
	wait := lt.Run(ch)
	for range proxies {
		r := <-ch
		resp := ipcprotocol.Response{Type: "result", Tag: r.Tag, LatencyMs: r.Delay}
		if r.Error != nil {
			resp.Error = r.Error.Error()
		}
		sendResult(resp)
	}
	wait()
	return nil
}

func (t *sbTester) TestSpeed(ctx context.Context, configs []*core.OutboundConfig, settings ipcprotocol.SpeedSettings, sendResult func(ipcprotocol.Response)) error {
	instance, _, validConfigs, err := createBox(ctx, configs)

	validMap := make(map[string]struct{})
	for _, cfg := range validConfigs {
		validMap[cfg.Tag] = struct{}{}
	}

	for _, cfg := range configs {
		if _, ok := validMap[cfg.Tag]; !ok {
			sendResult(ipcprotocol.Response{Type: "result", Tag: cfg.Tag, Error: "validation failed"})
		}
	}

	if err != nil {
		if len(validConfigs) > 0 {
			for _, cfg := range validConfigs {
				sendResult(ipcprotocol.Response{Type: "result", Tag: cfg.Tag, Error: err.Error()})
			}
		}
		return nil
	}

	if len(validConfigs) == 0 {
		return nil
	}

	if err := instance.Start(); err != nil {
		return err
	}
	defer instance.Close()

	sbOuts := instance.Outbound().Outbounds()
	proxies := make([]testers.ProxyInfo, 0, len(sbOuts))
	dialers := make([]testers.DialerFunc, 0, len(sbOuts))

	for _, sbOut := range sbOuts {
		tag := sbOut.Tag()
		proxies = append(proxies, testers.ProxyInfo{Tag: tag, Type: sbOut.Type()})
		o := sbOut
		dialers = append(dialers, func(ctx context.Context, network, addr string) (net.Conn, error) {
			return o.DialContext(ctx, network, metadata.ParseSocksaddr(addr))
		})
	}

	mode := testers.Download
	if settings.Mode == "upload" {
		mode = testers.Upload
	}
	timeout := time.Duration(settings.TimeoutMs) * time.Millisecond

	stSettings := testers.SpeedTestSettings{
		Mode:        mode,
		Provider:    testers.CloudflareProvider,
		Timeout:     timeout,
		TargetBytes: settings.TargetBytes,
	}
	st, err := testers.NewSpeedTest(ctx, stSettings, proxies, dialers, CreateTLSConfigProvider())
	if err != nil {
		return err
	}

	ch := make(chan testers.SpeedTestResult, len(proxies))
	wait := st.Run(ch)
	for range proxies {
		r := <-ch
		resp := ipcprotocol.Response{Type: "result", Tag: r.Tag, Speed: r.Speed}
		if r.Error != nil {
			resp.Error = r.Error.Error()
		}
		sendResult(resp)
	}
	wait()
	return nil
}

func createBox(ctx context.Context, configs []*core.OutboundConfig) (*box.Box, map[string]int, []*core.OutboundConfig, error) {
	adapter := NewAdapter()
	validationErrors := make(map[string]int)
	var validOutbounds []option.Outbound
	var validConfigs []*core.OutboundConfig

	for _, cfg := range configs {
		sbOut, err := adapter.ConvertOutbound(cfg)
		if err != nil {
			validationErrors[cfg.Type+": "+err.Error()]++
			continue
		}
		testCtx := include.Context(ctx)
		tmp, err := box.New(box.Options{
			Context: testCtx,
			Options: option.Options{Outbounds: []option.Outbound{*sbOut}},
		})
		if err != nil {
			validationErrors[cfg.Type+": "+err.Error()]++
			continue
		}
		tmp.Close()
		validOutbounds = append(validOutbounds, *sbOut)
		validConfigs = append(validConfigs, cfg)
	}

	if len(validOutbounds) == 0 {
		return nil, validationErrors, nil, fmt.Errorf("no valid configs")
	}

	opts := option.Options{
		Log:       &option.LogOptions{Level: "panic", Timestamp: true},
		Outbounds: validOutbounds,
	}
	instanceCtx := include.Context(ctx)
	instance, err := box.New(box.Options{Context: instanceCtx, Options: opts})
	if err != nil {
		return nil, validationErrors, validConfigs, err
	}
	return instance, validationErrors, validConfigs, nil
}

func main() {
	testerframework.Run(&sbTester{})
}
