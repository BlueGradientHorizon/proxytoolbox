package parsers

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

func buildOutboundTLSOptions(query url.Values, protocol string) (*core.TLSConfig, error) {
	config := &core.TLSConfig{}

	securityKey := "security"

	if protocol == "vmess" {
		securityKey = "tls"
	}

	security := query.Get(securityKey) // "none"
	sni := query.Get("sni")
	alpn := query.Get("alpn")
	fp := query.Get("fp")
	pbk := query.Get("pbk")
	sid := query.Get("sid")
	// pqv := query.Get("pqv")
	ech := query.Get("ech")
	allowInsecure := query.Get("allowInsecure") == "1"
	insecure := query.Get("insecure") == "1"

	if security != "" {
		if (security != "tls") && (security != "reality") && (security != "none") {
			return nil, fmt.Errorf("buildOutboundTLSOptions: unsupported security parameter %s", security)
		}

		if security != "none" {
			config.Enabled = true
		}

		config.ServerName = sni
		config.Fingerprint = fp

		if alpn != "" {
			parts := strings.Split(alpn, ",")
			config.ALPN = parts
		}

		if ech != "" {
			config.ECH = &core.ECHConfig{
				Config: []string{ech},
			}
		}

		if insecure || allowInsecure {
			config.Insecure = true
		}
	}

	if security == "reality" {
		config.Reality = &core.RealityConfig{
			PublicKey: pbk,
			ShortID:   sid,
		}

		// uTLS fingerprint is required by reality client
		if fp == "" {
			config.Fingerprint = "chrome"
		}
	}

	return config, nil
}

func buildV2RayTransportOptions(query url.Values, protocol string) (*core.TransportConfig, error) {
	config := &core.TransportConfig{}

	typeKey := "type"
	serviceNameKey := "serviceName"
	// modeKey := "mode"

	if protocol == "vmess" {
		typeKey = "net"
		serviceNameKey = "path"
		// modeKey = "type"
	}

	// spx := query.Get("spx")
	path := query.Get("path")
	host := query.Get("host")
	// headerType := query.Get("headerType")    // "none"
	serviceName := query.Get(serviceNameKey) // sni or host
	// authority := query.Get("authority")
	// seed := query.Get("seed")
	// mode := query.Get(modeKey)

	type_ := query.Get(typeKey) // "raw"

	switch type_ {
	case "", "raw", "tcp":
		config.Type = "tcp"
	case "http", "h2":
		config.Type = "http"
		config.HTTP = &core.HTTPConfig{
			Host:   []string{host},
			Path:   path,
			Method: "GET",
		}
	case "ws", "websocket":
		config.Type = "ws"
		if path == "" {
			path = "/"
		}
		config.WebSocket = &core.WebSocketConfig{
			Path: path,
			Host: host,
		}
	case "quic":
		config.Type = "quic"
		config.QUIC = &core.QUICConfig{}
	case "grpc":
		config.Type = "grpc"
		config.GRPC = &core.GRPCConfig{
			ServiceName: serviceName,
		}
	case "httpupgrade":
		config.Type = "httpupgrade"
		config.HTTPUpgrade = &core.HTTPUpgradeConfig{
			Host: host,
			Path: path,
		}
	case "kcp":
		return nil, errors.New("buildV2RayTransportOptions: transport kcp unsupported")
	case "mkcp":
		return nil, errors.New("buildV2RayTransportOptions: transport mkcp unsupported")
	case "xhttp":
		return nil, errors.New("buildV2RayTransportOptions: transport xhttp unsupported")
	case "splithttp":
		return nil, errors.New("buildV2RayTransportOptions: transport splithttp unsupported")
	default:
		return nil, fmt.Errorf("buildV2RayTransportOptions: unknown transport %s", type_)
	}

	return config, nil
}

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

func parseConfigURI(uri string, scheme string) (*url.URL, error) {
	if scheme == "trojan" {
		u, err := fixTrojanURI(uri)
		if err != nil {
			return nil, errors.New("parseConfigURI: " + err.Error())
		}
		return u, nil
	}

	u, err := url.Parse(uri)
	if err != nil {
		return nil, errors.New("parseConfigURI: " + err.Error())
	}

	if u.Scheme == "" {
		u.Scheme = scheme
	}

	return u, nil
}

func parseNetlocForEndpoint(u *url.URL) (string, uint16, bool) {
	netloc := u.Host
	ipv6Regexp := regexp.MustCompile(`^\[([a-fA-F0-9:]+)\]:(\d+)$`)
	match := ipv6Regexp.FindStringSubmatch(netloc)

	var address string
	var port int

	if len(match) > 0 {
		address = match[1]
		p, _ := strconv.Atoi(match[2])
		port = p
	} else {
		host, portStr, err := net.SplitHostPort(netloc)
		if err != nil {
			address = netloc
			pStr := u.Port()
			if pStr != "" {
				p, err := strconv.Atoi(pStr)
				if err != nil {
					return "", 0, false
				}
				port = p
			} else {
				port = 0
			}
		} else {
			address = host
			p, _ := strconv.Atoi(portStr)
			port = p
		}
	}

	address = strings.TrimPrefix(address, "[")
	address = strings.TrimSuffix(address, "]")

	if port < 0 || port > math.MaxUint16 {
		return "", 0, false
	}

	return address, uint16(port), true
}

func extractCommonURIData(uri string, scheme string) (*url.URL, string, uint16, error) {
	parsedURI, err := parseConfigURI(uri, scheme)
	if err != nil {
		return nil, "", 0, errors.New("extractCommonURIData: " + err.Error())
	}

	address, port, ok := parseNetlocForEndpoint(parsedURI)
	if !ok {
		return nil, "", 0, errors.New("extractCommonURIData: cannot parse netloc for endpoint")
	}

	return parsedURI, address, port, nil
}
