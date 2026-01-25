package daemon

import (
	"strings"
	"testing"

	"github.com/adamavenir/fray/internal/types"
)

func TestDetectSpawnMode(t *testing.T) {
	mode, msg := detectSpawnMode("@alice /fly build it", "alice")
	if mode != SpawnModeFly || msg != "build it" {
		t.Fatalf("expected fly with message, got %s %q", mode, msg)
	}

	mode, msg = detectSpawnMode("@alice /hop", "alice")
	if mode != SpawnModeHop || msg != "" {
		t.Fatalf("expected hop with empty message, got %s %q", mode, msg)
	}

	mode, msg = detectSpawnMode("@alice /land wrap up", "alice")
	if mode != SpawnModeLand || msg != "wrap up" {
		t.Fatalf("expected land with message, got %s %q", mode, msg)
	}

	mode, msg = detectSpawnMode("hello @alice", "alice")
	if mode != SpawnModeNormal || msg != "" {
		t.Fatalf("expected normal, got %s %q", mode, msg)
	}
}

func TestInlinePromptsIncludeAgentAndTrigger(t *testing.T) {
	agent := types.Agent{AgentID: "alice"}
	d := &Daemon{}

	fly := d.buildFlyPromptInline(agent, "do work", "Room: [msg-1]")
	if !strings.Contains(fly, "@alice") || !strings.Contains(fly, "User's task") {
		t.Fatalf("fly prompt missing agent or task context")
	}

	resume := d.buildResumePromptInline(agent, "Room: [msg-2]")
	if !strings.Contains(resume, "@alice") || !strings.Contains(resume, "Trigger messages") {
		t.Fatalf("resume prompt missing context")
	}

	land := d.buildLandPromptInline(agent, "Room: [msg-3]")
	if !strings.Contains(land, "@alice") || !strings.Contains(land, "standup") {
		t.Fatalf("land prompt missing instructions")
	}
}
