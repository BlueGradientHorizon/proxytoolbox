package parsers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

type ProxyConfig struct {
	Config  *core.OutboundConfig
	ConnURI string
}

type ConfigParser interface {
	ParseConfig(string) (*ProxyConfig, error)
}

func ParseConfig(connURI string) (*ProxyConfig, error) {
	connURI = strings.TrimSpace(connURI)
	if connURI == "" {
		return nil, errors.New("ParseConfig: empty configuration URI")
	}

	splitURI := strings.SplitN(connURI, "://", 2)

	// TODO should be case-insensitive
	parsers := map[string]ConfigParser{
		"vless":       VLESSParser{},
		"trojan":      TrojanParser{},
		"vmess":       VMessParser{},
		"ss":          ShadowsocksParser{},
		"shadowsocks": ShadowsocksParser{},
		"hysteria2":   Hysteria2Parser{},
		"hy2":         Hysteria2Parser{},
		"wg":          WireguardParser{},
		"wireguard":   WireguardParser{},
		"socks":       SocksParser{},
		"socks5":      SocksParser{},
		"http":        HTTPParser{},
		"https":       HTTPParser{},
		"vlite":       VLiteParser{},
	}

	scheme := strings.ToLower(splitURI[0])
	if parser, ok := parsers[scheme]; ok {
		config, err := parser.ParseConfig(connURI)
		if err != nil {
			return nil, errors.New("ParseConfig: " + err.Error())
		}
		return config, nil
	} else {
		return nil, fmt.Errorf("ParseConfig: unknown config URI scheme %s", splitURI[0])
	}
}
