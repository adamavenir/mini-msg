package daemon

import (
	"sync"
	"time"

	"github.com/adamavenir/fray/internal/usage"
)

// TokenState summarizes token state for presence detection.
type TokenState struct {
	HasInputTokens  bool
	HasOutputTokens bool
	TotalInput      int64
	TotalOutput     int64
}

// tokenCache caches transcript parsing results to avoid repeated file reads.
var tokenCache = struct {
	sync.RWMutex
	data map[string]tokenCacheEntry
}{
	data: make(map[string]tokenCacheEntry),
}

type tokenCacheEntry struct {
	state     *TokenState
	fetchedAt time.Time
}

// Fast poll TTL for spawning/prompting agents (250ms as per spec)
const tokenPollTTL = 250 * time.Millisecond

// GetTokenStateForDriver fetches token state for a session ID by parsing transcript files.
// Returns nil if token state cannot be determined.
func GetTokenStateForDriver(driver, sessionID string) *TokenState {
	if sessionID == "" {
		return nil
	}

	cacheKey := driver + ":" + sessionID

	// Check cache first
	tokenCache.RLock()
	if entry, ok := tokenCache.data[cacheKey]; ok {
		if time.Since(entry.fetchedAt) < tokenPollTTL {
			tokenCache.RUnlock()
			return entry.state
		}
	}
	tokenCache.RUnlock()

	// Parse transcript directly
	sessionUsage, err := usage.GetSessionUsageByDriver(sessionID, driver)
	if err != nil || sessionUsage == nil {
		// Cache the miss
		tokenCache.Lock()
		tokenCache.data[cacheKey] = tokenCacheEntry{state: nil, fetchedAt: time.Now()}
		tokenCache.Unlock()
		return nil
	}

	// Convert to TokenState
	state := &TokenState{
		TotalInput:      sessionUsage.InputTokens + sessionUsage.CachedTokens,
		TotalOutput:     sessionUsage.OutputTokens,
		HasInputTokens:  sessionUsage.InputTokens > 0 || sessionUsage.CachedTokens > 0,
		HasOutputTokens: sessionUsage.OutputTokens > 0,
	}

	// Cache the result
	tokenCache.Lock()
	tokenCache.data[cacheKey] = tokenCacheEntry{state: state, fetchedAt: time.Now()}
	tokenCache.Unlock()

	return state
}

// GetTokenState fetches token state for a session ID (defaults to claude driver).
// Returns nil if session not found.
func GetTokenState(sessionID string) *TokenState {
	return GetTokenStateForDriver("claude", sessionID)
}

// ClearTokenCacheForDriver clears the cache entry for a driver/session pair (call on session end).
func ClearTokenCacheForDriver(driver, sessionID string) {
	cacheKey := driver + ":" + sessionID
	tokenCache.Lock()
	delete(tokenCache.data, cacheKey)
	tokenCache.Unlock()
}

// ClearTokenCache clears the cache entry for a session (call on session end).
// This is a convenience wrapper that defaults to the claude driver.
func ClearTokenCache(sessionID string) {
	ClearTokenCacheForDriver("claude", sessionID)
}
