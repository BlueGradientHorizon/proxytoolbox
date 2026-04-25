// Package testrunner provides a high-level abstraction for managing proxy core
// lifecycle and test execution. It offers both convenience methods that handle
// everything automatically and low-level APIs for fine-grained control.
package testrunner

import (
	"context"

	"github.com/bluegradienthorizon/proxytoolbox/core"
)

// CoreType identifies supported proxy cores
type CoreType string

const (
	// SingBoxCore represents the sing-box proxy core
	SingBoxCore CoreType = "singbox"
	// Future: XrayCore, ClashCore, etc.
)

// CoreManager manages the lifecycle of a proxy core instance.
// Different proxy cores (sing-box, xray, clash) implement this interface
// to provide pluggable core support without modifying the test runner.
type CoreManager interface {
	// Create creates a new core instance with the provided configurations.
	// It validates configurations and returns a CoreInstance along with
	// validation errors grouped by error message.
	//
	// Parameters:
	//   - ctx: Context for cancellation and timeout control
	//   - configs: Slice of core-agnostic outbound configurations
	//
	// Returns:
	//   - CoreInstance: The created core instance (not yet started)
	//   - map[string]int: Validation errors mapped to occurrence counts
	//   - error: Error if core creation fails completely
	Create(ctx context.Context, configs []*core.OutboundConfig) (CoreInstance, map[string]int, error)

	// Info returns metadata about this core implementation including
	// name, version, and type identifier.
	Info() CoreInfo
}

// CoreInstance represents a running proxy core instance.
// It provides methods for lifecycle management and outbound access.
type CoreInstance interface {
	// Start starts the core instance, making it ready to handle connections.
	// Must be called after Create and before using outbounds.
	Start() error

	// Stop stops the core instance and cleans up all resources.
	// Should be called when the instance is no longer needed.
	Stop() error

	// GetOutbounds returns all configured outbounds from the running instance.
	// The outbounds can be used with testers for latency and speed testing.
	GetOutbounds() []core.Outbound

	// GetOutboundByTag retrieves a specific outbound by its tag.
	// Returns an error if the tag is not found.
	GetOutboundByTag(tag string) (core.Outbound, error)
}

// CoreInfo contains metadata about a proxy core implementation.
// This information is used for discovery and display purposes.
type CoreInfo struct {
	// Name is the human-readable name (e.g., "sing-box")
	Name string

	// Version is the core version (e.g., "v1.12.17")
	Version string

	// Type is the internal identifier used in the registry
	Type CoreType
}
