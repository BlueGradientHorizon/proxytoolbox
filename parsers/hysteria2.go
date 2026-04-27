package parsers

import (
	"errors"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

type Hysteria2Parser struct{}

func (p Hysteria2Parser) ParseConfig(connURI string) (*ProxyConfig, error) {
	connURI, err := tryFixURI(connURI)
	if err != nil {
		return nil, errors.New("Hysteria2Parser.ParseConfig: " + err.Error())
	}

	uri, addr, port, err := extractCommonURIData(connURI, "hysteria2", nil)
	if err != nil {
		return nil, errors.New("Hysteria2Parser.ParseConfig: " + err.Error())
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
		tlsConfig.ServerName = addr
	}

	// Create generic OutboundConfig
	config := &core.OutboundConfig{
		Type:     "hysteria2",
		Server:   addr,
		Port:     port,
		Settings: settings,
		TLS:      tlsConfig,
	}

	return &ProxyConfig{
		Config:  config,
		ConnURI: connURI,
	}, nil
}
