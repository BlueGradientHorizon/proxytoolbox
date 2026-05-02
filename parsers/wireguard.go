package parsers

import (
	"errors"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

type WireguardParser struct{}

func (p WireguardParser) ParseConfig(connURI string) (*ProxyConfig, error) {
	connURI, err := tryFixURI(connURI)
	if err != nil {
		return nil, errors.New("WireguardParser.ParseConfig: " + err.Error())
	}

	uri, addr, port, err := extractCommonURIData(connURI, "wireguard", nil)
	if err != nil {
		return nil, errors.New("WireguardParser.ParseConfig: " + err.Error())
	}

	params := uri.Query()

	secretKey := params.Get("privateKey")
	peerPublicKey := uri.User.Username()

	if secretKey == "" {
		secretKey = uri.User.Username()
		peerPublicKey = params.Get("publicKey")
		if peerPublicKey == "" {
			peerPublicKey = params.Get("peerPublicKey")
		}
	}

	addressStr := params.Get("address")
	if addressStr == "" {
		addressStr = params.Get("localAddress")
	}

	var address []string
	if addressStr != "" {
		address = strings.Split(addressStr, ",")
	}

	config := &core.OutboundConfig{
		Type:   "wireguard",
		Server: addr,
		Port:   port,
		Settings: core.WireguardSettings{
			SecretKey: secretKey,
			Address:   address,
			Peers: []core.WireguardPeer{
				{
					PublicKey: peerPublicKey,
					Endpoint:  uri.Host,
				},
			},
		},
	}

	return &ProxyConfig{
		Config:  config,
		ConnURI: connURI,
	}, nil
}
