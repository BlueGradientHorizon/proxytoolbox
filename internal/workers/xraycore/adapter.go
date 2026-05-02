package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"

	"github.com/bluegradienthorizon/proxytoolbox/core"

	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/grpc"
	"github.com/xtls/xray-core/transport/internet/httpupgrade" // NEW
	"github.com/xtls/xray-core/transport/internet/kcp"
	"github.com/xtls/xray-core/transport/internet/reality"
	"github.com/xtls/xray-core/transport/internet/splithttp"
	"github.com/xtls/xray-core/transport/internet/tcp"
	xtls "github.com/xtls/xray-core/transport/internet/tls"
	"github.com/xtls/xray-core/transport/internet/websocket"
	"google.golang.org/protobuf/proto"

	_ "github.com/xtls/xray-core/main/distro/all"
)

func ssCipher(method string) shadowsocks.CipherType {
	switch method {
	case "aes-128-gcm":
		return shadowsocks.CipherType_AES_128_GCM
	case "aes-256-gcm":
		return shadowsocks.CipherType_AES_256_GCM
	case "chacha20-poly1305", "chacha20-ietf-poly1305":
		return shadowsocks.CipherType_CHACHA20_POLY1305
	case "xchacha20-poly1305", "xchacha20-ietf-poly1305":
		return shadowsocks.CipherType_XCHACHA20_POLY1305
	case "none":
		return shadowsocks.CipherType_NONE
	default:
		return shadowsocks.CipherType_UNKNOWN
	}
}

func (a *xrayAdapter) convertTransport(config *core.TransportConfig) (string, *internet.TransportConfig, error) {
	if config == nil {
		return "tcp", nil, nil
	}

	protocolName := "tcp"
	var ts proto.Message

	switch config.Type {
	case "tcp", "raw", "":
		protocolName = "tcp"
		ts = &tcp.Config{}
	case "ws", "websocket":
		protocolName = "websocket"
		wsCfg := &websocket.Config{}
		if config.WebSocket != nil {
			wsCfg.Path = config.WebSocket.Path
			wsCfg.Host = config.WebSocket.Host
		}
		ts = wsCfg
	case "grpc":
		protocolName = "grpc"
		grpcCfg := &grpc.Config{}
		if config.GRPC != nil {
			grpcCfg.ServiceName = config.GRPC.ServiceName
		}
		ts = grpcCfg
	case "httpupgrade":
		protocolName = "httpupgrade"
		huCfg := &httpupgrade.Config{}
		if config.HTTPUpgrade != nil {
			huCfg.Path = config.HTTPUpgrade.Path
			huCfg.Host = config.HTTPUpgrade.Host
		}
		ts = huCfg
	case "splithttp":
		protocolName = "splithttp"
		shCfg := &splithttp.Config{}
		if config.SplitHTTP != nil {
			shCfg.Host = config.SplitHTTP.Host
			shCfg.Path = config.SplitHTTP.Path
		}
		ts = shCfg
	case "xhttp":
		protocolName = "splithttp"
		xhCfg := &splithttp.Config{}
		if config.XHTTP != nil {
			xhCfg.Host = config.XHTTP.Host
			xhCfg.Path = config.XHTTP.Path
			xhCfg.Mode = config.XHTTP.Mode
		}
		ts = xhCfg
	case "kcp", "mkcp":
		protocolName = "mkcp"
		ts = &kcp.Config{
			Mtu:              &kcp.MTU{Value: 1350},
			Tti:              &kcp.TTI{Value: 50},
			UplinkCapacity:   &kcp.UplinkCapacity{Value: 5},
			DownlinkCapacity: &kcp.DownlinkCapacity{Value: 20},
		}
	default:
		return "", nil, fmt.Errorf("unsupported transport type %s", config.Type)
	}

	var transConfig *internet.TransportConfig
	if ts != nil {
		transConfig = &internet.TransportConfig{
			ProtocolName: protocolName,
			Settings:     serial.ToTypedMessage(ts),
		}
	}

	return protocolName, transConfig, nil
}

func (a *xrayAdapter) convertTLS(config *core.TLSConfig) (string, []*serial.TypedMessage, error) {
	if config == nil || !config.Enabled {
		return "", nil, nil
	}

	if config.Reality != nil {
		pbk, _ := base64.RawURLEncoding.DecodeString(config.Reality.PublicKey)
		sid, _ := hex.DecodeString(config.Reality.ShortID)
		secType := serial.GetMessageType(&reality.Config{})
		secSettings := []*serial.TypedMessage{
			serial.ToTypedMessage(&reality.Config{
				ServerName:  config.ServerName,
				PublicKey:   pbk,
				ShortId:     sid,
				Fingerprint: config.Fingerprint,
				SpiderX:     "/",
			}),
		}
		return secType, secSettings, nil
	}

	tlsCfg := &xtls.Config{
		ServerName:    config.ServerName,
		AllowInsecure: config.Insecure,
		NextProtocol:  config.ALPN,
		Fingerprint:   config.Fingerprint,
	}
	if config.ECH != nil && len(config.ECH.Config) > 0 {
		tlsCfg.EchConfigList = config.ECH.Config[0]
	}
	secType := serial.GetMessageType(&xtls.Config{})
	secSettings := []*serial.TypedMessage{
		serial.ToTypedMessage(tlsCfg),
	}
	return secType, secSettings, nil
}
