//go:build !darwin

package daemon

// newPlatformDetector creates the fallback detector for non-Darwin platforms.
// Linux and Windows use process-alive-only detection until OS-specific adapters are added.
func newPlatformDetector() ActivityDetector {
	return NewFallbackDetector()
}
