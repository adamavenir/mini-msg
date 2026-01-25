package chat

import (
	"testing"
)

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		haystack string
		needle   string
		want     bool
	}{
		// Empty needle matches everything
		{"fave", "", true},
		{"quit", "", true},
		// Exact prefix matches
		{"fave", "f", true},
		{"fave", "fa", true},
		{"fave", "fav", true},
		{"fave", "fave", true},
		// Fuzzy matches (chars in order)
		{"unfave", "uf", true},
		{"unfave", "ufa", true},
		{"archive", "ar", true},
		{"archive", "arc", true},
		{"archive", "arv", true}, // a-r-chi-v-e
		{"rename", "rn", true},   // r-e-n-ame
		{"rename", "rm", true},   // r-ena-m-e (fuzzy: r then m)
		// No match
		{"fave", "x", false},
		{"fave", "vf", false}, // out of order
		{"quit", "abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.haystack+"_"+tt.needle, func(t *testing.T) {
			got := fuzzyMatch(tt.haystack, tt.needle)
			if got != tt.want {
				t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
			}
		})
	}
}

func TestBuildCommandSuggestions(t *testing.T) {
	tests := []struct {
		prefix     string
		wantFirst  string // first suggestion's Insert value
		wantMinLen int    // minimum number of suggestions
	}{
		// Empty shows all commands (up to limit)
		{"", "/quit", 8},
		// Prefix filters
		{"q", "/quit", 1},
		{"f", "/fave", 2},    // fave, follow
		{"un", "/unfave", 3}, // unfave, unfollow, unmute
		// Fuzzy matching
		{"uf", "/unfave", 1},
		{"rm", "/rm", 1},
		// No match
		{"xyz", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			got := buildCommandSuggestions(tt.prefix)
			if len(got) < tt.wantMinLen {
				t.Errorf("buildCommandSuggestions(%q) returned %d suggestions, want at least %d", tt.prefix, len(got), tt.wantMinLen)
			}
			if tt.wantFirst != "" && len(got) > 0 && got[0].Insert != tt.wantFirst {
				t.Errorf("buildCommandSuggestions(%q)[0].Insert = %q, want %q", tt.prefix, got[0].Insert, tt.wantFirst)
			}
		})
	}
}
