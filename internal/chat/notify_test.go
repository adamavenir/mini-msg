package chat

import "testing"

func TestIsDirectMention(t *testing.T) {
	tests := []struct {
		body    string
		agentID string
		want    bool
	}{
		// Direct mentions at start
		{"@opus check this", "opus", true},
		{"@opus.frontend check this", "opus.frontend", true},
		{"@op check this", "opus", false}, // partial string match not sufficient

		// Hierarchical matching: @opus notifies opus.frontend (parent prefix matches child)
		{"@opus check this", "opus.frontend", true},
		// But @opus.frontend does NOT notify opus (child doesn't notify parent)
		{"@opus.frontend check this", "opus", false},

		// Not direct mentions (FYI)
		{"hey @opus", "opus", false},
		{"check this @opus", "opus", false},
		{" @opus leading space", "opus", false},

		// Multiple mentions - only first counts as direct
		{"@alice @opus do this", "opus", false},
		{"@alice @opus do this", "alice", true},

		// @all is not a direct mention (it's broadcast, handled separately)
		{"@all announcement", "opus", false},

		// Edge cases
		{"", "opus", false},
		{"no mentions", "opus", false},
		{"@OPUS uppercase", "opus", false}, // mentions are lowercase
	}

	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			got := IsDirectMention(tt.body, tt.agentID)
			if got != tt.want {
				t.Errorf("IsDirectMention(%q, %q) = %v, want %v", tt.body, tt.agentID, got, tt.want)
			}
		})
	}
}

func TestTruncateNotification(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 100, "short"},
		{"hello\nworld", 100, "hello world"},
		{"  multiple   spaces  ", 100, "multiple spaces"},
		{"this is a long message that needs truncation", 20, "this is a long mess…"},
		{"", 100, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncateNotification(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateNotification(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}
