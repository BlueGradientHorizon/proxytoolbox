package testrunner

import "fmt"

// CoreCreationError indicates failure to create a core instance
type CoreCreationError struct {
	CoreType string
	Cause    error
}

// Error implements the error interface for CoreCreationError
func (e *CoreCreationError) Error() string {
	return fmt.Sprintf("failed to create %s core: %v", e.CoreType, e.Cause)
}

// Unwrap returns the underlying cause error
func (e *CoreCreationError) Unwrap() error {
	return e.Cause
}

// CoreStartupError indicates failure to start a core instance
type CoreStartupError struct {
	CoreType string
	Cause    error
}

// Error implements the error interface for CoreStartupError
func (e *CoreStartupError) Error() string {
	return fmt.Sprintf("failed to start %s core: %v", e.CoreType, e.Cause)
}

// Unwrap returns the underlying cause error
func (e *CoreStartupError) Unwrap() error {
	return e.Cause
}

// ValidationError indicates configuration validation failures
type ValidationError struct {
	InvalidCount int
	Errors       map[string]int
}

// Error implements the error interface for ValidationError
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %d configuration(s): %v", e.InvalidCount, e.Errors)
}

// NoValidConfigsError indicates all configurations failed validation
type NoValidConfigsError struct {
	TotalCount int
	Errors     map[string]int
}

// Error implements the error interface for NoValidConfigsError
func (e *NoValidConfigsError) Error() string {
	return fmt.Sprintf("all %d configuration(s) failed validation: %v", e.TotalCount, e.Errors)
}
