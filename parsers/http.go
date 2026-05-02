package parsers

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

type HTTPParser struct{}

func (p HTTPParser) ParseConfig(connURI string) (*ProxyConfig, error) {
	connURI, err := tryFixURI(connURI)
	if err != nil {
		return nil, errors.New("HTTPParser.ParseConfig: " + err.Error())
	}

	uri, addr, port, err := extractCommonURIData(connURI, "http", nil)
	if err != nil {
		return nil, errors.New("HTTPParser.ParseConfig: " + err.Error())
	}

	username := uri.User.Username()
	password, _ := uri.User.Password()

	if password == "" && username != "" {
		dec, err := base64.StdEncoding.DecodeString(username)
		if err != nil {
			dec, err = base64.URLEncoding.DecodeString(username)
		}
		if err != nil {
			dec, err = base64.RawURLEncoding.DecodeString(username)
		}
		if err != nil {
			dec, err = base64.RawStdEncoding.DecodeString(username)
		}
		if err == nil {
			decStr := string(dec)
			if strings.Contains(decStr, ":") {
				parts := strings.SplitN(decStr, ":", 2)
				username = parts[0]
				password = parts[1]
			}
		}
	}

	params := uri.Query()

	TLSOptions, err := buildOutboundTLSOptions(params, "http")
	if err != nil {
		return nil, errors.New("HTTPParser.ParseConfig: " + err.Error())
	}

	if uri.Scheme == "https" {
		if !TLSOptions.Enabled {
			TLSOptions.Enabled = true
			if TLSOptions.ServerName == "" {
				TLSOptions.ServerName = addr
			}
		}
	}

	config := &core.OutboundConfig{
		Type:   "http",
		Server: addr,
		Port:   port,
		Settings: core.HTTPSettings{
			User: username,
			Pass: password,
		},
		TLS: TLSOptions,
	}

	return &ProxyConfig{
		Config:  config,
		ConnURI: connURI,
	}, nil
}
