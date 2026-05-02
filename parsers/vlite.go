package parsers

import (
	"errors"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

type VLiteParser struct{}

func (p VLiteParser) ParseConfig(connURI string) (*ProxyConfig, error) {
	connURI, err := tryFixURI(connURI)
	if err != nil {
		return nil, errors.New("VLiteParser.ParseConfig: " + err.Error())
	}

	uri, addr, port, err := extractCommonURIData(connURI, "vlite", nil)
	if err != nil {
		return nil, errors.New("VLiteParser.ParseConfig: " + err.Error())
	}

	password := uri.User.Username()

	config := &core.OutboundConfig{
		Type:   "vlite",
		Server: addr,
		Port:   port,
		Settings: core.VLiteSettings{
			Password: password,
		},
	}

	return &ProxyConfig{
		Config:  config,
		ConnURI: connURI,
	}, nil
}
