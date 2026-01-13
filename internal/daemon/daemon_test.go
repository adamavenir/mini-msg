package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// testHarness provides a temp fray project for integration tests.
type testHarness struct {
	t           *testing.T
	projectDir  string
	projectPath string
	db          *sql.DB
	debouncer   *MentionDebouncer
}

// newTestHarness creates a temp fray project and returns the harness.
func newTestHarness(t *testing.T) *testHarness {
	t.Helper()

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	projectDir := t.TempDir()
	frayDir := filepath.Join(projectDir, ".fray")
	if err := os.MkdirAll(frayDir, 0755); err != nil {
		t.Fatalf("mkdir .fray: %v", err)
	}

	// Write minimal config
	configPath := filepath.Join(frayDir, "fray-config.json")
	if err := os.WriteFile(configPath, []byte(`{"channel_id":"ch-test","channel_name":"test"}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Create empty JSONL files so DiscoverProject finds the project
	for _, name := range []string{"messages.jsonl", "agents.jsonl"} {
		path := filepath.Join(frayDir, name)
		if err := os.WriteFile(path, []byte{}, 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	project, err := core.DiscoverProject(projectDir)
	if err != nil {
		t.Fatalf("discover project: %v", err)
	}

	database, err := db.OpenDatabase(project)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := db.InitSchema(database); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	t.Cleanup(func() {
		database.Close()
	})

	return &testHarness{
		t:           t,
		projectDir:  projectDir,
		projectPath: project.DBPath,
		db:          database,
		debouncer:   NewMentionDebouncer(database, project.DBPath),
	}
}

// createAgent creates a test agent.
func (h *testHarness) createAgent(agentID string, managed bool) types.Agent {
	h.t.Helper()
	return h.createAgentWithTrust(agentID, managed, nil)
}

func (h *testHarness) createAgentWithTrust(agentID string, managed bool, trust []string) types.Agent {
	h.t.Helper()

	now := time.Now().Unix()
	agent := types.Agent{
		AgentID:      agentID,
		RegisteredAt: now,
		LastSeen:     now,
		Managed:      managed,
		Presence:     types.PresenceOffline,
	}
	if managed {
		agent.Invoke = &types.InvokeConfig{
			Driver:         "claude",
			PromptDelivery: types.PromptDeliveryStdin,
			Trust:          trust,
		}
	}

	if err := db.CreateAgent(h.db, agent); err != nil {
		h.t.Fatalf("create agent %s: %v", agentID, err)
	}

	created, err := db.GetAgent(h.db, agentID)
	if err != nil {
		h.t.Fatalf("get agent %s: %v", agentID, err)
	}
	return *created
}

// postMessage creates a test message.
func (h *testHarness) postMessage(fromAgent, body string, msgType types.MessageType) types.Message {
	h.t.Helper()

	msg := types.Message{
		TS:        time.Now().Unix(),
		FromAgent: fromAgent,
		Body:      body,
		Type:      msgType,
		Home:      "room",
	}

	// Extract mentions
	bases, _ := db.GetAgentBases(h.db)
	msg.Mentions = core.ExtractMentions(body, bases)

	created, err := db.CreateMessage(h.db, msg)
	if err != nil {
		h.t.Fatalf("create message: %v", err)
	}
	return created
}

// postReply creates a reply to an existing message.
func (h *testHarness) postReply(fromAgent, body, replyTo string, msgType types.MessageType) types.Message {
	h.t.Helper()

	msg := types.Message{
		TS:        time.Now().Unix(),
		FromAgent: fromAgent,
		Body:      body,
		Type:      msgType,
		Home:      "room",
		ReplyTo:   &replyTo,
	}

	bases, _ := db.GetAgentBases(h.db)
	msg.Mentions = core.ExtractMentions(body, bases)

	created, err := db.CreateMessage(h.db, msg)
	if err != nil {
		h.t.Fatalf("create reply: %v", err)
	}
	return created
}

// --- Helper Function Tests (Unit-style) ---

func TestIsSelfMention(t *testing.T) {
	tests := []struct {
		name     string
		msgFrom  string
		agentID  string
		expected bool
	}{
		{"self mention", "alice", "alice", true},
		{"different agent", "bob", "alice", false},
		{"sub-agent not self", "alice.1", "alice", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := types.Message{FromAgent: tt.msgFrom}
			result := IsSelfMention(msg, tt.agentID)
			if result != tt.expected {
				t.Errorf("IsSelfMention(%q, %q) = %v, want %v", tt.msgFrom, tt.agentID, result, tt.expected)
			}
		})
	}
}

func TestIsAllMentionOnly(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		mentions []string
		agentID  string
		expected bool
	}{
		// @all only cases - should skip spawn
		{"all only", "@all hey everyone", []string{"all", "alice", "bob"}, "alice", true},
		{"all only bob", "@all hey everyone", []string{"all", "alice", "bob"}, "bob", true},
		{"all with text after", "@all please review this", []string{"all", "opus"}, "opus", true},

		// Direct mention - should NOT skip spawn
		{"direct mention", "@alice hey", []string{"alice"}, "alice", false},
		{"all plus direct", "@all @alice hey you specifically", []string{"all", "alice", "bob"}, "alice", false},
		{"direct without all", "@alice @bob hey", []string{"alice", "bob"}, "alice", false},

		// No @all in mentions - should NOT skip spawn
		{"no all", "@alice hey", []string{"alice"}, "alice", false},

		// Edge cases
		{"email not mention", "contact test@alice.com", []string{}, "alice", false},
		{"all only with punctuation", "@all!", []string{"all", "alice"}, "alice", true},
		{"agent name substring", "@all @alicebot hey", []string{"all", "alice", "alicebot"}, "alice", true},
		{"direct with dot suffix", "@alice.1 hey", []string{"alice.1"}, "alice", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := types.Message{Body: tt.body, Mentions: tt.mentions}
			result := IsAllMentionOnly(msg, tt.agentID)
			if result != tt.expected {
				t.Errorf("IsAllMentionOnly(body=%q, mentions=%v, agent=%q) = %v, want %v",
					tt.body, tt.mentions, tt.agentID, result, tt.expected)
			}
		})
	}
}

func TestIsDirectAddress(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		agentID  string
		expected bool
	}{
		// Direct address cases
		{"direct single", "@alice hey", "alice", true},
		{"direct multiple first", "@alice @bob hey", "alice", true},
		{"direct multiple second", "@alice @bob hey", "bob", true},
		{"direct with punctuation", "@alice, what do you think?", "alice", true},
		{"direct subagent", "@alice hey", "alice.1", true},
		{"direct to subagent", "@alice.1 hey", "alice.1", true},
		{"direct parent gets subagent mention", "@alice.1 hey", "alice", true},

		// NOT direct address
		{"mid-sentence mention", "hey @alice what's up", "alice", false},
		{"no @ prefix", "alice hey", "alice", false},
		{"FYI pattern", "FYI @alice this happened", "alice", false},
		{"fyi lowercase", "fyi @alice this happened", "alice", false},
		{"CC pattern", "CC @alice @bob", "alice", false},
		{"cc lowercase", "cc @alice", "alice", false},
		{"heads up pattern", "heads up @alice", "alice", false},
		{"wrong agent", "@bob hey", "alice", false},
		{"mention after text", "check this @alice", "alice", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := types.Message{Body: tt.body}
			result := IsDirectAddress(msg, tt.agentID)
			if result != tt.expected {
				t.Errorf("IsDirectAddress(%q, %q) = %v, want %v", tt.body, tt.agentID, result, tt.expected)
			}
		})
	}
}

func TestMatchesMention(t *testing.T) {
	tests := []struct {
		name     string
		mention  string
		agentID  string
		expected bool
	}{
		{"exact match", "alice", "alice", true},
		{"mention matches subagent", "alice", "alice.1", true},
		{"mention matches deep subagent", "alice", "alice.frontend.1", true},
		{"subagent mention matches parent", "alice.1", "alice", true},
		{"no match different base", "bob", "alice", false},
		{"partial no match", "ali", "alice", false},
		{"different subagent", "alice.2", "alice.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesMention(tt.mention, tt.agentID)
			if result != tt.expected {
				t.Errorf("matchesMention(%q, %q) = %v, want %v", tt.mention, tt.agentID, result, tt.expected)
			}
		})
	}
}

func TestCanTriggerSpawn(t *testing.T) {
	tests := []struct {
		name        string
		msgType     types.MessageType
		fromAgent   string
		threadOwner *string
		expected    bool
	}{
		{"human in room", types.MessageTypeUser, "adam", nil, true},
		{"agent in room", types.MessageTypeAgent, "bob", nil, false},
		{"human in owned thread", types.MessageTypeUser, "adam", strPtr("alice"), true},
		{"owner in own thread", types.MessageTypeAgent, "alice", strPtr("alice"), true},
		{"non-owner agent in thread", types.MessageTypeAgent, "bob", strPtr("alice"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := types.Message{
				Type:      tt.msgType,
				FromAgent: tt.fromAgent,
			}
			var thread *types.Thread
			if tt.threadOwner != nil {
				thread = &types.Thread{OwnerAgent: tt.threadOwner}
			}
			// Pass nil database - wake trust check will return false, matching legacy behavior
			result := CanTriggerSpawn(nil, msg, thread)
			if result != tt.expected {
				t.Errorf("CanTriggerSpawn() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCanTriggerSpawnWithWakeTrust(t *testing.T) {
	h := newTestHarness(t)

	// Create agent with wake trust
	h.createAgentWithTrust("delegator", true, []string{"wake"})

	// Agent without trust
	h.createAgent("regular", true)

	// Delegator with wake trust can trigger spawn
	msg := types.Message{
		Type:      types.MessageTypeAgent,
		FromAgent: "delegator",
	}
	if !CanTriggerSpawn(h.db, msg, nil) {
		t.Error("agent with wake trust should be able to trigger spawn")
	}

	// Regular agent without trust cannot
	msg.FromAgent = "regular"
	if CanTriggerSpawn(h.db, msg, nil) {
		t.Error("agent without wake trust should not be able to trigger spawn")
	}
}

// --- Integration Tests ---

func TestIsReplyToAgent(t *testing.T) {
	h := newTestHarness(t)

	// Create agents
	h.createAgent("alice", true)
	h.createAgent("bob", false)

	// Alice posts a message
	aliceMsg := h.postMessage("alice", "I have a question", types.MessageTypeAgent)

	// Bob replies to Alice
	bobReply := h.postReply("bob", "Here's my answer", aliceMsg.ID, types.MessageTypeUser)

	// Test: bob's reply IS a reply to alice
	if !IsReplyToAgent(h.db, bobReply, "alice") {
		t.Error("expected bob's reply to be detected as reply to alice")
	}

	// Test: bob's reply is NOT a reply to bob
	if IsReplyToAgent(h.db, bobReply, "bob") {
		t.Error("bob's reply should not be detected as reply to bob")
	}

	// Test: original message is NOT a reply to anyone
	if IsReplyToAgent(h.db, aliceMsg, "alice") {
		t.Error("original message should not be a reply")
	}
}

func TestIsReplyToAgent_SubagentMatching(t *testing.T) {
	h := newTestHarness(t)

	// Create agents - alice and alice.1 (subagent)
	h.createAgent("alice", true)
	h.createAgent("alice.1", true)
	h.createAgent("bob", false)

	// alice.1 posts a message
	subagentMsg := h.postMessage("alice.1", "From subagent", types.MessageTypeAgent)

	// Bob replies to alice.1
	bobReply := h.postReply("bob", "Reply to subagent", subagentMsg.ID, types.MessageTypeUser)

	// Parent agent "alice" should get notified of replies to "alice.1"
	if !IsReplyToAgent(h.db, bobReply, "alice") {
		t.Error("parent agent should be notified of replies to subagent")
	}

	// Subagent should also match exactly
	if !IsReplyToAgent(h.db, bobReply, "alice.1") {
		t.Error("subagent should match reply to itself")
	}
}

func TestAgentAlreadyReplied(t *testing.T) {
	h := newTestHarness(t)

	h.createAgent("alice", true)
	h.createAgent("bob", false)

	// Alice posts, Bob replies, Alice hasn't replied yet
	aliceMsg := h.postMessage("alice", "Hello", types.MessageTypeAgent)
	bobReply := h.postReply("bob", "Hi there", aliceMsg.ID, types.MessageTypeUser)

	// Alice hasn't replied to Bob's message yet
	if AgentAlreadyReplied(h.db, bobReply.ID, "alice") {
		t.Error("alice should not have replied yet")
	}

	// Alice replies to Bob's message
	h.postReply("alice", "Thanks!", bobReply.ID, types.MessageTypeAgent)

	// Now Alice has replied
	if !AgentAlreadyReplied(h.db, bobReply.ID, "alice") {
		t.Error("alice should be detected as having replied")
	}

	// Bob hasn't replied to his own message
	if AgentAlreadyReplied(h.db, bobReply.ID, "bob") {
		t.Error("bob should not have replied to his own message")
	}
}

func TestAgentAlreadyReplied_SubagentMatching(t *testing.T) {
	h := newTestHarness(t)

	h.createAgent("alice", true)
	h.createAgent("alice.1", true)
	h.createAgent("bob", false)

	// Bob posts a message
	bobMsg := h.postMessage("bob", "Hey alice team", types.MessageTypeUser)

	// alice.1 (subagent) replies
	h.postReply("alice.1", "Got it", bobMsg.ID, types.MessageTypeAgent)

	// Parent agent "alice" should be detected as having replied (via subagent)
	if !AgentAlreadyReplied(h.db, bobMsg.ID, "alice") {
		t.Error("parent should be detected when subagent replied")
	}

	// Subagent should also match exactly
	if !AgentAlreadyReplied(h.db, bobMsg.ID, "alice.1") {
		t.Error("subagent should match its own reply")
	}
}

func TestDebouncer_WatermarkTracking(t *testing.T) {
	h := newTestHarness(t)

	agent := h.createAgent("alice", true)

	// Initial watermark should be empty
	watermark := h.debouncer.GetWatermark(agent.AgentID)
	if watermark != "" {
		t.Errorf("expected empty watermark, got %q", watermark)
	}

	// Post a message
	msg := h.postMessage("bob", "@alice hey", types.MessageTypeUser)

	// Update watermark
	if err := h.debouncer.UpdateWatermark(agent.AgentID, msg.ID); err != nil {
		t.Fatalf("update watermark: %v", err)
	}

	// Verify watermark updated
	watermark = h.debouncer.GetWatermark(agent.AgentID)
	if watermark != msg.ID {
		t.Errorf("expected watermark %q, got %q", msg.ID, watermark)
	}
}

func TestDebouncer_PendingMentions(t *testing.T) {
	h := newTestHarness(t)

	h.createAgent("alice", true)

	// Queue some mentions
	h.debouncer.QueueMention("alice", "msg-1")
	h.debouncer.QueueMention("alice", "msg-2")
	h.debouncer.QueueMention("alice", "msg-1") // Duplicate - should be ignored

	// Check pending count
	if count := h.debouncer.PendingCount("alice"); count != 2 {
		t.Errorf("expected 2 pending, got %d", count)
	}

	// Flush pending
	pending := h.debouncer.FlushPending("alice")
	if len(pending) != 2 {
		t.Errorf("expected 2 flushed, got %d", len(pending))
	}

	// Pending should be empty after flush
	if h.debouncer.HasPending("alice") {
		t.Error("expected no pending after flush")
	}
}

func TestShouldSpawn_PresenceStates(t *testing.T) {
	h := newTestHarness(t)

	tests := []struct {
		name     string
		presence types.PresenceState
		expected bool
	}{
		{"offline spawns", types.PresenceOffline, true},
		{"idle spawns", types.PresenceIdle, true},
		{"empty spawns", "", true},
		{"spawning queues", types.PresenceSpawning, false},
		{"active queues", types.PresenceActive, false},
		{"error does not spawn", types.PresenceError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := types.Agent{
				AgentID:  "alice",
				Presence: tt.presence,
			}
			msg := types.Message{
				FromAgent: "bob", // Not self
				Body:      "@alice hey",
			}
			result := h.debouncer.ShouldSpawn(agent, msg)
			if result != tt.expected {
				t.Errorf("ShouldSpawn(presence=%q) = %v, want %v", tt.presence, result, tt.expected)
			}
		})
	}
}

func TestShouldSpawn_SelfMentionNeverSpawns(t *testing.T) {
	h := newTestHarness(t)

	agent := types.Agent{
		AgentID:  "alice",
		Presence: types.PresenceOffline, // Would normally spawn
	}
	msg := types.Message{
		FromAgent: "alice", // Self mention
		Body:      "@alice reminder to myself",
	}

	if h.debouncer.ShouldSpawn(agent, msg) {
		t.Error("self mention should never spawn")
	}
}

// --- End-to-End Mention Detection Tests ---

func TestMentionDetection_DirectAddressWakes(t *testing.T) {
	h := newTestHarness(t)

	alice := h.createAgent("alice", true)
	h.createAgent("bob", false)

	// Bob directly addresses alice
	msg := h.postMessage("bob", "@alice can you help?", types.MessageTypeUser)

	// Should be detected as direct address
	if !IsDirectAddress(msg, alice.AgentID) {
		t.Error("@alice at start should be direct address")
	}

	// Should trigger spawn (alice is offline)
	if !h.debouncer.ShouldSpawn(alice, msg) {
		t.Error("direct address to offline agent should spawn")
	}
}

func TestMentionDetection_FYIDoesNotWake(t *testing.T) {
	h := newTestHarness(t)

	alice := h.createAgent("alice", true)
	h.createAgent("bob", false)

	// Bob FYIs alice
	msg := h.postMessage("bob", "FYI @alice the deploy is done", types.MessageTypeUser)

	// Should NOT be direct address
	if IsDirectAddress(msg, alice.AgentID) {
		t.Error("FYI pattern should not be direct address")
	}
}

func TestMentionDetection_ChainReplyWakes(t *testing.T) {
	h := newTestHarness(t)

	alice := h.createAgent("alice", true)
	h.createAgent("bob", false)

	// Alice posts something
	aliceMsg := h.postMessage("alice", "What do you think about this approach?", types.MessageTypeAgent)

	// Bob replies (without explicit @mention)
	bobReply := h.postReply("bob", "Looks good to me", aliceMsg.ID, types.MessageTypeUser)

	// Reply should wake alice even without @mention
	if !IsReplyToAgent(h.db, bobReply, alice.AgentID) {
		t.Error("reply to alice's message should wake alice")
	}
}

// Helper
func strPtr(s string) *string {
	return &s
}

// --- Mock Driver for Spawn Testing ---

// SpawnRecord captures a Spawn call for verification.
type SpawnRecord struct {
	Agent  types.Agent
	Prompt string
}

// MockDriver implements Driver for testing spawn flow.
type MockDriver struct {
	spawns     []SpawnRecord
	spawnErr   error
	spawnProc  *Process
	cleanups   []string // sessionIDs cleaned up
	cleanupErr error
}

func NewMockDriver() *MockDriver {
	return &MockDriver{
		spawns:   []SpawnRecord{},
		cleanups: []string{},
	}
}

func (d *MockDriver) Name() string { return "mock" }

func (d *MockDriver) Spawn(ctx context.Context, agent types.Agent, prompt string) (*Process, error) {
	d.spawns = append(d.spawns, SpawnRecord{Agent: agent, Prompt: prompt})
	if d.spawnErr != nil {
		return nil, d.spawnErr
	}
	if d.spawnProc != nil {
		return d.spawnProc, nil
	}
	// Default: return a mock process that runs for 5s (long enough for tests)
	// Use CommandContext so the process gets killed when daemon stops
	cmd := exec.CommandContext(ctx, "sleep", "5")
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Process{
		Cmd:       cmd,
		StartedAt: time.Now(),
		SessionID: fmt.Sprintf("mock-session-%d", len(d.spawns)),
	}, nil
}

func (d *MockDriver) Cleanup(proc *Process) error {
	if proc != nil {
		d.cleanups = append(d.cleanups, proc.SessionID)
		// Kill the sleep process
		if proc.Cmd != nil && proc.Cmd.Process != nil {
			proc.Cmd.Process.Kill()
		}
	}
	return d.cleanupErr
}

func (d *MockDriver) SpawnCount() int {
	return len(d.spawns)
}

func (d *MockDriver) LastSpawn() *SpawnRecord {
	if len(d.spawns) == 0 {
		return nil
	}
	return &d.spawns[len(d.spawns)-1]
}

// --- Daemon Spawn Flow Tests ---

// daemonHarness extends testHarness with daemon support.
type daemonHarness struct {
	*testHarness
	daemon     *Daemon
	mockDriver *MockDriver
}

// newDaemonHarness creates a harness with a mock driver.
func newDaemonHarness(t *testing.T) *daemonHarness {
	h := newTestHarness(t)

	project, _ := core.DiscoverProject(h.projectDir)

	cfg := Config{
		PollInterval: 50 * time.Millisecond,
		Debug:        false,
	}
	daemon := New(project, h.db, cfg)

	// Inject mock driver
	mockDriver := NewMockDriver()
	daemon.drivers["mock"] = mockDriver

	return &daemonHarness{
		testHarness: h,
		daemon:      daemon,
		mockDriver:  mockDriver,
	}
}

// createManagedAgent creates a managed agent with mock driver.
func (h *daemonHarness) createManagedAgent(agentID string) types.Agent {
	h.t.Helper()

	now := time.Now().Unix()
	agent := types.Agent{
		AgentID:      agentID,
		RegisteredAt: now,
		LastSeen:     now,
		Managed:      true,
		Presence:     types.PresenceOffline,
		Invoke: &types.InvokeConfig{
			Driver:         "mock",
			PromptDelivery: types.PromptDeliveryArgs,
			SpawnTimeoutMs: 5000,
			IdleAfterMs:    100,
			MinCheckinMs:   1000,
		},
	}

	if err := db.CreateAgent(h.db, agent); err != nil {
		h.t.Fatalf("create agent %s: %v", agentID, err)
	}

	created, err := db.GetAgent(h.db, agentID)
	if err != nil {
		h.t.Fatalf("get agent %s: %v", agentID, err)
	}
	return *created
}

// simulateRunningProcess adds a mock process to daemon's tracking map.
// This simulates an agent that was spawned and is currently running.
// Call after Start() to avoid cleanup resetting the state.
func (h *daemonHarness) simulateRunningProcess(agentID string) {
	h.t.Helper()

	// Create a long-running process that won't exit during the test
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		h.t.Fatalf("simulate process: %v", err)
	}

	proc := &Process{
		Cmd:       cmd,
		StartedAt: time.Now(),
		SessionID: fmt.Sprintf("simulated-session-%s", agentID),
	}

	h.daemon.mu.Lock()
	h.daemon.processes[agentID] = proc
	h.daemon.mu.Unlock()
}

func TestSpawnFlow_DirectMention(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create managed agent
	alice := h.createManagedAgent("alice")
	h.createAgent("bob", false)

	// Bob mentions alice
	h.postMessage("bob", "@alice please help", types.MessageTypeUser)

	// Start daemon and let it poll
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	// Wait for spawn
	time.Sleep(200 * time.Millisecond)

	// Verify spawn occurred
	if h.mockDriver.SpawnCount() != 1 {
		t.Errorf("expected 1 spawn, got %d", h.mockDriver.SpawnCount())
	}

	// Verify prompt contains trigger info
	spawn := h.mockDriver.LastSpawn()
	if spawn == nil {
		t.Fatal("expected spawn record")
	}
	if !strings.Contains(spawn.Prompt, "You are @alice") {
		t.Errorf("expected prompt to say 'You are @alice', got: %s", spawn.Prompt)
	}
	// Prompt should include trigger message reference
	if !strings.Contains(spawn.Prompt, "Trigger messages") {
		t.Errorf("expected prompt to include 'Trigger messages', got: %s", spawn.Prompt)
	}

	// Verify presence updated (may be spawning, active, or idle if process exited quickly)
	updated, _ := db.GetAgent(h.db, alice.AgentID)
	validPresences := map[types.PresenceState]bool{
		types.PresenceSpawning: true,
		types.PresenceActive:   true,
		types.PresenceIdle:     true, // process may have exited already
	}
	if !validPresences[updated.Presence] {
		t.Errorf("expected presence spawning/active/idle, got %s", updated.Presence)
	}
}

func TestSpawnFlow_NoSpawnOnFYI(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create managed agent
	h.createManagedAgent("alice")
	h.createAgent("bob", false)

	// Bob FYIs alice (not direct address)
	h.postMessage("bob", "FYI @alice the build passed", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// No spawn should occur
	if h.mockDriver.SpawnCount() != 0 {
		t.Errorf("expected 0 spawns for FYI, got %d", h.mockDriver.SpawnCount())
	}
}

func TestSpawnFlow_NoSpawnOnAllMention(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create multiple managed agents
	h.createManagedAgent("alice")
	h.createManagedAgent("bob")
	h.createAgent("carol", false)

	// Carol broadcasts to @all
	h.postMessage("carol", "@all the meeting starts in 5 minutes", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// No spawn should occur - @all is ambient notification only
	if h.mockDriver.SpawnCount() != 0 {
		t.Errorf("expected 0 spawns for @all mention, got %d", h.mockDriver.SpawnCount())
	}
}

func TestSpawnFlow_SpawnOnAllPlusDirectMention(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create multiple managed agents
	h.createManagedAgent("alice")
	h.createManagedAgent("bob")
	h.createAgent("carol", false)

	// Carol broadcasts to @all but specifically addresses @alice
	h.postMessage("carol", "@all @alice can you lead the meeting?", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Only alice should spawn (direct mention), not bob (@all only)
	if h.mockDriver.SpawnCount() != 1 {
		t.Errorf("expected 1 spawn for direct mention, got %d", h.mockDriver.SpawnCount())
	}

	// Verify alice was spawned
	spawn := h.mockDriver.LastSpawn()
	if spawn == nil || spawn.Agent.AgentID != "alice" {
		t.Errorf("expected alice to spawn, got %v", spawn)
	}
}

func TestSpawnFlow_SpawnOnReply(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create agents
	h.createManagedAgent("alice")
	h.createAgent("bob", false)

	// Alice posts something (manually set presence to offline after)
	aliceMsg := h.postMessage("alice", "What do you think?", types.MessageTypeAgent)
	db.UpdateAgentPresence(h.db, "alice", types.PresenceOffline)

	// Bob replies (no @mention, but it's a reply to alice)
	h.postReply("bob", "Looks good to me!", aliceMsg.ID, types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Should spawn because bob replied to alice's message
	if h.mockDriver.SpawnCount() != 1 {
		t.Errorf("expected 1 spawn on reply, got %d", h.mockDriver.SpawnCount())
	}
}

func TestSpawnFlow_NoSpawnOnSelfMention(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create managed agent
	h.createManagedAgent("alice")

	// Alice mentions herself
	h.postMessage("alice", "@alice reminder to check this later", types.MessageTypeAgent)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// No spawn - self mentions don't trigger
	if h.mockDriver.SpawnCount() != 0 {
		t.Errorf("expected 0 spawns for self-mention, got %d", h.mockDriver.SpawnCount())
	}
}

func TestSpawnFlow_NoSpawnWhenActive(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create managed agent and bob
	alice := h.createManagedAgent("alice")
	h.createAgent("bob", false)

	// First: bob mentions alice (will trigger spawn)
	h.postMessage("bob", "@alice first request", types.MessageTypeUser)

	// Start daemon - alice will spawn (mock process runs for 5s)
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	// Wait for spawn to complete (includes 2s ccusage timeout if not installed)
	time.Sleep(3 * time.Second)

	// Verify first spawn happened
	if h.mockDriver.SpawnCount() != 1 {
		t.Fatalf("expected 1 spawn for first mention, got %d", h.mockDriver.SpawnCount())
	}

	// Now bob mentions alice again while she's still running
	h.postMessage("bob", "@alice second request", types.MessageTypeUser)

	time.Sleep(200 * time.Millisecond)

	// Should queue the second mention, not spawn again
	if h.mockDriver.SpawnCount() != 1 {
		t.Errorf("expected 1 spawn total, got %d", h.mockDriver.SpawnCount())
	}

	// Verify second mention was queued
	if !h.daemon.debouncer.HasPending(alice.AgentID) {
		t.Error("expected second mention to be queued")
	}
}

func TestSpawnFlow_WatermarkAdvances(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create agents
	alice := h.createManagedAgent("alice")
	h.createAgent("bob", false)

	// Initial watermark is empty
	wm := h.daemon.debouncer.GetWatermark(alice.AgentID)
	if wm != "" {
		t.Errorf("expected empty watermark, got %q", wm)
	}

	// Bob mentions alice
	msg := h.postMessage("bob", "@alice hello", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	// Wait for spawn to complete (includes ccusage timeout if not available)
	time.Sleep(3 * time.Second)

	// Watermark should advance past the processed message
	wm = h.daemon.debouncer.GetWatermark(alice.AgentID)
	if wm != msg.ID {
		t.Errorf("expected watermark %q, got %q", msg.ID, wm)
	}
}

// --- Session Lifecycle Tests ---

func TestSessionLifecycle_FreshSpawnSetsSessionID(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create agent with no session ID (fresh)
	alice := h.createManagedAgent("alice")
	if alice.LastSessionID != nil {
		t.Error("expected nil LastSessionID initially")
	}
	h.createAgent("bob", false)

	// Bob mentions alice
	h.postMessage("bob", "@alice hello", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	// Wait for spawn to complete (includes ccusage timeout if not available)
	time.Sleep(3 * time.Second)

	// Session ID should be set after spawn
	updated, _ := db.GetAgent(h.db, alice.AgentID)
	if updated.LastSessionID == nil || *updated.LastSessionID == "" {
		t.Error("expected LastSessionID to be set after spawn")
	}

	// Verify spawn was a fresh spawn (prompt should match)
	spawn := h.mockDriver.LastSpawn()
	if spawn == nil {
		t.Fatal("expected spawn record")
	}
}

func TestSessionLifecycle_ResumeUsesExistingSessionID(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create agent WITH existing session ID (simulates previous spawn that ended with idle)
	// Presence must be idle (not offline) for resume - offline means explicit bye (fresh start)
	existingSessionID := "existing-session-abc123"
	alice := h.createManagedAgent("alice")
	db.UpdateAgentSessionID(h.db, alice.AgentID, existingSessionID)
	db.UpdateAgentPresence(h.db, alice.AgentID, types.PresenceIdle)
	h.createAgent("bob", false)

	// Bob mentions alice
	h.postMessage("bob", "@alice help please", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// The mock driver receives the agent with LastSessionID set.
	// The actual spawn flow should pass this to the driver for resume.
	spawn := h.mockDriver.LastSpawn()
	if spawn == nil {
		t.Fatal("expected spawn record")
	}
	// Agent passed to driver should have the existing session ID
	if spawn.Agent.LastSessionID == nil || *spawn.Agent.LastSessionID != existingSessionID {
		t.Errorf("expected agent.LastSessionID %q, got %v", existingSessionID, spawn.Agent.LastSessionID)
	}
}

func TestSessionLifecycle_ByePreservesSessionIDForDisplay(t *testing.T) {
	h := newDaemonHarness(t)

	// Create agent with session ID
	alice := h.createManagedAgent("alice")
	sessionID := "session-to-preserve"
	db.UpdateAgentSessionID(h.db, alice.AgentID, sessionID)
	db.UpdateAgentPresence(h.db, alice.AgentID, types.PresenceActive)

	// Verify it's set
	updated, _ := db.GetAgent(h.db, alice.AgentID)
	if updated.LastSessionID == nil || *updated.LastSessionID != sessionID {
		t.Fatalf("setup failed: session ID not set")
	}

	// Simulate bye: set presence to offline but PRESERVE session ID (for token display)
	// This is the new behavior - session ID stays for activity panel token usage
	db.UpdateAgentPresence(h.db, alice.AgentID, types.PresenceOffline)

	// Session ID should still be present (preserved for display)
	updated, _ = db.GetAgent(h.db, alice.AgentID)
	if updated.LastSessionID == nil || *updated.LastSessionID != sessionID {
		t.Errorf("expected session ID preserved for display, got %v", updated.LastSessionID)
	}
	if updated.Presence != types.PresenceOffline {
		t.Errorf("expected offline presence, got %q", updated.Presence)
	}
}

func TestSessionLifecycle_OfflineAgentStartsFreshSession(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create agent in offline state (simulates bye) but WITH session ID (for display)
	alice := h.createManagedAgent("alice")
	oldSessionID := "old-session-from-display"
	db.UpdateAgentSessionID(h.db, alice.AgentID, oldSessionID)
	// Agent is already offline from createManagedAgent

	h.createAgent("bob", false)

	// Bob mentions alice
	h.postMessage("bob", "@alice please help", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Verify spawn occurred
	spawn := h.mockDriver.LastSpawn()
	if spawn == nil {
		t.Fatal("expected spawn record")
	}

	// Agent passed to driver should have nil session ID (fresh start for offline agents)
	// even though DB still has the old session ID for display
	if spawn.Agent.LastSessionID != nil {
		t.Errorf("expected nil LastSessionID for offline agent spawn, got %q", *spawn.Agent.LastSessionID)
	}
}

func TestSessionLifecycle_BackReactivatesAgent(t *testing.T) {
	h := newDaemonHarness(t)

	// Create agent that left
	alice := h.createManagedAgent("alice")
	now := time.Now().Unix()
	db.UpdateAgent(h.db, alice.AgentID, db.AgentUpdates{
		LeftAt: types.OptionalInt64{Set: true, Value: &now},
	})
	db.UpdateAgentPresence(h.db, alice.AgentID, types.PresenceOffline)

	// Verify agent is marked as left
	updated, _ := db.GetAgent(h.db, alice.AgentID)
	if updated.LeftAt == nil {
		t.Fatal("setup failed: agent should be marked as left")
	}

	// Simulate back: clear left_at (this is what back command does)
	db.UpdateAgent(h.db, alice.AgentID, db.AgentUpdates{
		LeftAt: types.OptionalInt64{Set: true, Value: nil},
	})

	// Agent should be reactivated
	updated, _ = db.GetAgent(h.db, alice.AgentID)
	if updated.LeftAt != nil {
		t.Error("expected left_at to be nil after back")
	}
}

func TestSessionLifecycle_IdleDoesNotPreventSpawn(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create agent in idle state (typical after session ends)
	alice := h.createManagedAgent("alice")
	db.UpdateAgentPresence(h.db, alice.AgentID, types.PresenceIdle)
	h.createAgent("bob", false)

	// Bob mentions alice
	h.postMessage("bob", "@alice are you there?", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Should spawn - idle agents can be woken
	if h.mockDriver.SpawnCount() != 1 {
		t.Errorf("expected 1 spawn for idle agent, got %d", h.mockDriver.SpawnCount())
	}
}

func TestSessionLifecycle_SessionRecordedInJSONL(t *testing.T) {
	h := newDaemonHarness(t)
	// No defer - we stop explicitly to check JSONL, then need to avoid double-stop

	// Create agents
	h.createManagedAgent("alice")
	h.createAgent("bob", false)

	// Bob mentions alice
	h.postMessage("bob", "@alice test session recording", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	// Stop daemon to ensure session end is recorded
	if err := h.daemon.Stop(); err != nil {
		t.Fatalf("stop daemon: %v", err)
	}

	// Read agents.jsonl and verify session_start was recorded
	agentsPath := filepath.Join(h.projectDir, ".fray", "agents.jsonl")
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read agents.jsonl: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "session_start") {
		t.Error("expected session_start record in agents.jsonl")
	}
}

// --- Error Recovery Tests ---

func TestErrorRecovery_SpawnFailureSetsErrorPresence(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Configure mock driver to fail on spawn
	h.mockDriver.spawnErr = fmt.Errorf("simulated spawn failure")

	// Create agents
	h.createManagedAgent("alice")
	h.createAgent("bob", false)

	// Bob mentions alice
	h.postMessage("bob", "@alice please help", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Spawn should have been attempted at least once
	// (Note: current daemon may retry multiple times per poll interval)
	if h.mockDriver.SpawnCount() < 1 {
		t.Errorf("expected at least 1 spawn attempt, got %d", h.mockDriver.SpawnCount())
	}

	// Presence should be error
	updated, _ := db.GetAgent(h.db, "alice")
	if updated.Presence != types.PresenceError {
		t.Errorf("expected presence 'error', got %q", updated.Presence)
	}
}

func TestErrorRecovery_ErrorPresenceDoesNotSpawn(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create agent in error state
	h.createManagedAgent("alice")
	db.UpdateAgentPresence(h.db, "alice", types.PresenceError)
	h.createAgent("bob", false)

	// Bob mentions alice
	h.postMessage("bob", "@alice try again", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Error state blocks auto-spawn - requires manual recovery via "fray back"
	// This prevents infinite retry loops on persistent failures
	if h.mockDriver.SpawnCount() != 0 {
		t.Errorf("expected 0 spawns for error agent, got %d", h.mockDriver.SpawnCount())
	}
}

func TestErrorRecovery_BackResetsErrorPresence(t *testing.T) {
	h := newDaemonHarness(t)

	// Create agent in error state
	h.createManagedAgent("alice")
	db.UpdateAgentPresence(h.db, "alice", types.PresenceError)

	// Verify error state
	updated, _ := db.GetAgent(h.db, "alice")
	if updated.Presence != types.PresenceError {
		t.Fatalf("setup failed: expected error presence")
	}

	// Simulate back: reset presence (this is what back command does)
	db.UpdateAgentPresence(h.db, "alice", types.PresenceOffline)

	// Agent should be offline, ready for new spawn
	updated, _ = db.GetAgent(h.db, "alice")
	if updated.Presence != types.PresenceOffline {
		t.Errorf("expected offline presence after back, got %q", updated.Presence)
	}
}

func TestErrorRecovery_ProcessExitWithErrorSetsPresence(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Configure mock driver to return a process that exits with error code
	// Note: Don't set SessionID - quick failures with session IDs are treated as
	// resume failures (graceful recovery) and go to idle instead of error
	cmd := exec.Command("sh", "-c", "exit 1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start failing process: %v", err)
	}
	h.mockDriver.spawnProc = &Process{
		Cmd:       cmd,
		StartedAt: time.Now(),
	}

	// Create agents
	h.createManagedAgent("alice")
	h.createAgent("bob", false)

	// Bob mentions alice
	h.postMessage("bob", "@alice test error exit", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	// Wait for spawn + process to exit and be detected (includes ccusage timeout)
	time.Sleep(3 * time.Second)

	// Presence should be error (non-zero exit code)
	updated, _ := db.GetAgent(h.db, "alice")
	if updated.Presence != types.PresenceError {
		t.Errorf("expected presence 'error' after failed exit, got %q", updated.Presence)
	}
}

func TestErrorRecovery_MultipleAgentsIndependentlyManaged(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create two managed agents and carol
	h.createManagedAgent("alice")
	h.createManagedAgent("bob")
	h.createAgent("carol", false)

	// Start daemon first
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	// Put alice in active state with a tracked process (already running)
	db.UpdateAgentPresence(h.db, "alice", types.PresenceActive)
	h.simulateRunningProcess("alice")

	// Carol mentions both
	h.postMessage("carol", "@alice @bob help needed", types.MessageTypeUser)

	time.Sleep(200 * time.Millisecond)

	// Bob should spawn, alice should be queued (already has running process)
	if h.mockDriver.SpawnCount() < 1 {
		t.Errorf("expected at least 1 spawn, got %d", h.mockDriver.SpawnCount())
	}

	// Verify bob was spawned
	var bobSpawned bool
	for _, spawn := range h.mockDriver.spawns {
		if spawn.Agent.AgentID == "bob" {
			bobSpawned = true
			break
		}
	}
	if !bobSpawned {
		t.Error("expected bob to spawn")
	}
}

func TestSpawnFlow_MultipleAgentsMentionedAllSpawn(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Create two managed agents and carol (human)
	h.createManagedAgent("alice")
	h.createManagedAgent("bob")
	h.createAgent("carol", false)

	// Carol mentions both agents - both should spawn
	h.postMessage("carol", "@alice @bob please collaborate on this", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	// Wait for poll cycles - spawn rate-limiting adds 500ms between spawns
	time.Sleep(700 * time.Millisecond)

	// Both should spawn
	if h.mockDriver.SpawnCount() != 2 {
		t.Errorf("expected 2 spawns for @alice @bob, got %d", h.mockDriver.SpawnCount())
	}

	// Verify both were spawned
	agentsSpawned := make(map[string]bool)
	for _, spawn := range h.mockDriver.spawns {
		agentsSpawned[spawn.Agent.AgentID] = true
	}
	if !agentsSpawned["alice"] {
		t.Error("expected alice to spawn")
	}
	if !agentsSpawned["bob"] {
		t.Error("expected bob to spawn")
	}
}

func TestErrorRecovery_SignalKillSetsIdle(t *testing.T) {
	h := newDaemonHarness(t)
	defer h.daemon.Stop()

	// Configure mock driver to return a process that gets killed by signal
	cmd := exec.Command("sh", "-c", "kill -9 $$") // Kill self with SIGKILL
	if err := cmd.Start(); err != nil {
		t.Fatalf("start signal process: %v", err)
	}
	h.mockDriver.spawnProc = &Process{
		Cmd:       cmd,
		StartedAt: time.Now(),
		SessionID: "signal-killed-session",
	}

	// Create agents
	h.createManagedAgent("alice")
	h.createAgent("bob", false)

	// Bob mentions alice
	h.postMessage("bob", "@alice test signal kill", types.MessageTypeUser)

	// Start daemon
	ctx := context.Background()
	if err := h.daemon.Start(ctx); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	// Wait for spawn + process to exit and be detected (includes ccusage timeout)
	time.Sleep(3 * time.Second)

	// Presence should be idle (signal kill exit_code=-1 should NOT be error)
	updated, _ := db.GetAgent(h.db, "alice")
	if updated.Presence != types.PresenceIdle {
		t.Errorf("expected presence 'idle' after signal kill, got %q", updated.Presence)
	}
}
