// Package core provides core-agnostic interfaces and types for proxy configurations
// and connections. This abstraction layer allows the library to support multiple
// proxy cores (sing-box, xray, clash) without modifying parser or tester logic.
package core

import (
	"context"
	"net"

	"github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// Outbound represents a generic proxy connection interface that any core
// implementation must satisfy. This interface abstracts the connection
// operations needed by testers, allowing them to work with any proxy core.
type Outbound interface {
	// Tag returns the unique identifier for this outbound
	Tag() string

	// Type returns the protocol type (vless, trojan, vmess, ss, hysteria2)
	Type() string

	// DialContext establishes a connection through the proxy to the specified destination.
	// This is the core method used by latency and speed testers.
	DialContext(ctx context.Context, network string, destination metadata.Socksaddr) (net.Conn, error)

	// NewConnection wraps an existing connection through the proxy (for advanced use cases)
	NewConnection(ctx context.Context, conn net.Conn, destination metadata.Socksaddr) (net.Conn, error)

	// NewPacketConnection creates a packet-based connection through the proxy (for UDP)
	NewPacketConnection(ctx context.Context, conn N.PacketConn, destination metadata.Socksaddr) (N.PacketConn, error)
}

// ConfigConverter converts generic configurations to core-specific types.
// Each proxy core (sing-box, xray, clash) implements this interface to
// translate core-agnostic OutboundConfig into its native outbound type.
type ConfigConverter interface {
	// ConvertOutbound converts a generic OutboundConfig to a core-specific outbound
	// that implements the Outbound interface
	ConvertOutbound(config *OutboundConfig) (Outbound, error)
}
