// Package testrunner provides a high-level abstraction for managing proxy core
// lifecycle and test execution.
package testrunner

import "fmt"

// GetSupportedCores returns a list of all proxy cores supported by this library.
// This function returns static information without creating manager instances.
//
// Returns:
//   - Slice of CoreInfo for all supported cores
func GetSupportedCores() []CoreInfo {
	return []CoreInfo{
		SingBoxCoreInfo(),
	}
}

// GetCoreManager creates and returns a core manager for the specified core type.
//
// Parameters:
//   - coreType: The type of core to get a manager for
//   - logLevel: Log level for the core (e.g., "panic", "error", "warn", "info", "debug")
//
// Returns:
//   - CoreManager: The core manager instance
//   - error: Error if the core type is not supported
func GetCoreManager(coreType CoreType, logLevel string) (CoreManager, error) {
	switch coreType {
	case SingBoxCore:
		return NewSingBoxCoreManager(logLevel), nil
	default:
		return nil, fmt.Errorf("unsupported core type '%s'", coreType)
	}
}
