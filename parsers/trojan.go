package parsers

import (
	"errors"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/utils"
)

type TrojanParser struct{}

func (p TrojanParser) ParseProfile(connURI string) (*ProxyProfile, error) {
	connURI, err := utils.TryFixURI(connURI)
	if err != nil {
		return nil, errors.New("TrojanParser.ParseProfile: " + err.Error())
	}

	url, addr, port, err := extractCommonURIData(connURI, "trojan")
	if err != nil {
		return nil, errors.New("TrojanParser.ParseProfile: " + err.Error())
	}

	params := url.Query()

	password := url.User.Username()

	TLSOptions, err := buildOutboundTLSOptions(params, "trojan")
	if err != nil {
		return nil, errors.New("TrojanParser.ParseProfile: " + err.Error())
	}

	transportOptions, err := buildV2RayTransportOptions(params, "trojan")
	if err != nil {
		return nil, errors.New("TrojanParser.ParseProfile: " + err.Error())
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

	return &ProxyProfile{
		Config:  config,
		ConnURI: connURI,
	}, nil
}
