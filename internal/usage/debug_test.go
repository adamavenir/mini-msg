package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindClaudePaths(t *testing.T) {
	paths := getClaudePaths()
	t.Logf("Claude paths found: %v", paths)

	for _, basePath := range paths {
		// List project directories
		projectDirs, err := os.ReadDir(basePath)
		if err != nil {
			t.Logf("  Error reading %s: %v", basePath, err)
			continue
		}

		t.Logf("  Projects in %s:", basePath)
		for i, pd := range projectDirs {
			if i >= 3 {
				t.Logf("    ... and %d more", len(projectDirs)-3)
				break
			}
			if pd.IsDir() {
				// Check for sessions subdirectory
				sessionsPath := filepath.Join(basePath, pd.Name(), "sessions")
				if _, err := os.Stat(sessionsPath); err == nil {
					t.Logf("    %s (has sessions/)", pd.Name())
				} else {
					t.Logf("    %s (no sessions/)", pd.Name())
				}
			}
		}
	}
}

func TestFindClaudeTranscript(t *testing.T) {
	// Try to find a transcript
	paths := getClaudePaths()
	if len(paths) == 0 {
		t.Skip("No Claude paths available")
	}

	// Find a sample session ID
	var sampleSessionID string
	var samplePath string

	for _, basePath := range paths {
		projectDirs, err := os.ReadDir(basePath)
		if err != nil {
			continue
		}

		for _, pd := range projectDirs {
			if !pd.IsDir() {
				continue
			}

			// Check both locations: project root and sessions subdirectory
			for _, subdir := range []string{"", "sessions"} {
				searchPath := filepath.Join(basePath, pd.Name(), subdir)
				files, err := os.ReadDir(searchPath)
				if err != nil {
					continue
				}

				for _, f := range files {
					if strings.HasSuffix(f.Name(), ".jsonl") && !f.IsDir() {
						sampleSessionID = strings.TrimSuffix(f.Name(), ".jsonl")
						samplePath = filepath.Join(searchPath, f.Name())
						break
					}
				}
				if sampleSessionID != "" {
					break
				}
			}
			if sampleSessionID != "" {
				break
			}
		}
		if sampleSessionID != "" {
			break
		}
	}

	if sampleSessionID == "" {
		t.Skip("No Claude session files found")
	}

	t.Logf("Sample session ID: %s", sampleSessionID)
	t.Logf("Actual path: %s", samplePath)

	// Try to find it with findClaudeTranscript
	foundPath := findClaudeTranscript(sampleSessionID)
	t.Logf("findClaudeTranscript result: %s", foundPath)

	if foundPath == "" {
		t.Error("findClaudeTranscript failed to find the transcript")
	} else if foundPath != samplePath {
		t.Errorf("Found different path: expected %s, got %s", samplePath, foundPath)
	}
}

func TestFindMySession(t *testing.T) {
	// Test finding my specific session ID
	mySessionID := "4b6551cc-a1d8-491a-9009-a7dd04a574ca"

	foundPath := findClaudeTranscript(mySessionID)
	t.Logf("findClaudeTranscript(%s) = %s", mySessionID, foundPath)

	if foundPath == "" {
		// Try to find it manually
		paths := getClaudePaths()
		t.Logf("Searching in: %v", paths)

		for _, basePath := range paths {
			projectDirs, _ := os.ReadDir(basePath)
			for _, pd := range projectDirs {
				if !pd.IsDir() {
					continue
				}
				transcriptPath := filepath.Join(basePath, pd.Name(), mySessionID+".jsonl")
				if _, err := os.Stat(transcriptPath); err == nil {
					t.Logf("Found at: %s", transcriptPath)
				}
			}
		}
		t.Skip("Session transcript not found (test is machine-specific)")
	} else {
		// Parse and show usage
		usage, err := parseClaudeTranscript(mySessionID, foundPath)
		if err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}
		t.Logf("My usage: model=%s, input=%d, output=%d, context=%d%%",
			usage.Model, usage.InputTokens, usage.OutputTokens, usage.ContextPercent)
	}
}

func TestFindCodexPaths(t *testing.T) {
	paths := getCodexPaths()
	t.Logf("Codex paths found: %v", paths)
}

func TestCodexFindBySessionID(t *testing.T) {
	paths := getCodexPaths()
	if len(paths) == 0 {
		t.Skip("No Codex paths available")
	}

	// Find any session file
	var samplePath string
	filepath.WalkDir(paths[0], func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".jsonl") && samplePath == "" {
			samplePath = path
			return filepath.SkipAll
		}
		return nil
	})

	if samplePath == "" {
		t.Skip("No Codex session files found")
	}

	// Read the session ID from file (first line has "id" field)
	data, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		t.Skip("Empty file")
	}

	// Extract ID from first line (format: {"id":"xxx",...})
	firstLine := lines[0]
	idStart := strings.Index(firstLine, `"id":"`)
	if idStart < 0 {
		t.Skipf("Couldn't find id in first line")
	}
	idStart += 6 // len(`"id":"`)
	idEnd := strings.Index(firstLine[idStart:], `"`)
	sessionID := firstLine[idStart : idStart+idEnd]

	t.Logf("Session ID: %s", sessionID)
	t.Logf("File path: %s", samplePath)

	// Test findCodexTranscript
	foundPath := findCodexTranscript(sessionID)
	t.Logf("findCodexTranscript result: %s", foundPath)

	if foundPath != samplePath {
		t.Errorf("Expected to find %s, got %s", samplePath, foundPath)
	}
}

func TestCodexParseRealTranscript(t *testing.T) {
	paths := getCodexPaths()
	if len(paths) == 0 {
		t.Skip("No Codex paths available")
	}

	// Find the most recent .jsonl file
	var mostRecentPath string
	var mostRecentTime time.Time

	filepath.WalkDir(paths[0], func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".jsonl") {
			info, err := d.Info()
			if err == nil && info.ModTime().After(mostRecentTime) {
				mostRecentTime = info.ModTime()
				mostRecentPath = path
			}
		}
		return nil
	})

	if mostRecentPath == "" {
		t.Skip("No Codex session files found")
	}

	t.Logf("Testing with: %s", mostRecentPath)

	// Parse the transcript
	usage, err := parseCodexTranscript("test-session", mostRecentPath)
	if err != nil {
		t.Fatalf("parseCodexTranscript failed: %v", err)
	}

	t.Logf("Parsed usage:")
	t.Logf("  Model: %s", usage.Model)
	t.Logf("  Input tokens: %d", usage.InputTokens)
	t.Logf("  Output tokens: %d", usage.OutputTokens)
	t.Logf("  Cached tokens: %d", usage.CachedTokens)
	t.Logf("  Context limit: %d", usage.ContextLimit)
	t.Logf("  Context percent: %d%%", usage.ContextPercent)

	// Verify we got some data
	if usage.InputTokens == 0 && usage.OutputTokens == 0 {
		t.Error("Expected some token usage data")
	}

	if usage.Model == "" {
		t.Log("Warning: Model not detected")
	}

	if usage.ContextLimit == 0 {
		t.Error("Expected context limit to be set")
	}
}

func TestCodexEventTypes(t *testing.T) {
	paths := getCodexPaths()
	if len(paths) == 0 {
		t.Skip("No Codex paths available")
	}

	// Find most recent file
	var mostRecentPath string
	var mostRecentTime time.Time

	filepath.WalkDir(paths[0], func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".jsonl") {
			info, err := d.Info()
			if err == nil && info.ModTime().After(mostRecentTime) {
				mostRecentTime = info.ModTime()
				mostRecentPath = path
			}
		}
		return nil
	})

	if mostRecentPath == "" {
		t.Skip("No Codex session files found")
	}

	data, err := os.ReadFile(mostRecentPath)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	lines := strings.Split(string(data), "\n")

	// Collect all event types
	eventTypes := make(map[string]int)
	for _, line := range lines {
		if line == "" {
			continue
		}
		var event struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(line), &event) == nil && event.Type != "" {
			eventTypes[event.Type]++
		}
	}

	t.Logf("Event types in transcript: %v", eventTypes)

	// Check for thread-related events
	if _, ok := eventTypes["thread.started"]; ok {
		t.Log("Found thread.started events")
	} else {
		t.Log("No thread.started events found")
	}

	// Check first line for session_meta
	if len(lines) > 0 {
		var firstEvent struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if json.Unmarshal([]byte(lines[0]), &firstEvent) == nil {
			t.Logf("First event: type=%q, id=%q", firstEvent.Type, firstEvent.ID)
		}
	}
}
