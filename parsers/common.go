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
	spx := query.Get("spx")
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
			SpiderX:   spx,
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
	case "kcp", "mkcp":
		config.Type = "kcp"
		config.KCP = &core.KCPConfig{
			Seed: query.Get("seed"),
		}
	case "xhttp":
		config.Type = "xhttp"
		config.XHTTP = &core.XHTTPConfig{
			Host: host,
			Path: path,
			Mode: query.Get("mode"),
		}
	case "splithttp":
		config.Type = "splithttp"
		config.SplitHTTP = &core.SplitHTTPConfig{
			Host: host,
			Path: path,
		}
	default:
		return nil, fmt.Errorf("buildV2RayTransportOptions: unknown transport %s", type_)
	}

	return config, nil
}

type CustomURIFixer func(uri string) (*url.URL, error)

func parseConfigURI(uri string, scheme string, fixer CustomURIFixer) (*url.URL, error) {
	var u *url.URL
	var err error

	if fixer != nil {
		u, err = fixer(uri)
		if err != nil {
			return nil, errors.New("parseConfigURI: " + err.Error())
		}
	} else {
		u, err = url.Parse(uri)
		if err != nil {
			return nil, errors.New("parseConfigURI: " + err.Error())
		}
	}

	if u.Scheme == "" {
		u.Scheme = scheme
	}
	return u, nil
}

var ipv6Regexp = regexp.MustCompile(`^\[([a-fA-F0-9:]+)\]:(\d+)$`)

func parseNetlocForEndpoint(u *url.URL) (string, uint16, bool) {
	// TODO: return error instead of port 0
	netloc := u.Host
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

	if port <= 0 || port > math.MaxUint16 {
		return "", 0, false
	}

	return address, uint16(port), true
}

func extractCommonURIData(uri string, scheme string, fixer CustomURIFixer) (*url.URL, string, uint16, error) {
	parsedURI, err := parseConfigURI(uri, scheme, fixer)
	if err != nil {
		return nil, "", 0, errors.New("extractCommonURIData: " + err.Error())
	}

	address, port, ok := parseNetlocForEndpoint(parsedURI)
	if !ok {
		return nil, "", 0, errors.New("extractCommonURIData: cannot parse netloc for endpoint")
	}

	return parsedURI, address, port, nil
}
