package chat

import "testing"

func TestInlineIDPattern(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"#fray-abc123", []string{"#fray-abc123"}},
		{"#msg-xyz789", []string{"#msg-xyz789"}},
		{"#abc123", []string{"#abc123"}},
		{"#thrd-12345678", []string{"#thrd-12345678"}},
		{"See #fray-abc and #msg-def", []string{"#fray-abc", "#msg-def"}},
		{"Fixed #fray-y590 today", []string{"#fray-y590"}},
		{"No IDs here", nil},
		{"#", nil},
		{"#ABC", nil}, // uppercase not matched
		{"#123", nil}, // must start with letter or have prefix
	}

	for _, tt := range tests {
		matches := inlineIDPattern.FindAllString(tt.input, -1)
		if len(matches) != len(tt.expected) {
			t.Errorf("input %q: got %d matches, expected %d", tt.input, len(matches), len(tt.expected))
			continue
		}
		for i, m := range matches {
			if m != tt.expected[i] {
				t.Errorf("input %q: match[%d] = %q, expected %q", tt.input, i, m, tt.expected[i])
			}
		}
	}
}
