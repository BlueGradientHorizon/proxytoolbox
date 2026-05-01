package parsers

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

type ShadowsocksParser struct{}

func (p ShadowsocksParser) ParseConfig(connURI string) (*ProxyConfig, error) {
	connURI, err := tryFixURI(connURI)
	if err != nil {
		return nil, errors.New("ShadowsocksParser.ParseConfig: " + err.Error())
	}

	// Handle full base64 encoded ss:// links
	// base64Part := connURI[5:] // skip "ss://"
	// fragment := ""
	// if idx := strings.Index(base64Part, "#"); idx != -1 {
	// 	fragment = base64Part[idx:]
	// 	base64Part = base64Part[:idx]
	// }

	// base64PartUnescaped, _ := url.PathUnescape(base64Part)

	// decodedBytes, decErr := base64.StdEncoding.DecodeString(base64PartUnescaped)
	// if decErr != nil {
	// 	decodedBytes, decErr = base64.URLEncoding.DecodeString(base64PartUnescaped)
	// }
	// if decErr != nil {
	// 	decodedBytes, decErr = base64.RawURLEncoding.DecodeString(base64PartUnescaped)
	// }
	// if decErr != nil {
	// 	decodedBytes, decErr = base64.RawStdEncoding.DecodeString(base64PartUnescaped)
	// }

	// if decErr == nil && strings.Contains(string(decodedBytes), "@") {
	// 	decodedStr := string(decodedBytes)
	// 	lastAt := strings.LastIndex(decodedStr, "@")
	// 	user := decodedStr[:lastAt]
	// 	hostPort := decodedStr[lastAt+1:]
	// 	userEscaped := strings.ReplaceAll(url.PathEscape(user), "@", "%40")
	// 	connURI = "ss://" + userEscaped + "@" + hostPort + fragment
	// }

	uri, addr, port, err := extractCommonURIData(connURI, "shadowsocks", nil)
	if err != nil {
		return nil, errors.New("ShadowsocksParser.ParseConfig: " + err.Error())
	}

	params := uri.Query()

	var method, password string

	username := uri.User.Username()
	uriPassword, hasPassword := uri.User.Password()

	var authPart string
	if hasPassword {
		authPart = username + ":" + uriPassword
	} else {
		authPart = username
	}

	if !strings.Contains(authPart, ":") {
		decodedAuthBytes, err := base64.StdEncoding.DecodeString(authPart)
		if err != nil {
			decodedAuthBytes, err = base64.URLEncoding.DecodeString(authPart)
		}
		if err != nil {
			decodedAuthBytes, err = base64.RawURLEncoding.DecodeString(authPart)
		}
		if err == nil {
			decodedAuth := string(decodedAuthBytes)
			if strings.Contains(decodedAuth, ":") {
				method, password, _ = strings.Cut(decodedAuth, ":")
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
