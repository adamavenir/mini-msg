package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/usage"
)

// tokenCache caches usage results to avoid repeated file parsing.
var tokenCache = struct {
	sync.RWMutex
	data map[string]tokenCacheEntry
}{data: make(map[string]tokenCacheEntry)}

type tokenCacheEntry struct {
	usage     *TokenUsage
	fetchedAt time.Time
}

// tokenCacheTTL controls how long token usage is cached.
// Reduced from 30s to 5s for faster feedback on active agents.
const tokenCacheTTL = 5 * time.Second

// getTokenUsage fetches token usage for a session ID using internal/usage package.
// Returns nil if session not found or no usage data.
func getTokenUsage(sessionID string) *TokenUsage {
	return getTokenUsageWithFallback(sessionID, "")
}

// getTokenUsageWithFallback fetches token usage with JSONL fallback.
// If transcript parsing fails, falls back to persisted usage snapshots.
func getTokenUsageWithFallback(sessionID, projectDBPath string) *TokenUsage {
	if sessionID == "" {
		return nil
	}

	// Check cache
	tokenCache.RLock()
	if entry, ok := tokenCache.data[sessionID]; ok {
		if time.Since(entry.fetchedAt) < tokenCacheTTL {
			tokenCache.RUnlock()
			return entry.usage
		}
	}
	tokenCache.RUnlock()

	// Use internal/usage package to get session usage from transcript
	sessionUsage, err := usage.GetSessionUsage(sessionID)
	if err != nil || sessionUsage == nil || sessionUsage.InputTokens == 0 {
		// Transcript unavailable - try fallback to persisted JSONL snapshot
		if projectDBPath != "" {
			snapshot := db.GetLatestUsageSnapshot(projectDBPath, sessionID)
			if snapshot != nil && (snapshot.InputTokens > 0 || snapshot.OutputTokens > 0) {
				tuiUsage := &TokenUsage{
					SessionID:   sessionID,
					TotalTokens: snapshot.InputTokens + snapshot.OutputTokens,
					Entries: []TokenUsageEntry{
						{
							InputTokens:     snapshot.InputTokens,
							OutputTokens:    snapshot.OutputTokens,
							CacheReadTokens: snapshot.CachedTokens,
						},
					},
				}
				// Cache with shorter TTL since this is historical data
				tokenCache.Lock()
				tokenCache.data[sessionID] = tokenCacheEntry{usage: tuiUsage, fetchedAt: time.Now()}
				tokenCache.Unlock()
				return tuiUsage
			}
		}

		// No data available
		tokenCache.Lock()
		tokenCache.data[sessionID] = tokenCacheEntry{usage: nil, fetchedAt: time.Now()}
		tokenCache.Unlock()
		return nil
	}

	// Convert to TokenUsage format expected by panels
	tuiUsage := &TokenUsage{
		SessionID:   sessionUsage.SessionID,
		TotalTokens: sessionUsage.InputTokens + sessionUsage.OutputTokens,
		Entries: []TokenUsageEntry{
			{
				InputTokens:     sessionUsage.InputTokens,
				OutputTokens:    sessionUsage.OutputTokens,
				CacheReadTokens: sessionUsage.CachedTokens,
			},
		},
	}

	// Cache the result
	tokenCache.Lock()
	tokenCache.data[sessionID] = tokenCacheEntry{usage: tuiUsage, fetchedAt: time.Now()}
	tokenCache.Unlock()

	return tuiUsage
}

// daemonLockInfo matches the daemon.lock file format.
type daemonLockInfo struct {
	PID       int   `json:"pid"`
	StartedAt int64 `json:"started_at"`
}

// readDaemonStartedAt reads the daemon's started_at timestamp from daemon.lock.
// Returns 0 if the file doesn't exist or can't be read.
func readDaemonStartedAt(projectDBPath string) int64 {
	frayDir := filepath.Dir(projectDBPath)
	lockPath := filepath.Join(frayDir, "daemon.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return 0
	}
	var info daemonLockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return 0
	}
	return info.StartedAt
}
