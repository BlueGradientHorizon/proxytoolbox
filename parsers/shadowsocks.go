package parsers

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/utils"
)

type ShadowsocksParser struct{}

func (p ShadowsocksParser) ParseConfig(connURI string) (*ProxyConfig, error) {
	connURI, err := utils.TryFixURI(connURI)
	if err != nil {
		return nil, errors.New("ShadowsocksParser.ParseConfig: " + err.Error())
	}

	uri, addr, port, err := extractCommonURIData(connURI, "shadowsocks")
	if err != nil {
		return nil, errors.New("ShadowsocksParser.ParseConfig: " + err.Error())
	}

	decodedHostBytes, err := base64.StdEncoding.DecodeString(uri.Host)
	if err == nil {
		decodedHost := string(decodedHostBytes)
		uri, addr, port, err = extractCommonURIData("ss://"+decodedHost+"#"+uri.RawFragment, "shadowsocks")
		if err != nil {
			return nil, errors.New("ShadowsocksParser.ParseConfig: " + err.Error())
		}
	}

	params := uri.Query()

	var method, password string

	authPart := uri.User.String()
	uriPassword, _ := uri.User.Password()

	if !strings.Contains(authPart, ":") && uriPassword == "" {
		userPart := uri.User.String()
		decodedAuthBytes, err := base64.StdEncoding.DecodeString(userPart)
		if err == nil {
			decodedAuth := string(decodedAuthBytes)
			if strings.Count(decodedAuth, ":") > 0 {
				method, password, _ = strings.Cut(decodedAuth, ":")
			} else {
				return nil, errors.New("ShadowsocksParser.ParseConfig: malformed base64 encoded user:pass tuple")
			}
		}
	}

	if method == "" && password == "" {
		if strings.Contains(authPart, ":") {
			method, password, _ = strings.Cut(authPart, ":")
		} else {
			method = params.Get("method")
			if method == "" {
				method = "none"
			}
			password = authPart
		}
	}

	// Create generic OutboundConfig with Shadowsocks settings
	config := &core.OutboundConfig{
		Type:   "shadowsocks",
		Server: addr,
		Port:   port,
		Settings: core.ShadowsocksSettings{
			Method:   method,
			Password: password,
		},
	}

	return &ProxyConfig{
		Config:  config,
		ConnURI: connURI,
	}, nil
}
