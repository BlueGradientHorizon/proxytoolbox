// Package testrunner provides a high-level abstraction for managing proxy core
// lifecycle and test execution.
package testrunner

import (
	"context"
	"fmt"

	"github.com/bluegradienthorizon/proxytoolbox/adapters/singbox"
	"github.com/bluegradienthorizon/proxytoolbox/core"
	"github.com/bluegradienthorizon/proxytoolbox/utils"
	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
)

// SingBoxCoreManager implements CoreManager for sing-box.
// It manages the lifecycle of sing-box instances and converts generic
// configurations to sing-box-specific types.
type SingBoxCoreManager struct {
	adapter  *singbox.Adapter
	logLevel string
}

// SingBoxCoreInstance implements CoreInstance for sing-box.
// It wraps a sing-box instance and provides lifecycle management
// and outbound access through the generic CoreInstance interface.
type SingBoxCoreInstance struct {
	instance *box.Box
	adapter  *singbox.Adapter
}

// NewSingBoxCoreManager creates a new sing-box core manager.
// The logLevel parameter controls sing-box's logging verbosity.
// Common values: "panic", "fatal", "error", "warn", "info", "debug", "trace"
//
// Parameters:
//   - logLevel: The logging level for sing-box (default: "panic" for minimal output)
//
// Returns:
//   - A new SingBoxCoreManager instance
func NewSingBoxCoreManager(logLevel string) *SingBoxCoreManager {
	if logLevel == "" {
		logLevel = "panic"
	}
	return &SingBoxCoreManager{
		adapter:  singbox.NewAdapter(),
		logLevel: logLevel,
	}
}

// Create creates a sing-box instance with the provided configurations.
// It validates configurations by attempting to create sing-box instances,
// collecting validation errors by type.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - configs: Slice of core-agnostic outbound configurations
//
// Returns:
//   - CoreInstance: The created sing-box instance (not yet started)
//   - map[string]int: Validation errors mapped to occurrence counts
//   - error: Error if core creation fails completely
func (m *SingBoxCoreManager) Create(ctx context.Context, configs []*core.OutboundConfig) (CoreInstance, map[string]int, error) {
	if len(configs) == 0 {
		return nil, nil, fmt.Errorf("no configurations provided")
	}

	validationErrors := make(map[string]int)
	var validOutbounds []option.Outbound

	// Convert and validate each configuration
	for _, config := range configs {
		// Convert generic config to sing-box outbound
		sbOutbound, err := m.adapter.ConvertOutbound(config)
		if err != nil {
			validationErrors[config.Type+": "+err.Error()]++
			continue
		}

		// Validate by attempting to create a temporary instance
		testCtx := include.Context(ctx)
		testInstance, err := box.New(box.Options{
			Context: testCtx,
			Options: option.Options{
				Outbounds: []option.Outbound{*sbOutbound},
			},
		})
		if err != nil {
			validationErrors[config.Type+": "+err.Error()]++
			continue
		}
		testInstance.Close()

		// Configuration is valid, add to list
		validOutbounds = append(validOutbounds, *sbOutbound)
	}

	// Check if we have any valid configurations
	if len(validOutbounds) == 0 {
		return nil, validationErrors, fmt.Errorf("no valid configurations: all %d configs failed validation", len(configs))
	}

	// Create the actual sing-box instance with all valid outbounds
	opts := option.Options{
		Log: &option.LogOptions{
			Level:     m.logLevel,
			Timestamp: true,
		},
		Outbounds: validOutbounds,
	}

	instanceCtx := include.Context(ctx)
	instance, err := box.New(box.Options{
		Context: instanceCtx,
		Options: opts,
	})
	if err != nil {
		return nil, validationErrors, fmt.Errorf("failed to create sing-box instance: %w", err)
	}

	return &SingBoxCoreInstance{
		instance: instance,
		adapter:  m.adapter,
	}, validationErrors, nil
}

// Info returns metadata about the sing-box core implementation.
//
// Returns:
//   - CoreInfo with name, version, and type information
func (m *SingBoxCoreManager) Info() CoreInfo {
	return SingBoxCoreInfo()
}

// SingBoxCoreInfo returns static metadata about the sing-box core.
// This function can be called without creating a manager instance.
//
// Returns:
//   - CoreInfo with name, version, and type information
func SingBoxCoreInfo() CoreInfo {
	return CoreInfo{
		Name:    "sing-box",
		Version: utils.GetModuleVersion("github.com/sagernet/sing-box"),
		Type:    SingBoxCore,
	}
}

// Start starts the sing-box instance, making it ready to handle connections.
// Must be called after Create and before using outbounds.
//
// Returns:
//   - error: Error if startup fails
func (i *SingBoxCoreInstance) Start() error {
	if i.instance == nil {
		return fmt.Errorf("cannot start: instance is nil")
	}
	return i.instance.Start()
}

// Stop stops the sing-box instance and cleans up all resources.
// Should be called when the instance is no longer needed.
//
// Returns:
//   - error: Error if shutdown fails (logged but not critical)
func (i *SingBoxCoreInstance) Stop() error {
	if i.instance == nil {
		return nil // Already stopped or never started
	}
	return i.instance.Close()
}

// GetOutbounds returns all configured outbounds from the running instance.
// The outbounds are wrapped to implement the generic core.Outbound interface.
//
// Returns:
//   - Slice of core.Outbound interfaces wrapping sing-box outbounds
func (i *SingBoxCoreInstance) GetOutbounds() []core.Outbound {
	if i.instance == nil {
		return nil
	}

	sbOutbounds := i.instance.Outbound().Outbounds()
	wrapped := make([]core.Outbound, 0, len(sbOutbounds))
	for _, sbOut := range sbOutbounds {
		wrapped = append(wrapped, singbox.NewOutboundWrapper(sbOut))
	}
	return wrapped
}

// GetOutboundByTag retrieves a specific outbound by its tag.
// The outbound is wrapped to implement the generic core.Outbound interface.
//
// Parameters:
//   - tag: The unique identifier of the outbound to retrieve
//
// Returns:
//   - core.Outbound: The wrapped outbound
//   - error: Error if the tag is not found
func (i *SingBoxCoreInstance) GetOutboundByTag(tag string) (core.Outbound, error) {
	if i.instance == nil {
		return nil, fmt.Errorf("instance is nil")
	}

	sbOutbounds := i.instance.Outbound().Outbounds()
	for _, sbOut := range sbOutbounds {
		if sbOut.Tag() == tag {
			return singbox.NewOutboundWrapper(sbOut), nil
		}
	}

	return nil, fmt.Errorf("outbound with tag '%s' not found", tag)
}
