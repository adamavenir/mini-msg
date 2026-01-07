package command

import "testing"

func TestSanitizeThreadName(t *testing.T) {
	tests := []struct {
		input    string
		want     string
		wantDiff bool
	}{
		// Already valid
		{"my-thread", "my-thread", false},
		{"design", "design", false},
		{"v2-api", "v2-api", false},

		// PascalCase gets hyphenated at case boundaries
		{"MyThread", "my-thread", true},
		// All uppercase stays together (no hyphens between consecutive uppercase)
		{"ALLCAPS", "allcaps", true},

		// Spaces to hyphens
		{"my thread name", "my-thread-name", true},

		// Underscores to hyphens
		{"my_thread_name", "my-thread-name", true},

		// CamelCase to kebab-case (hyphen before uppercase following lowercase)
		{"camelCase", "camel-case", true},
		{"PascalCase", "pascal-case", true},
		{"XMLParser", "xmlparser", true}, // All caps followed by Pascal = no hyphen before X

		// Mixed cases
		{"My Thread_Name", "my-thread-name", true},
		{"  Some_Mixed Case  ", "some-mixed-case", true},

		// Invalid characters removed
		{"my@thread!", "mythread", true},
		{"name#123", "name123", true},

		// Edge cases
		{"", "", false},
		{"   ", "", false},
		{"---", "", false}, // Already empty after trim, no diff
		{"123abc", "123abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, diff := SanitizeThreadName(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeThreadName(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if diff != tt.wantDiff {
				t.Errorf("SanitizeThreadName(%q) diff = %v, want %v", tt.input, diff, tt.wantDiff)
			}
		})
	}
}
