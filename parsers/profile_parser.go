package parsers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

type ProxyProfile struct {
	Config  *core.OutboundConfig
	ConnURI string
}

type ProfileParser interface {
	ParseProfile(string) (*ProxyProfile, error)
}

func ParseProfile(connURI string) (*ProxyProfile, error) {
	connURI = strings.TrimSpace(connURI)
	if connURI == "" {
		return nil, errors.New("ParseProfile: empty configuration URI")
	}

	splitURI := strings.Split(connURI, "://")

	parsers := map[string]ProfileParser{
		"vless":     VLESSParser{},
		"trojan":    TrojanParser{},
		"vmess":     VMessParser{},
		"ss":        ShadowsocksParser{},
		"hysteria2": Hysteria2Parser{},
		"hy2":       Hysteria2Parser{},
	}

	if parser, ok := parsers[splitURI[0]]; ok {
		profile, err := parser.ParseProfile(connURI)
		if err != nil {
			return nil, errors.New("ParseProfile: " + err.Error())
		}
		return profile, nil
	} else {
		return nil, fmt.Errorf("ParseProfile: unknown profile URI scheme %s", splitURI[0])
	}
}
