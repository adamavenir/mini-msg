package daemon

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"time"
)

// CCUsageSession holds token usage data from ccusage for presence detection.
type CCUsageSession struct {
	SessionID   string `json:"sessionId"`
	TotalCost   float64
	TotalTokens int64
	Entries     []CCUsageEntry `json:"entries"`
}

// CCUsageEntry represents a single API call entry.
type CCUsageEntry struct {
	Timestamp           string  `json:"timestamp"`
	InputTokens         int64   `json:"inputTokens"`
	OutputTokens        int64   `json:"outputTokens"`
	CacheCreationTokens int64   `json:"cacheCreationTokens"`
	CacheReadTokens     int64   `json:"cacheReadTokens"`
	Model               string  `json:"model"`
	CostUSD             float64 `json:"costUSD"`
}

// CCUsageState summarizes token state for presence detection.
type CCUsageState struct {
	HasInputTokens  bool
	HasOutputTokens bool
	TotalInput      int64
	TotalOutput     int64
}

// ccusageCache caches ccusage results to avoid repeated shell calls.
// Each driver has its own availability tracking since they use different packages.
var ccusageCache = struct {
	sync.RWMutex
	data        map[string]ccusageCacheEntry
	unavailable map[string]time.Time // driver -> when marked unavailable
}{
	data:        make(map[string]ccusageCacheEntry),
	unavailable: make(map[string]time.Time),
}

type ccusageCacheEntry struct {
	state     *CCUsageState
	fetchedAt time.Time
}

// Fast poll TTL for spawning/prompting agents (250ms as per spec)
const ccusageFastTTL = 250 * time.Millisecond

// How long to remember that ccusage is unavailable before retrying
const ccusageUnavailableTTL = 5 * time.Minute

// ccusageCommand returns the npx command args for a given driver.
// Returns nil for drivers without ccusage support.
func ccusageCommand(driver, sessionID string) []string {
	switch driver {
	case "claude", "":
		// ccusage (main package) for Claude Code
		return []string{"npx", "ccusage", "session", "--id", sessionID, "--json", "--offline"}
	case "codex":
		// @ccusage/codex for OpenAI Codex CLI
		return []string{"npx", "@ccusage/codex", "session", "--id", sessionID, "--json", "--offline"}
	case "opencode":
		// @ccusage/opencode doesn't exist yet (PR #776 in progress)
		// Return nil to gracefully degrade - presence detection will use fallback heuristics
		return nil
	default:
		return nil
	}
}

// GetCCUsageStateForDriver fetches token state for a session ID via the driver-specific ccusage package.
// Returns nil if the package is not installed, not available for the driver, or session not found.
func GetCCUsageStateForDriver(driver, sessionID string) *CCUsageState {
	if sessionID == "" {
		return nil
	}

	// Get command for this driver
	cmdArgs := ccusageCommand(driver, sessionID)
	if cmdArgs == nil {
		// Driver doesn't have ccusage support
		return nil
	}

	cacheKey := driver + ":" + sessionID

	// Check if this driver's ccusage was previously determined to be unavailable
	ccusageCache.RLock()
	if unavailAt, ok := ccusageCache.unavailable[driver]; ok {
		if time.Since(unavailAt) < ccusageUnavailableTTL {
			ccusageCache.RUnlock()
			return nil
		}
	}
	// Check session cache
	if entry, ok := ccusageCache.data[cacheKey]; ok {
		if time.Since(entry.fetchedAt) < ccusageFastTTL {
			ccusageCache.RUnlock()
			return entry.state
		}
	}
	ccusageCache.RUnlock()

	// Call ccusage with timeout - 2s is enough for npx to start and query
	// Returns nil on timeout so presence detection degrades gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	output, err := cmd.Output()
	if err != nil {
		// Check if this is a "command not found" type error - mark this driver's ccusage unavailable
		if ctx.Err() == context.DeadlineExceeded {
			// Timeout likely means npx is slow or package isn't installed
			ccusageCache.Lock()
			ccusageCache.unavailable[driver] = time.Now()
			ccusageCache.Unlock()
		}
		// Cache the miss for this session
		ccusageCache.Lock()
		ccusageCache.data[cacheKey] = ccusageCacheEntry{state: nil, fetchedAt: time.Now()}
		ccusageCache.Unlock()
		return nil
	}

	var session CCUsageSession
	if err := json.Unmarshal(output, &session); err != nil {
		ccusageCache.Lock()
		ccusageCache.data[cacheKey] = ccusageCacheEntry{state: nil, fetchedAt: time.Now()}
		ccusageCache.Unlock()
		return nil
	}

	// Aggregate tokens across all entries
	state := &CCUsageState{}
	for _, entry := range session.Entries {
		state.TotalInput += entry.InputTokens + entry.CacheCreationTokens + entry.CacheReadTokens
		state.TotalOutput += entry.OutputTokens
	}
	state.HasInputTokens = state.TotalInput > 0
	state.HasOutputTokens = state.TotalOutput > 0

	// Cache the result
	ccusageCache.Lock()
	ccusageCache.data[cacheKey] = ccusageCacheEntry{state: state, fetchedAt: time.Now()}
	ccusageCache.Unlock()

	return state
}

// GetCCUsageState fetches token state for a session ID via ccusage (Claude Code).
// This is a convenience wrapper that defaults to the claude driver.
// Returns nil if ccusage is not installed or session not found.
func GetCCUsageState(sessionID string) *CCUsageState {
	return GetCCUsageStateForDriver("claude", sessionID)
}

// ClearCCUsageCacheForDriver clears the cache entry for a driver/session pair (call on session end).
func ClearCCUsageCacheForDriver(driver, sessionID string) {
	cacheKey := driver + ":" + sessionID
	ccusageCache.Lock()
	delete(ccusageCache.data, cacheKey)
	ccusageCache.Unlock()
}

// ClearCCUsageCache clears the cache entry for a session (call on session end).
// This is a convenience wrapper that defaults to the claude driver.
func ClearCCUsageCache(sessionID string) {
	ClearCCUsageCacheForDriver("claude", sessionID)
}
