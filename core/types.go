package core

// OutboundConfig represents a generic proxy configuration independent of any
// specific core implementation. Parsers produce this type, and adapters convert
// it to core-specific types.
type OutboundConfig struct {
	// Tag is the unique identifier for this outbound
	Tag string

	// Type is the protocol type (vless, trojan, vmess, ss, hysteria2)
	Type string

	// Server is the proxy server address (hostname or IP)
	Server string

	// Port is the proxy server port
	Port uint16

	// Settings contains protocol-specific configuration
	Settings ProtocolSettings

	// TLS contains TLS/Reality/ECH configuration (optional)
	TLS *TLSConfig

	// Transport contains transport layer configuration (optional)
	Transport *TransportConfig
}

// ProtocolSettings is an interface for protocol-specific configuration.
// Each protocol implements this interface with its own settings struct.
type ProtocolSettings interface {
	// Protocol returns the protocol name
	Protocol() string
}

// VLESSSettings contains VLESS-specific configuration
type VLESSSettings struct {
	// UUID is the user identifier
	UUID string

	// Flow is the flow control mode (e.g., xtls-rprx-vision)
	Flow string
}

// Protocol returns "vless"
func (v VLESSSettings) Protocol() string { return "vless" }

// TrojanSettings contains Trojan-specific configuration
type TrojanSettings struct {
	// Password is the trojan password
	Password string
}

// Protocol returns "trojan"
func (t TrojanSettings) Protocol() string { return "trojan" }

// VMessSettings contains VMess-specific configuration
type VMessSettings struct {
	// UUID is the user identifier
	UUID string

	// AlterID is the alterId value (legacy, usually 0)
	AlterID int

	// Security is the encryption method (auto, aes-128-gcm, chacha20-poly1305, none)
	Security string
}

// Protocol returns "vmess"
func (v VMessSettings) Protocol() string { return "vmess" }

// ShadowsocksSettings contains Shadowsocks-specific configuration
type ShadowsocksSettings struct {
	// Method is the encryption method
	Method string

	// Password is the shadowsocks password
	Password string
}

// Protocol returns "ss"
func (s ShadowsocksSettings) Protocol() string { return "ss" }

// Hysteria2Settings contains Hysteria2-specific configuration
type Hysteria2Settings struct {
	// Password is the hysteria2 password
	Password string

	// Obfs contains obfuscation configuration (optional)
	Obfs *ObfsConfig
}

// Protocol returns "hysteria2"
func (h Hysteria2Settings) Protocol() string { return "hysteria2" }

// WireguardSettings contains Wireguard-specific configuration
type WireguardSettings struct {
	SecretKey string
	Address   []string
	Peers     []WireguardPeer
}

// Protocol returns "wireguard"
func (w WireguardSettings) Protocol() string { return "wireguard" }

// WireguardPeer represents a peer in Wireguard configuration
type WireguardPeer struct {
	PublicKey string
	Endpoint  string
}

// SocksSettings contains Socks-specific configuration
type SocksSettings struct {
	Version string
	User    string
	Pass    string
}

// Protocol returns "socks"
func (s SocksSettings) Protocol() string { return "socks" }

// HTTPSettings contains HTTP-specific configuration
type HTTPSettings struct {
	User string
	Pass string
}

// Protocol returns "http"
func (h HTTPSettings) Protocol() string { return "http" }

// VLiteSettings contains VLite-specific configuration
type VLiteSettings struct {
	Password string
}

// Protocol returns "vlite"
func (v VLiteSettings) Protocol() string { return "vlite" }

// TLSConfig represents generic TLS configuration including Reality and ECH
type TLSConfig struct {
	// Enabled indicates whether TLS is enabled
	Enabled bool

	// ServerName is the SNI (Server Name Indication)
	ServerName string

	// Insecure allows insecure TLS connections (skip certificate verification)
	Insecure bool

	// ALPN is the list of Application-Layer Protocol Negotiation values
	ALPN []string

	// Fingerprint is the TLS fingerprint for uTLS (e.g., chrome, firefox)
	Fingerprint string

	// Reality contains REALITY protocol configuration (optional)
	Reality *RealityConfig

	// ECH contains Encrypted Client Hello configuration (optional)
	ECH *ECHConfig
}

// RealityConfig represents REALITY protocol configuration
type RealityConfig struct {
	// PublicKey is the REALITY public key
	PublicKey string

	// ShortID is the REALITY short ID
	ShortID string
}

// ECHConfig represents Encrypted Client Hello configuration
type ECHConfig struct {
	// Config is the list of ECH configuration strings
	Config []string
}

// TransportConfig represents generic transport layer configuration
// for protocols like WebSocket, HTTP/2, gRPC, etc.
type TransportConfig struct {
	// Type is the transport type (tcp, http, ws, quic, grpc, httpupgrade)
	Type string

	// HTTP contains HTTP/2 transport configuration (optional)
	HTTP *HTTPConfig

	// WebSocket contains WebSocket transport configuration (optional)
	WebSocket *WebSocketConfig

	// QUIC contains QUIC transport configuration (optional)
	QUIC *QUICConfig

	// GRPC contains gRPC transport configuration (optional)
	GRPC *GRPCConfig

	// HTTPUpgrade contains HTTP Upgrade transport configuration (optional)
	HTTPUpgrade *HTTPUpgradeConfig

	// XHTTP contains XHTTP transport configuration (optional)
	XHTTP *XHTTPConfig

	// SplitHTTP contains SplitHTTP transport configuration (optional)
	SplitHTTP *SplitHTTPConfig

	// KCP contains mKCP transport configuration (optional)
	KCP *KCPConfig
}

// HTTPConfig represents HTTP/2 transport configuration
type HTTPConfig struct {
	// Host is the list of HTTP host headers
	Host []string

	// Path is the HTTP path
	Path string

	// Method is the HTTP method (usually GET)
	Method string
}

// WebSocketConfig represents WebSocket transport configuration
type WebSocketConfig struct {
	// Path is the WebSocket path
	Path string

	// Host is the WebSocket host header
	Host string
}

// QUICConfig represents QUIC transport configuration
type QUICConfig struct {
	// QUIC-specific fields can be added here as needed
}

// GRPCConfig represents gRPC transport configuration
type GRPCConfig struct {
	// ServiceName is the gRPC service name
	ServiceName string
}

// HTTPUpgradeConfig represents HTTP Upgrade transport configuration
type HTTPUpgradeConfig struct {
	// Host is the HTTP host header
	Host string

	// Path is the HTTP path
	Path string
}

// XHTTPConfig represents XHTTP transport configuration
type XHTTPConfig struct {
	Host  string
	Path  string
	Mode  string
	Extra map[string]string
}

// SplitHTTPConfig represents SplitHTTP transport configuration
type SplitHTTPConfig struct {
	Host string
	Path string
}

// KCPConfig represents mKCP transport configuration
type KCPConfig struct {
	Seed string
}

// ObfsConfig represents obfuscation configuration for protocols like Hysteria2
type ObfsConfig struct {
	// Type is the obfuscation type (e.g., salamander)
	Type string

	// Password is the obfuscation password
	Password string
}
