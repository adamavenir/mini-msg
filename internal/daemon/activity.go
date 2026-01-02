package daemon

import (
	"os"
	"sync"
	"time"
)

// ActivityDetector detects process activity for presence tracking.
type ActivityDetector interface {
	// IsActive returns true if the process is currently active (network I/O or CPU).
	IsActive(pid int) bool

	// LastActivityTime returns when activity was last detected for the process.
	LastActivityTime(pid int) time.Time

	// IsAlive returns true if the process is still running.
	IsAlive(pid int) bool

	// RecordActivity manually records activity for a process (e.g., from stdout).
	RecordActivity(pid int)
}

// activityRecord tracks activity state for a process.
type activityRecord struct {
	lastActivity time.Time
	lastCheck    time.Time
	wasActive    bool
}

// BaseActivityDetector provides common functionality for platform-specific detectors.
type BaseActivityDetector struct {
	mu      sync.RWMutex
	records map[int]*activityRecord
}

// NewBaseActivityDetector creates a base detector.
func NewBaseActivityDetector() *BaseActivityDetector {
	return &BaseActivityDetector{
		records: make(map[int]*activityRecord),
	}
}

// getOrCreate returns the activity record for a pid, creating if needed.
func (d *BaseActivityDetector) getOrCreate(pid int) *activityRecord {
	d.mu.Lock()
	defer d.mu.Unlock()

	if rec, ok := d.records[pid]; ok {
		return rec
	}

	rec := &activityRecord{
		lastActivity: time.Now(),
		lastCheck:    time.Now(),
	}
	d.records[pid] = rec
	return rec
}

// RecordActivity marks the process as having activity now.
func (d *BaseActivityDetector) RecordActivity(pid int) {
	rec := d.getOrCreate(pid)
	d.mu.Lock()
	rec.lastActivity = time.Now()
	rec.wasActive = true
	d.mu.Unlock()
}

// LastActivityTime returns when activity was last detected.
func (d *BaseActivityDetector) LastActivityTime(pid int) time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if rec, ok := d.records[pid]; ok {
		return rec.lastActivity
	}
	return time.Time{}
}

// IsAlive returns true if the process exists.
func (d *BaseActivityDetector) IsAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds. Use Signal(0) to check if process exists.
	err = proc.Signal(nil)
	return err == nil
}

// Cleanup removes tracking for a process.
func (d *BaseActivityDetector) Cleanup(pid int) {
	d.mu.Lock()
	delete(d.records, pid)
	d.mu.Unlock()
}

// FallbackDetector provides a simple detector that relies on process liveness
// and manual activity recording. Used on platforms without OS-specific detection.
type FallbackDetector struct {
	*BaseActivityDetector
}

// NewFallbackDetector creates a detector that uses process-alive as proxy for activity.
func NewFallbackDetector() *FallbackDetector {
	return &FallbackDetector{
		BaseActivityDetector: NewBaseActivityDetector(),
	}
}

// IsActive returns true if process is alive and had recent recorded activity.
// Since we can't detect network/CPU, we rely on RecordActivity being called
// when stdout/stderr output is seen.
func (d *FallbackDetector) IsActive(pid int) bool {
	if !d.IsAlive(pid) {
		return false
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	rec, ok := d.records[pid]
	if !ok {
		// No activity record - process hasn't shown any activity yet.
		// Return false to allow spawn timeout to trigger.
		return false
	}

	// Consider active if activity in last 5 seconds
	return time.Since(rec.lastActivity) < 5*time.Second
}

// NewActivityDetector creates the appropriate detector for the current platform.
func NewActivityDetector() ActivityDetector {
	return newPlatformDetector()
}
