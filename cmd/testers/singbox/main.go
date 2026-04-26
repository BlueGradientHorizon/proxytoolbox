package main

import (
	"context"
	"fmt"
	"net"
	"sync"
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

type sbTester struct {
	mu          sync.Mutex
	configs     []*core.OutboundConfig
	configMap   map[string]*core.OutboundConfig
	outboundMap map[string]option.Outbound
}

func (t *sbTester) Info() ipcprotocol.CoreInfo {
	return ipcprotocol.CoreInfo{
		Name:    "sing-box",
		Version: utils.GetModuleVersion("github.com/sagernet/sing-box"),
	}
}

func (t *sbTester) Validate(ctx context.Context, configs []*core.OutboundConfig, sendResult func(ipcprotocol.Response)) error {
	adapter := NewAdapter()
	var validationErrors []ipcprotocol.ValidationError
	var validOutbounds []option.Outbound
	var validConfigs []*core.OutboundConfig
	outboundMap := make(map[string]option.Outbound)

	for _, cfg := range configs {
		sbOut, err := adapter.ConvertOutbound(cfg)
		if err != nil {
			validationErrors = append(validationErrors, ipcprotocol.ValidationError{
				Tag:   cfg.Tag,
				Error: "convert: " + cfg.Type + ": " + err.Error(),
			})
			continue
		}
		tmp, err := newBoxInstance(ctx, []option.Outbound{*sbOut})
		if err != nil {
			validationErrors = append(validationErrors, ipcprotocol.ValidationError{
				Tag:   cfg.Tag,
				Error: "instantiate: " + cfg.Type + ": " + err.Error(),
			})
			continue
		}
		tmp.Close()

		validOutbounds = append(validOutbounds, *sbOut)
		validConfigs = append(validConfigs, cfg)
		outboundMap[cfg.Tag] = *sbOut
	}

	instance, err := newBoxInstance(ctx, validOutbounds)
	if instance != nil {
		instance.Close()
	}

	if err != nil {
		validationErrors = append(validationErrors, ipcprotocol.ValidationError{
			Tag:   "",
			Error: err.Error(),
		})
	}

	sendResult(ipcprotocol.Response{
		Type:             ipcprotocol.ResponseTypeValidation,
		ValidationErrors: validationErrors,
	})

	// Store valid configs for subsequent test requests
	t.mu.Lock()
	t.configs = validConfigs
	t.configMap = make(map[string]*core.OutboundConfig, len(validConfigs))
	t.outboundMap = outboundMap
	for _, cfg := range validConfigs {
		t.configMap[cfg.Tag] = cfg
	}
	t.mu.Unlock()

	return nil
}

func (t *sbTester) selectOutbounds(tags []string) ([]*core.OutboundConfig, []option.Outbound) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(tags) == 0 {
		out := make([]*core.OutboundConfig, len(t.configs))
		copy(out, t.configs)
		outbounds := make([]option.Outbound, 0, len(t.configs))
		for _, cfg := range t.configs {
			outbounds = append(outbounds, t.outboundMap[cfg.Tag])
		}
		return out, outbounds
	}

	out := make([]*core.OutboundConfig, 0, len(tags))
	outbounds := make([]option.Outbound, 0, len(tags))
	for _, tag := range tags {
		if cfg, ok := t.configMap[tag]; ok {
			out = append(out, cfg)
			outbounds = append(outbounds, t.outboundMap[tag])
		}
	}
	return out, outbounds
}

func (t *sbTester) TestLatency(ctx context.Context, settings ipcprotocol.LatencySettings, tags []string, sendResult func(ipcprotocol.Response)) error {
	configs, outbounds := t.selectOutbounds(tags)

	// Report validation-failed for requested tags that are not present in the stored set
	foundTags := make(map[string]struct{}, len(configs))
	for _, cfg := range configs {
		foundTags[cfg.Tag] = struct{}{}
	}
	for _, tag := range tags {
		if _, ok := foundTags[tag]; !ok {
			sendResult(ipcprotocol.Response{Type: ipcprotocol.ResponseTypeResult, Tag: tag, Error: "validation failed"})
		}
	}

	if len(configs) == 0 {
		return nil
	}

	instance, err := newBoxInstance(ctx, outbounds)
	if err != nil {
		for _, cfg := range configs {
			sendResult(ipcprotocol.Response{Type: ipcprotocol.ResponseTypeResult, Tag: cfg.Tag, Error: err.Error()})
		}
		return nil
	}

	if err := instance.Start(); err != nil {
		instance.Close()
		return err
	}

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
		instance.Close()
		return err
	}

	ch := make(chan testers.LatencyTestResult, len(proxies))
	wait := lt.Run(ch)
	for range proxies {
		r := <-ch
		resp := ipcprotocol.Response{Type: ipcprotocol.ResponseTypeResult, Tag: r.Tag, LatencyMs: r.Delay}
		if r.Error != nil {
			resp.Error = r.Error.Error()
		}
		sendResult(resp)
	}
	wait()

	// Close asynchronously so the "done" response isn't delayed by teardown.
	go func() {
		start := time.Now()
		instance.Close()
		fmt.Printf("instance.Close() took %v\n", time.Since(start))
	}()
	return nil
}

func (t *sbTester) TestSpeed(ctx context.Context, settings ipcprotocol.SpeedSettings, tags []string, sendResult func(ipcprotocol.Response)) error {
	configs, outbounds := t.selectOutbounds(tags)

	// Report validation-failed for requested tags that are not present in the stored set
	foundTags := make(map[string]struct{}, len(configs))
	for _, cfg := range configs {
		foundTags[cfg.Tag] = struct{}{}
	}
	for _, tag := range tags {
		if _, ok := foundTags[tag]; !ok {
			sendResult(ipcprotocol.Response{Type: ipcprotocol.ResponseTypeResult, Tag: tag, Error: "validation failed"})
		}
	}

	if len(configs) == 0 {
		return nil
	}

	instance, err := newBoxInstance(ctx, outbounds)
	if err != nil {
		for _, cfg := range configs {
			sendResult(ipcprotocol.Response{Type: ipcprotocol.ResponseTypeResult, Tag: cfg.Tag, Error: err.Error()})
		}
		return nil
	}

	if err := instance.Start(); err != nil {
		instance.Close()
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
		instance.Close()
		return err
	}

	ch := make(chan testers.SpeedTestResult, len(proxies))
	wait := st.Run(ch)
	for range proxies {
		r := <-ch
		resp := ipcprotocol.Response{Type: ipcprotocol.ResponseTypeResult, Tag: r.Tag, Speed: r.Speed}
		if r.Error != nil {
			resp.Error = r.Error.Error()
		}
		sendResult(resp)
	}
	wait()

	// Close asynchronously so the "done" response isn't delayed by teardown.
	go func() {
		start := time.Now()
		instance.Close()
		fmt.Printf("instance.Close() took %v\n", time.Since(start))
	}()
	return nil
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
	testerframework.Run(&sbTester{})
}
