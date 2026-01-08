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
var ccusageCache = struct {
	sync.RWMutex
	data        map[string]ccusageCacheEntry
	unavailable bool      // true if ccusage is not available (skip future calls)
	checkedAt   time.Time // when we last checked availability
}{data: make(map[string]ccusageCacheEntry)}

type ccusageCacheEntry struct {
	state     *CCUsageState
	fetchedAt time.Time
}

// Fast poll TTL for spawning/prompting agents (250ms as per spec)
const ccusageFastTTL = 250 * time.Millisecond

// How long to remember that ccusage is unavailable before retrying
const ccusageUnavailableTTL = 5 * time.Minute

// GetCCUsageState fetches token state for a session ID via ccusage.
// Returns nil if ccusage is not installed or session not found.
func GetCCUsageState(sessionID string) *CCUsageState {
	if sessionID == "" {
		return nil
	}

	// Check if ccusage was previously determined to be unavailable
	ccusageCache.RLock()
	if ccusageCache.unavailable && time.Since(ccusageCache.checkedAt) < ccusageUnavailableTTL {
		ccusageCache.RUnlock()
		return nil
	}
	// Check session cache
	if entry, ok := ccusageCache.data[sessionID]; ok {
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
	cmd := exec.CommandContext(ctx, "npx", "ccusage", "session", "--id", sessionID, "--json", "--offline")
	output, err := cmd.Output()
	if err != nil {
		// Check if this is a "command not found" type error - mark ccusage unavailable
		if ctx.Err() == context.DeadlineExceeded {
			// Timeout likely means npx is slow or ccusage isn't installed
			ccusageCache.Lock()
			ccusageCache.unavailable = true
			ccusageCache.checkedAt = time.Now()
			ccusageCache.Unlock()
		}
		// Cache the miss for this session
		ccusageCache.Lock()
		ccusageCache.data[sessionID] = ccusageCacheEntry{state: nil, fetchedAt: time.Now()}
		ccusageCache.Unlock()
		return nil
	}

	var session CCUsageSession
	if err := json.Unmarshal(output, &session); err != nil {
		ccusageCache.Lock()
		ccusageCache.data[sessionID] = ccusageCacheEntry{state: nil, fetchedAt: time.Now()}
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
	ccusageCache.data[sessionID] = ccusageCacheEntry{state: state, fetchedAt: time.Now()}
	ccusageCache.Unlock()

	return state
}

// ClearCCUsageCache clears the cache entry for a session (call on session end).
func ClearCCUsageCache(sessionID string) {
	ccusageCache.Lock()
	delete(ccusageCache.data, sessionID)
	ccusageCache.Unlock()
}
