package usage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCodexTranscript(t *testing.T) {
	// Create a temp file with sample Codex transcript data
	tmpDir := t.TempDir()
	transcriptPath := filepath.Join(tmpDir, "test-session.jsonl")

	// Sample Codex JSONL format
	content := `{"type":"event_msg","timestamp":"2025-01-14T10:00:00Z","payload":{"type":"token_count","info":{"model":"gpt-5","total_token_usage":{"input_tokens":1000,"cached_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":500,"total_tokens":1500}}}}
{"type":"event_msg","timestamp":"2025-01-14T10:01:00Z","payload":{"type":"token_count","info":{"model":"gpt-5","total_token_usage":{"input_tokens":2000,"cached_input_tokens":100,"cache_read_input_tokens":0,"output_tokens":1000,"total_tokens":3100}}}}
`

	if err := os.WriteFile(transcriptPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test transcript: %v", err)
	}

	usage, err := parseCodexTranscript("test-session", transcriptPath)
	if err != nil {
		t.Fatalf("parseCodexTranscript failed: %v", err)
	}

	t.Logf("Parsed usage: %+v", usage)

	if usage.Driver != "codex" {
		t.Errorf("Expected driver 'codex', got %q", usage.Driver)
	}

	if usage.Model != "gpt-5" {
		t.Errorf("Expected model 'gpt-5', got %q", usage.Model)
	}

	// Should use the last total_token_usage entry (cumulative)
	if usage.InputTokens != 2000 {
		t.Errorf("Expected 2000 input tokens, got %d", usage.InputTokens)
	}

	if usage.OutputTokens != 1000 {
		t.Errorf("Expected 1000 output tokens, got %d", usage.OutputTokens)
	}

	if usage.CachedTokens != 100 {
		t.Errorf("Expected 100 cached tokens, got %d", usage.CachedTokens)
	}
}

func TestParseCodexTranscript_ModelNameVariants(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := filepath.Join(tmpDir, "test-session.jsonl")

	// Test model_name field instead of model
	content := `{"type":"event_msg","timestamp":"2025-01-14T10:00:00Z","payload":{"type":"token_count","info":{"model_name":"gpt-5-turbo","total_token_usage":{"input_tokens":1000,"output_tokens":500}}}}
`

	if err := os.WriteFile(transcriptPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test transcript: %v", err)
	}

	usage, err := parseCodexTranscript("test-session", transcriptPath)
	if err != nil {
		t.Fatalf("parseCodexTranscript failed: %v", err)
	}

	if usage.Model != "gpt-5-turbo" {
		t.Errorf("Expected model 'gpt-5-turbo', got %q", usage.Model)
	}
}

func TestParseCodexTranscript_MetadataModel(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := filepath.Join(tmpDir, "test-session.jsonl")

	// Test metadata.model field
	content := `{"type":"event_msg","timestamp":"2025-01-14T10:00:00Z","payload":{"type":"token_count","info":{"metadata":{"model":"o1-preview"},"total_token_usage":{"input_tokens":1000,"output_tokens":500}}}}
`

	if err := os.WriteFile(transcriptPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test transcript: %v", err)
	}

	usage, err := parseCodexTranscript("test-session", transcriptPath)
	if err != nil {
		t.Fatalf("parseCodexTranscript failed: %v", err)
	}

	if usage.Model != "o1-preview" {
		t.Errorf("Expected model 'o1-preview', got %q", usage.Model)
	}
}

func TestParseCodexTranscript_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	transcriptPath := filepath.Join(tmpDir, "test-session.jsonl")

	if err := os.WriteFile(transcriptPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write test transcript: %v", err)
	}

	usage, err := parseCodexTranscript("test-session", transcriptPath)
	if err != nil {
		t.Fatalf("parseCodexTranscript failed: %v", err)
	}

	// Empty file should return zero usage
	if usage.InputTokens != 0 {
		t.Errorf("Expected 0 input tokens for empty file, got %d", usage.InputTokens)
	}
}
