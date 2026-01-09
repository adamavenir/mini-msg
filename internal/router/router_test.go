package router

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRouterUnavailableReturnsDefault(t *testing.T) {
	// Router with non-existent directory should be unavailable
	r := New("/nonexistent/path")
	if r.Available() {
		t.Error("expected router to be unavailable for nonexistent path")
	}

	// Should return default result
	result := r.Route(RouterPayload{
		Message: "test message",
		From:    "user",
		Agent:   "agent",
	})

	if result.Mode != ModeDeepWork {
		t.Errorf("expected default mode deep-work, got %s", result.Mode)
	}
	if !result.ShouldSpawn {
		t.Error("expected default shouldSpawn=true")
	}
	if result.Confidence != 0.5 {
		t.Errorf("expected default confidence 0.5, got %f", result.Confidence)
	}
}

func TestRouterWithValidFile(t *testing.T) {
	// Create temp directory with router file
	dir := t.TempDir()
	llmDir := filepath.Join(dir, "llm")
	if err := os.MkdirAll(llmDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a minimal router that outputs JSON using new mlld syntax
	routerContent := `/import { @message } from @payload

/exe @hasFyi(text) = js { return text.toLowerCase().includes("fyi"); }

/exe @route(msg) = when first [
  @hasFyi(@msg) => { mode: "acknowledge", shouldSpawn: false, confidence: 0.9 }
  * => { mode: "deep-work", shouldSpawn: true, confidence: 0.5 }
]

/show @route(@message)
`
	if err := os.WriteFile(filepath.Join(llmDir, "router.mld"), []byte(routerContent), 0o644); err != nil {
		t.Fatal(err)
	}

	r := New(dir)
	if !r.Available() {
		t.Skip("mlld not available, skipping router execution test")
	}

	// Test FYI message (should not spawn)
	result := r.Route(RouterPayload{
		Message: "fyi just letting you know",
		From:    "user",
		Agent:   "agent",
	})
	if result.ShouldSpawn {
		t.Error("expected shouldSpawn=false for FYI message")
	}
	if result.Mode != ModeAcknowledge {
		t.Errorf("expected mode acknowledge, got %s", result.Mode)
	}
}
