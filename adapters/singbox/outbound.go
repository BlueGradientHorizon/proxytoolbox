// Package singbox provides an adapter for the sing-box proxy core.
// It converts generic core configurations to sing-box-specific types
// and wraps sing-box outbounds to implement the generic core.Outbound interface.
package singbox

import (
	"context"
	"errors"
	"net"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

// OutboundWrapper wraps a sing-box adapter.Outbound to implement the generic
// core.Outbound interface. This allows sing-box outbounds to be used with
// core-agnostic testers without modification.
//
// All connection operations are delegated to the underlying sing-box outbound,
// ensuring that sing-box's connection handling logic is preserved.
type OutboundWrapper struct {
	underlying adapter.Outbound
}

// NewOutboundWrapper creates a new wrapper around a sing-box outbound.
// The wrapper implements the core.Outbound interface by delegating all
// operations to the underlying sing-box adapter.Outbound.
//
// Parameters:
//   - outbound: The sing-box adapter.Outbound to wrap
//
// Returns:
//   - A core.Outbound interface implementation that wraps the sing-box outbound
func NewOutboundWrapper(outbound adapter.Outbound) core.Outbound {
	return &OutboundWrapper{
		underlying: outbound,
	}
}

// Tag returns the outbound's unique identifier tag.
// Delegates to the underlying sing-box outbound.
func (w *OutboundWrapper) Tag() string {
	return w.underlying.Tag()
}

// Type returns the outbound's protocol type (vless, trojan, vmess, ss, hysteria2).
// Delegates to the underlying sing-box outbound.
func (w *OutboundWrapper) Type() string {
	return w.underlying.Type()
}

// DialContext establishes a connection through the proxy to the specified destination.
// This is the primary method used by latency and speed testers.
// Delegates to the underlying sing-box outbound.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - network: Network type (e.g., "tcp", "udp")
//   - destination: Target address to connect to
//
// Returns:
//   - net.Conn: The established connection
//   - error: Any error that occurred during connection establishment
func (w *OutboundWrapper) DialContext(ctx context.Context, network string, destination metadata.Socksaddr) (net.Conn, error) {
	return w.underlying.DialContext(ctx, network, destination)
}

// NewConnection wraps an existing connection through the proxy.
// This is used for advanced connection handling scenarios.
//
// Note: This method is not supported by sing-box adapter.Outbound and will
// return an error. It is included in the interface for completeness and
// compatibility with other proxy cores that may support this functionality.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - conn: Existing connection to wrap
//   - destination: Target address
//
// Returns:
//   - net.Conn: The wrapped connection
//   - error: Always returns an error indicating this method is not supported
func (w *OutboundWrapper) NewConnection(ctx context.Context, conn net.Conn, destination metadata.Socksaddr) (net.Conn, error) {
	return nil, errors.New("NewConnection: not supported by sing-box adapter.Outbound")
}

// NewPacketConnection creates a packet-based connection through the proxy.
// This is used for UDP-based protocols.
//
// Note: This method is not supported by sing-box adapter.Outbound and will
// return an error. It is included in the interface for completeness and
// compatibility with other proxy cores that may support this functionality.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - conn: Packet connection to wrap
//   - destination: Target address
//
// Returns:
//   - N.PacketConn: The wrapped packet connection
//   - error: Always returns an error indicating this method is not supported
func (w *OutboundWrapper) NewPacketConnection(ctx context.Context, conn N.PacketConn, destination metadata.Socksaddr) (N.PacketConn, error) {
	return nil, errors.New("NewPacketConnection: not supported by sing-box adapter.Outbound")
}

// Unwrap returns the underlying sing-box adapter.Outbound.
// This method is provided for advanced use cases where direct access
// to sing-box-specific functionality is needed.
//
// Returns:
//   - The wrapped sing-box adapter.Outbound
func (w *OutboundWrapper) Unwrap() adapter.Outbound {
	return w.underlying
}
