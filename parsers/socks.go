package parsers

import (
	"errors"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

type SocksParser struct{}

func (p SocksParser) ParseConfig(connURI string) (*ProxyConfig, error) {
	connURI, err := tryFixURI(connURI)
	if err != nil {
		return nil, errors.New("SocksParser.ParseConfig: " + err.Error())
	}

	uri, addr, port, err := extractCommonURIData(connURI, "socks", nil)
	if err != nil {
		return nil, errors.New("SocksParser.ParseConfig: " + err.Error())
	}

	username := uri.User.Username()
	password, _ := uri.User.Password()

	if password == "" && username != "" {
		dec, err := tryDecodeBase64(username)
		if err == nil {
			decStr := string(dec)
			if strings.Contains(decStr, ":") {
				parts := strings.SplitN(decStr, ":", 2)
				username = parts[0]
				password = parts[1]
			}
		}
	}

	config := &core.OutboundConfig{
		Type:   "socks",
		Server: addr,
		Port:   port,
		Settings: core.SocksSettings{
			Version: "5",
			User:    username,
			Pass:    password,
		},
	}

	return &ProxyConfig{
		Config:  config,
		ConnURI: connURI,
	}, nil
}
