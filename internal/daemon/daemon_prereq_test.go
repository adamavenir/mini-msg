package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

func TestDaemonAcquireLockHandlesStaleLock(t *testing.T) {
	h := newTestHarness(t)
	project, err := core.DiscoverProject(h.projectDir)
	if err != nil {
		t.Fatalf("discover project: %v", err)
	}
	daemon := New(project, h.db, Config{})

	stale := LockInfo{PID: 999999, StartedAt: 1}
	data, _ := json.Marshal(stale)
	if err := os.WriteFile(daemon.lockPath, data, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	if err := daemon.acquireLock(); err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	defer daemon.releaseLock()

	lockedData, err := os.ReadFile(daemon.lockPath)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	var got LockInfo
	if err := json.Unmarshal(lockedData, &got); err != nil {
		t.Fatalf("unmarshal lock: %v", err)
	}
	if got.PID != os.Getpid() {
		t.Fatalf("expected lock PID %d, got %d", os.Getpid(), got.PID)
	}
}

func TestDaemonAcquireLockDetectsRunning(t *testing.T) {
	h := newTestHarness(t)
	project, err := core.DiscoverProject(h.projectDir)
	if err != nil {
		t.Fatalf("discover project: %v", err)
	}
	daemon := New(project, h.db, Config{})

	running := LockInfo{PID: os.Getpid(), StartedAt: time.Now().Unix()}
	data, _ := json.Marshal(running)
	if err := os.WriteFile(daemon.lockPath, data, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	if err := daemon.acquireLock(); err == nil {
		t.Fatalf("expected error for running lock")
	}
	_ = os.Remove(daemon.lockPath)
}

func TestIsLockedReportsRunningAndStale(t *testing.T) {
	frayDir := t.TempDir()
	lockPath := filepath.Join(frayDir, "daemon.lock")

	running := LockInfo{PID: os.Getpid(), StartedAt: time.Now().Unix()}
	data, _ := json.Marshal(running)
	if err := os.WriteFile(lockPath, data, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	if !IsLocked(frayDir) {
		t.Fatalf("expected IsLocked true for running pid")
	}

	stale := LockInfo{PID: 999999, StartedAt: 1}
	data, _ = json.Marshal(stale)
	if err := os.WriteFile(lockPath, data, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	if IsLocked(frayDir) {
		t.Fatalf("expected IsLocked false for stale pid")
	}
}

func TestIsActiveByTokensUsesTimeWindow(t *testing.T) {
	h := newTestHarness(t)
	project, err := core.DiscoverProject(h.projectDir)
	if err != nil {
		t.Fatalf("discover project: %v", err)
	}
	daemon := New(project, h.db, Config{})

	sessionID := "sess-1"
	nowMs := time.Now().UnixMilli()
	agent := types.Agent{
		AgentID:         "alice",
		LastSessionID:   &sessionID,
		TokensUpdatedAt: nowMs,
		Invoke: &types.InvokeConfig{
			IdleAfterMs: 100,
		},
	}

	if !daemon.isActiveByTokens(agent) {
		t.Fatalf("expected active when tokens updated recently")
	}

	agent.TokensUpdatedAt = nowMs - 1000
	if daemon.isActiveByTokens(agent) {
		t.Fatalf("expected inactive when tokens are stale")
	}
}

func TestCleanupStalePresenceResetsActiveAgents(t *testing.T) {
	h := newTestHarness(t)
	project, err := core.DiscoverProject(h.projectDir)
	if err != nil {
		t.Fatalf("discover project: %v", err)
	}
	daemon := New(project, h.db, Config{})

	agent := h.createAgent("alice", true)
	if err := db.UpdateAgentPresence(h.db, agent.AgentID, types.PresenceActive); err != nil {
		t.Fatalf("update presence: %v", err)
	}

	daemon.cleanupStalePresence()

	updated, err := db.GetAgent(h.db, agent.AgentID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if updated.Presence != types.PresenceOffline {
		t.Fatalf("expected presence offline, got %s", updated.Presence)
	}
	if updated.LeftAt == nil {
		t.Fatalf("expected left_at set after cleanup")
	}
}

func TestCaptureUsageSnapshotWritesRecord(t *testing.T) {
	h := newTestHarness(t)
	project, err := core.DiscoverProject(h.projectDir)
	if err != nil {
		t.Fatalf("discover project: %v", err)
	}
	daemon := New(project, h.db, Config{})

	agent := h.createAgent("alice", true)
	sessionID := "sess-usage-1"

	// Create a Claude transcript file in the expected location.
	home := os.Getenv("HOME")
	transcriptDir := filepath.Join(home, ".config", "claude", "projects", "test-project")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatalf("mkdir transcript dir: %v", err)
	}
	transcriptPath := filepath.Join(transcriptDir, sessionID+".jsonl")
	line := `{"sessionId":"` + sessionID + `","message":{"usage":{"input_tokens":10,"output_tokens":3,"cache_creation_input_tokens":0,"cache_read_input_tokens":5},"model":"sonnet"}}`
	if err := os.WriteFile(transcriptPath, []byte(line+"\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	daemon.captureUsageSnapshot(agent.AgentID, sessionID)

	agentsPath := filepath.Join(h.projectDir, ".fray", "agents.jsonl")
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read agents jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatalf("expected usage snapshot line")
	}
	var record db.UsageSnapshotJSONLRecord
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &record); err != nil {
		t.Fatalf("unmarshal usage snapshot: %v", err)
	}
	if record.Type != "usage_snapshot" {
		t.Fatalf("expected usage_snapshot type, got %s", record.Type)
	}
	if record.InputTokens != 15 || record.OutputTokens != 3 {
		t.Fatalf("unexpected tokens: input=%d output=%d", record.InputTokens, record.OutputTokens)
	}
}
