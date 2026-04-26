package parsers

import (
	"errors"
	"net/url"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

type TrojanParser struct{}

func fixTrojanURI(uri string) (*url.URL, error) {
	remarkSplitLastIndex := strings.LastIndex(uri, "#")

	var beforeRemark string
	if remarkSplitLastIndex == -1 {
		beforeRemark = uri
	} else {
		beforeRemark = uri[:remarkSplitLastIndex]
	}

	var remark string
	if remarkSplitLastIndex < len(uri) {
		remark = uri[remarkSplitLastIndex+1:]
	} else {
		remark = ""
	}

	lastAt := strings.LastIndex(beforeRemark, "@")
	if lastAt == -1 {
		return nil, errors.New("fixTrojanURI: malformed URI: symbol '@' not found")
	}

	beforeAt := beforeRemark[:lastAt]
	afterAt := beforeRemark[lastAt+1:]

	schemeSplit := strings.SplitN(beforeAt, "://", 2)
	if len(schemeSplit) < 2 {
		return nil, errors.New("fixTrojanURI: malformed URI: split by '://' failed")
	}
	scheme := schemeSplit[0]
	userInfo := schemeSplit[1]

	querySplit := strings.SplitN(afterAt, "?", 2)
	hostPort := querySplit[0]

	tempURI := scheme + "://placeholder@" + afterAt
	u, err := url.Parse(tempURI)
	if err != nil {
		return nil, errors.New("fixTrojanURI: " + err.Error())
	}

	u.User = url.User(userInfo)
	u.Host = strings.ReplaceAll(hostPort, "/", "")
	u.Fragment = remark
	return u, nil
}

func (p TrojanParser) ParseConfig(connURI string) (*ProxyConfig, error) {
	connURI, err := tryFixURI(connURI)
	if err != nil {
		return nil, errors.New("TrojanParser.ParseConfig: " + err.Error())
	}

	url, addr, port, err := extractCommonURIData(connURI, "trojan", fixTrojanURI)
	if err != nil {
		return nil, errors.New("TrojanParser.ParseConfig: " + err.Error())
	}

	params := url.Query()

	password := url.User.Username()

	TLSOptions, err := buildOutboundTLSOptions(params, "trojan")
	if err != nil {
		return nil, errors.New("TrojanParser.ParseConfig: " + err.Error())
	}

	transportOptions, err := buildV2RayTransportOptions(params, "trojan")
	if err != nil {
		return nil, errors.New("TrojanParser.ParseConfig: " + err.Error())
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

	return &ProxyConfig{
		Config:  config,
		ConnURI: connURI,
	}, nil
}
