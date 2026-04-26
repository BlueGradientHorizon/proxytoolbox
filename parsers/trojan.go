package parsers

import (
	"errors"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

type TrojanParser struct{}

func (p TrojanParser) ParseConfig(connURI string) (*ProxyConfig, error) {
	connURI, err := tryFixURI(connURI)
	if err != nil {
		return nil, errors.New("TrojanParser.ParseConfig: " + err.Error())
	}

	url, addr, port, err := extractCommonURIData(connURI, "trojan")
	if err != nil {
		return nil, errors.New("TrojanParser.ParseConfig: " + err.Error())
	}

	params := url.Query()

	password := url.User.Username()

	TLSOptions, err := buildOutboundTLSOptions(params, "trojan")
	if err != nil {
		return nil, errors.New("TrojanParser.ParseConfig: " + err.Error())
	}

	transportOptions, err := buildV2RayTransportOptions(params, "trojan")
	if err != nil {
		return nil, errors.New("TrojanParser.ParseConfig: " + err.Error())
	}

	// Create generic OutboundConfig with Trojan settings
	config := &core.OutboundConfig{
		Type:   "trojan",
		Server: addr,
		Port:   port,
		Settings: core.TrojanSettings{
			Password: password,
		},
		TLS:       TLSOptions,
		Transport: transportOptions,
	}

	return &ProxyConfig{
		Config:  config,
		ConnURI: connURI,
	}, nil
}
