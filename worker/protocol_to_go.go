package worker

import (
	"fmt"
	"time"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

func PBLatencyTestSettingsToGo(pb *PBLatencyTestSettings) LatencyTestSettings {
	if pb == nil {
		return LatencyTestSettings{}
	}
	return LatencyTestSettings{
		TestURL:     pb.GetTestUrl(),
		RawRequest:  pb.GetRawRequest(),
		Timeout:     time.Duration(pb.GetTimeoutNanos()),
		Concurrency: int(pb.GetConcurrency()),
	}
}

func PBSpeedTestSettingsToGo(pb *PBSpeedTestSettings) SpeedTestSettings {
	if pb == nil {
		return SpeedTestSettings{}
	}
	var mode SpeedTestMode
	switch pb.GetMode() {
	case PBSpeedTestMode_SPEED_DOWNLOAD:
		mode = SpeedTestModeDownload
	case PBSpeedTestMode_SPEED_UPLOAD:
		mode = SpeedTestModeUpload
	}
	return SpeedTestSettings{
		Mode:        mode,
		TestURL:     pb.GetTestUrl(),
		RawRequest:  pb.GetRawRequest(),
		Timeout:     time.Duration(pb.GetTimeoutNanos()),
		TargetBytes: pb.GetTargetBytes(),
		Concurrency: int(pb.GetConcurrency()),
	}
}

func PBOutboundConfigToCore(rc *PBOutboundConfig) (*core.OutboundConfig, error) {
	if rc == nil {
		return nil, fmt.Errorf("nil config")
	}
	cfg := &core.OutboundConfig{
		Tag:       pbS(rc.GetTag()),
		Type:      pbS(rc.GetType()),
		Server:    pbS(rc.GetServer()),
		Port:      uint16(rc.GetPort()),
		TLS:       pbTLSToCore(rc.GetTls()),
		Transport: pbTransportToCore(rc.GetTransport()),
	}
	settings, err := pbSettingsToCore(rc)
	if err != nil {
		return nil, err
	}
	cfg.Settings = settings
	return cfg, nil
}

func pbSettingsToCore(rc *PBOutboundConfig) (core.ProtocolSettings, error) {
	switch s := rc.GetSettings().(type) {
	case *PBOutboundConfig_Vless:
		v := s.Vless
		return core.VLESSSettings{
			UUID:       pbS(v.GetUuid()),
			Flow:       pbS(v.GetFlow()),
			Encryption: pbS(v.GetEncryption()),
		}, nil
	case *PBOutboundConfig_Trojan:
		return core.TrojanSettings{Password: pbS(s.Trojan.GetPassword())}, nil
	case *PBOutboundConfig_Vmess:
		v := s.Vmess
		return core.VMessSettings{
			UUID:     pbS(v.GetUuid()),
			AlterID:  int(v.GetAlterId()),
			Security: pbS(v.GetSecurity()),
		}, nil
	case *PBOutboundConfig_Shadowsocks:
		ss := s.Shadowsocks
		return core.ShadowsocksSettings{
			Method:   pbS(ss.GetMethod()),
			Password: pbS(ss.GetPassword()),
		}, nil
	case *PBOutboundConfig_Hysteria2:
		h := s.Hysteria2
		var obfs *core.ObfsConfig
		if o := h.GetObfs(); o != nil {
			obfs = &core.ObfsConfig{Type: pbS(o.GetType()), Password: pbS(o.GetPassword())}
		}
		return core.Hysteria2Settings{Password: pbS(h.GetPassword()), Obfs: obfs}, nil
	case *PBOutboundConfig_Wireguard:
		w := s.Wireguard
		peers := make([]core.WireguardPeer, len(w.GetPeers()))
		for i, p := range w.GetPeers() {
			peers[i] = core.WireguardPeer{
				PublicKey: pbS(p.GetPublicKey()),
				Endpoint:  pbS(p.GetEndpoint()),
			}
		}
		return core.WireguardSettings{
			SecretKey: pbS(w.GetSecretKey()),
			Address:   pbStringSlice(w.GetAddress()),
			Peers:     peers,
		}, nil
	case *PBOutboundConfig_Socks:
		socks := s.Socks
		return core.SocksSettings{
			Version: pbS(socks.GetVersion()),
			User:    pbS(socks.GetUser()),
			Pass:    pbS(socks.GetPass()),
		}, nil
	case *PBOutboundConfig_Http:
		return core.HTTPSettings{User: pbS(s.Http.GetUser()), Pass: pbS(s.Http.GetPass())}, nil
	case *PBOutboundConfig_Vlite:
		return core.VLiteSettings{Password: pbS(s.Vlite.GetPassword())}, nil
	default:
		return nil, fmt.Errorf("unknown type %s", pbS(rc.GetType()))
	}
}

func pbTLSToCore(pb *PBTLSConfig) *core.TLSConfig {
	if pb == nil {
		return nil
	}
	cfg := &core.TLSConfig{
		Enabled:     pb.GetEnabled(),
		ServerName:  pbS(pb.GetServerName()),
		Insecure:    pb.GetInsecure(),
		ALPN:        pbStringSlice(pb.GetAlpn()),
		Fingerprint: pbS(pb.GetFingerprint()),
	}
	if r := pb.GetReality(); r != nil {
		cfg.Reality = &core.RealityConfig{
			PublicKey: pbS(r.GetPublicKey()),
			ShortID:   pbS(r.GetShortId()),
			SpiderX:   pbS(r.GetSpiderX()),
		}
	}
	if e := pb.GetEch(); e != nil {
		cfg.ECH = &core.ECHConfig{Config: pbStringSlice(e.GetConfig())}
	}
	return cfg
}

func pbTransportToCore(pb *PBTransportConfig) *core.TransportConfig {
	if pb == nil {
		return nil
	}
	cfg := &core.TransportConfig{Type: pb.GetType()}
	if h := pb.GetHttp(); h != nil {
		cfg.HTTP = &core.HTTPConfig{Host: pbStringSlice(h.GetHost()), Path: pbS(h.GetPath()), Method: pbS(h.GetMethod())}
	}
	if ws := pb.GetWebsocket(); ws != nil {
		cfg.WebSocket = &core.WebSocketConfig{Path: pbS(ws.GetPath()), Host: pbS(ws.GetHost())}
	}
	if pb.GetQuic() != nil {
		cfg.QUIC = &core.QUICConfig{}
	}
	if g := pb.GetGrpc(); g != nil {
		cfg.GRPC = &core.GRPCConfig{ServiceName: pbS(g.GetServiceName())}
	}
	if hu := pb.GetHttpUpgrade(); hu != nil {
		cfg.HTTPUpgrade = &core.HTTPUpgradeConfig{Host: pbS(hu.GetHost()), Path: pbS(hu.GetPath())}
	}
	if x := pb.GetXhttp(); x != nil {
		cfg.XHTTP = &core.XHTTPConfig{
			Host: pbS(x.GetHost()), Path: pbS(x.GetPath()), Mode: pbS(x.GetMode()), Extra: pbMapToString(x.GetExtra()),
		}
	}
	if sh := pb.GetSplitHttp(); sh != nil {
		cfg.SplitHTTP = &core.SplitHTTPConfig{Host: pbS(sh.GetHost()), Path: pbS(sh.GetPath())}
	}
	if k := pb.GetKcp(); k != nil {
		cfg.KCP = &core.KCPConfig{Seed: pbS(k.GetSeed())}
	}
	return cfg
}
