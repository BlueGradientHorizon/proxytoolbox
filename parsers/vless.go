package parsers

import (
	"errors"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/utils"
)

type VLESSParser struct{}

func (p VLESSParser) ParseProfile(connURI string) (*ProxyProfile, error) {
	connURI, err := utils.TryFixURI(connURI)
	if err != nil {
		return nil, errors.New("VLESSParser.ParseProfile: " + err.Error())
	}

	uri, addr, port, err := extractCommonURIData(connURI, "vless")
	if err != nil {
		return nil, errors.New("VLESSParser.ParseProfile: " + err.Error())
	}

	params := uri.Query()

	flow := params.Get("flow")
	if flow == "xtls-rprx-vision-udp443" {
		flow = "xtls-rprx-vision"
	}

	TLSOptions, err := buildOutboundTLSOptions(params, "vless")
	if err != nil {
		return nil, errors.New("VLESSParser.ParseProfile: " + err.Error())
	}

	transportOptions, err := buildV2RayTransportOptions(params, "vless")
	if err != nil {
		return nil, errors.New("VLESSParser.ParseProfile: " + err.Error())
	}

	// Create generic OutboundConfig with VLESS settings
	config := &core.OutboundConfig{
		Type:   "vless",
		Server: addr,
		Port:   port,
		Settings: core.VLESSSettings{
			UUID: uri.User.Username(),
			Flow: flow,
		},
		TLS:       TLSOptions,
		Transport: transportOptions,
	}

	return &ProxyProfile{
		Config:  config,
		ConnURI: connURI,
	}, nil
}
