package worker

import (
	"github.com/bluegradienthorizon/proxytoolbox/core"
)

func LatencyTestSettingsToPB(s LatencyTestSettings) *PBLatencyTestSettings {
	return &PBLatencyTestSettings{
		TestUrl:      s.TestURL,
		RawRequest:   s.RawRequest,
		TimeoutNanos: int64(s.Timeout),
		Concurrency:  int32(s.Concurrency),
	}
}

func SpeedTestSettingsToPB(s SpeedTestSettings) *PBSpeedTestSettings {
	var mode PBSpeedTestMode
	switch s.Mode {
	case SpeedTestModeDownload:
		mode = PBSpeedTestMode_SPEED_DOWNLOAD
	case SpeedTestModeUpload:
		mode = PBSpeedTestMode_SPEED_UPLOAD
	}
	return &PBSpeedTestSettings{
		Mode:         mode,
		TestUrl:      s.TestURL,
		RawRequest:   s.RawRequest,
		TimeoutNanos: int64(s.Timeout),
		TargetBytes:  s.TargetBytes,
		Concurrency:  int32(s.Concurrency),
	}
}

func CoreToPBOutboundConfig(c *core.OutboundConfig) *PBOutboundConfig {
	if c == nil {
		return nil
	}
	pb := &PBOutboundConfig{
		Tag:       pbB(c.Tag),
		Type:      pbB(c.Type),
		Server:    pbB(c.Server),
		Port:      uint32(c.Port),
		Tls:       coreTLSToPB(c.TLS),
		Transport: coreTransportToPB(c.Transport),
	}
	setCoreSettingsOnPB(pb, c.Settings)
	return pb
}

func CoreConfigsToPBOutbound(configs []*core.OutboundConfig) []*PBOutboundConfig {
	out := make([]*PBOutboundConfig, 0, len(configs))
	for _, c := range configs {
		out = append(out, CoreToPBOutboundConfig(c))
	}
	return out
}

func setCoreSettingsOnPB(pb *PBOutboundConfig, settings core.ProtocolSettings) {
	if settings == nil {
		return
	}
	switch s := settings.(type) {
	case core.VLESSSettings:
		pb.Settings = &PBOutboundConfig_Vless{
			Vless: &PBVLESSSettings{Uuid: pbB(s.UUID), Flow: pbB(s.Flow), Encryption: pbB(s.Encryption)},
		}
	case core.TrojanSettings:
		pb.Settings = &PBOutboundConfig_Trojan{Trojan: &PBTrojanSettings{Password: pbB(s.Password)}}
	case core.VMessSettings:
		pb.Settings = &PBOutboundConfig_Vmess{
			Vmess: &PBVMessSettings{Uuid: pbB(s.UUID), AlterId: int32(s.AlterID), Security: pbB(s.Security)},
		}
	case core.ShadowsocksSettings:
		pb.Settings = &PBOutboundConfig_Shadowsocks{
			Shadowsocks: &PBShadowsocksSettings{Method: pbB(s.Method), Password: pbB(s.Password)},
		}
	case core.Hysteria2Settings:
		var obfs *PBObfsConfig
		if s.Obfs != nil {
			obfs = &PBObfsConfig{Type: pbB(s.Obfs.Type), Password: pbB(s.Obfs.Password)}
		}
		pb.Settings = &PBOutboundConfig_Hysteria2{
			Hysteria2: &PBHysteria2Settings{Password: pbB(s.Password), Obfs: obfs},
		}
	case core.WireguardSettings:
		peers := make([]*PBWireguardPeer, len(s.Peers))
		for i, p := range s.Peers {
			peers[i] = &PBWireguardPeer{PublicKey: pbB(p.PublicKey), Endpoint: pbB(p.Endpoint)}
		}
		pb.Settings = &PBOutboundConfig_Wireguard{
			Wireguard: &PBWireguardSettings{SecretKey: pbB(s.SecretKey), Address: pbStrings(s.Address), Peers: peers},
		}
	case core.SocksSettings:
		pb.Settings = &PBOutboundConfig_Socks{
			Socks: &PBSocksSettings{Version: pbB(s.Version), User: pbB(s.User), Pass: pbB(s.Pass)},
		}
	case core.HTTPSettings:
		pb.Settings = &PBOutboundConfig_Http{Http: &PBHTTPSettings{User: pbB(s.User), Pass: pbB(s.Pass)}}
	case core.VLiteSettings:
		pb.Settings = &PBOutboundConfig_Vlite{Vlite: &PBVLiteSettings{Password: pbB(s.Password)}}
	}
}

func coreTLSToPB(c *core.TLSConfig) *PBTLSConfig {
	if c == nil {
		return nil
	}
	pb := &PBTLSConfig{
		Enabled:     c.Enabled,
		ServerName:  pbB(c.ServerName),
		Insecure:    c.Insecure,
		Alpn:        pbStrings(c.ALPN),
		Fingerprint: pbB(c.Fingerprint),
	}
	if c.Reality != nil {
		pb.Reality = &PBRealityConfig{
			PublicKey: pbB(c.Reality.PublicKey),
			ShortId:   pbB(c.Reality.ShortID),
			SpiderX:   pbB(c.Reality.SpiderX),
		}
	}
	if c.ECH != nil {
		pb.Ech = &PBECHConfig{Config: pbStrings(c.ECH.Config)}
	}
	return pb
}

func coreTransportToPB(c *core.TransportConfig) *PBTransportConfig {
	if c == nil {
		return nil
	}
	pb := &PBTransportConfig{Type: c.Type}
	if c.HTTP != nil {
		pb.Http = &PBHTTPConfig{Host: pbStrings(c.HTTP.Host), Path: pbB(c.HTTP.Path), Method: pbB(c.HTTP.Method)}
	}
	if c.WebSocket != nil {
		pb.Websocket = &PBWebSocketConfig{Path: pbB(c.WebSocket.Path), Host: pbB(c.WebSocket.Host)}
	}
	if c.QUIC != nil {
		pb.Quic = &PBQUICConfig{}
	}
	if c.GRPC != nil {
		pb.Grpc = &PBGRPCConfig{ServiceName: pbB(c.GRPC.ServiceName)}
	}
	if c.HTTPUpgrade != nil {
		pb.HttpUpgrade = &PBHTTPUpgradeConfig{Host: pbB(c.HTTPUpgrade.Host), Path: pbB(c.HTTPUpgrade.Path)}
	}
	if c.XHTTP != nil {
		pb.Xhttp = &PBXHTTPConfig{
			Host: pbB(c.XHTTP.Host), Path: pbB(c.XHTTP.Path), Mode: pbB(c.XHTTP.Mode), Extra: pbStringMap(c.XHTTP.Extra),
		}
	}
	if c.SplitHTTP != nil {
		pb.SplitHttp = &PBSplitHTTPConfig{Host: pbB(c.SplitHTTP.Host), Path: pbB(c.SplitHTTP.Path)}
	}
	if c.KCP != nil {
		pb.Kcp = &PBKCPConfig{Seed: pbB(c.KCP.Seed)}
	}
	return pb
}
