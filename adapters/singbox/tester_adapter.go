// Package singbox provides adapters for using sing-box with generic testers.
package singbox

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/testers"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common"
	"github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/ntp"
)

// CreateDialerFunc creates a generic DialerFunc from a sing-box outbound.
// This function handles sing-box specific connection logic including early connection handshakes.
func CreateDialerFunc(outbound core.Outbound, startTime *interface{}) testers.DialerFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// Store start time for latency measurement
		if startTime != nil {
			*startTime = interface{}(nil) // Reset timing marker
		}

		instance, err := outbound.DialContext(ctx, network, metadata.ParseSocksaddr(addr))
		if err != nil {
			return nil, err
		}

		// Handle early connection handshakes (e.g., for XTLS)
		if earlyConn, isEarlyConn := common.Cast[N.EarlyConn](instance); isEarlyConn && earlyConn.NeedHandshake() {
			// Update start time after handshake for more accurate latency
			if startTime != nil {
				*startTime = interface{}(nil)
			}
		}

		return instance, nil
	}
}

// CreateTLSConfigProvider creates a TLS configuration provider for sing-box.
// This injects sing-box specific TLS settings like NTP time and root CA pool.
func CreateTLSConfigProvider() testers.TLSConfigProvider {
	return func(ctx context.Context) *tls.Config {
		return &tls.Config{
			Time:    ntp.TimeFuncFromContext(ctx),
			RootCAs: adapter.RootPoolFromContext(ctx),
		}
	}
}

// OutboundToProxyInfo converts a core.Outbound to a generic ProxyInfo.
func OutboundToProxyInfo(outbound core.Outbound) testers.ProxyInfo {
	return testers.ProxyInfo{
		Tag:  outbound.Tag(),
		Type: outbound.Type(),
	}
}
