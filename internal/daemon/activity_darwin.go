//go:build darwin

package daemon

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DarwinDetector uses macOS-specific tools to detect process activity.
type DarwinDetector struct {
	*BaseActivityDetector
}

// newPlatformDetector creates the Darwin-specific detector.
func newPlatformDetector() ActivityDetector {
	return &DarwinDetector{
		BaseActivityDetector: NewBaseActivityDetector(),
	}
}

// IsActive checks for network I/O or CPU activity on Darwin.
func (d *DarwinDetector) IsActive(pid int) bool {
	if !d.IsAlive(pid) {
		return false
	}

	// Check for network activity using lsof
	if d.hasNetworkActivity(pid) {
		d.RecordActivity(pid)
		return true
	}

	// Check for CPU activity using ps
	if d.hasCPUActivity(pid) {
		d.RecordActivity(pid)
		return true
	}

	// Fall back to recent recorded activity
	d.mu.RLock()
	defer d.mu.RUnlock()

	rec, ok := d.records[pid]
	if !ok {
		// No activity record - process hasn't shown any activity yet.
		// Return false to allow spawn timeout to trigger.
		return false
	}

	return time.Since(rec.lastActivity) < 5*time.Second
}

// hasNetworkActivity checks if the process has open network connections.
func (d *DarwinDetector) hasNetworkActivity(pid int) bool {
	// lsof -p <pid> -i -a returns network file descriptors for process
	cmd := exec.Command("lsof", "-p", strconv.Itoa(pid), "-i", "-a")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// If there's any output beyond the header, there are network connections
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return len(lines) > 1
}

// hasCPUActivity checks if the process is using CPU.
func (d *DarwinDetector) hasCPUActivity(pid int) bool {
	// ps -p <pid> -o %cpu= returns CPU percentage
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "%cpu=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	cpuStr := strings.TrimSpace(string(output))
	cpu, err := strconv.ParseFloat(cpuStr, 64)
	if err != nil {
		return false
	}

	// Consider active if using more than 0.1% CPU
	return cpu > 0.1
}

// Cleanup removes tracking for a process.
func (d *DarwinDetector) Cleanup(pid int) {
	d.BaseActivityDetector.Cleanup(pid)
}
