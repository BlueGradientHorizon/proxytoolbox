package parsers

import (
	"errors"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/utils"
)

type Hysteria2Parser struct{}

func (p Hysteria2Parser) ParseProfile(connURI string) (*ProxyProfile, error) {
	connURI, err := utils.TryFixURI(connURI)
	if err != nil {
		return nil, errors.New("Hysteria2Parser.ParseProfile: " + err.Error())
	}

	uri, addr, port, err := extractCommonURIData(connURI, "hysteria2")
	if err != nil {
		return nil, errors.New("Hysteria2Parser.ParseProfile: " + err.Error())
	}

	params := uri.Query()

	sni := params.Get("sni")
	insecure := params.Get("insecure") == "1"
	// pinSHA256 := params.Get("pinSHA256")
	obfsType := params.Get("obfs")
	salamanderPassword := params.Get("obfs-password")
	password := uri.User.Username()

	// Create Hysteria2Settings with obfuscation if present
	settings := core.Hysteria2Settings{
		Password: password,
	}

	if obfsType != "" && salamanderPassword != "" {
		settings.Obfs = &core.ObfsConfig{
			Type:     obfsType,
			Password: salamanderPassword,
		}
	}

	// Build TLS configuration
	tlsConfig := &core.TLSConfig{
		Enabled:    true,
		ServerName: sni,
		Insecure:   insecure,
	}

	if sni == "" {
		tlsConfig.Insecure = true
	}

	// Create generic OutboundConfig
	config := &core.OutboundConfig{
		Type:     "hysteria2",
		Server:   addr,
		Port:     port,
		Settings: settings,
		TLS:      tlsConfig,
	}

	return &ProxyProfile{
		Config:  config,
		ConnURI: connURI,
	}, nil
}
