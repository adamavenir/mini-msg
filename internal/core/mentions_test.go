package core

import "testing"

func TestExtractMentionsWithBases(t *testing.T) {
	bases := map[string]struct{}{
		"alice": {},
		"bob":   {},
	}

	body := "hey @alice and @bob.1 and email test@test.com @all @unknown"
	mentions := ExtractMentions(body, bases)

	if len(mentions) != 3 {
		t.Fatalf("expected 3 mentions, got %d", len(mentions))
	}
	assertMention(t, mentions, "alice")
	assertMention(t, mentions, "bob.1")
	assertMention(t, mentions, "all")
}

func assertMention(t *testing.T, mentions []string, value string) {
	t.Helper()
	for _, mention := range mentions {
		if mention == value {
			return
		}
	}
	t.Fatalf("expected mention %s", value)
}

func TestExtractInterruptSyntax(t *testing.T) {
	bases := map[string]struct{}{
		"alice": {},
		"bob":   {},
	}

	tests := []struct {
		name           string
		body           string
		expectAgents   []string
		expectDouble   map[string]bool
		expectNoSpawn  map[string]bool
	}{
		{
			name:         "single interrupt",
			body:         "!@alice need your help",
			expectAgents: []string{"alice"},
			expectDouble: map[string]bool{"alice": false},
			expectNoSpawn: map[string]bool{"alice": false},
		},
		{
			name:         "double interrupt fresh start",
			body:         "!!@bob start fresh",
			expectAgents: []string{"bob"},
			expectDouble: map[string]bool{"bob": true},
			expectNoSpawn: map[string]bool{"bob": false},
		},
		{
			name:         "interrupt no spawn",
			body:         "!@alice! stop now",
			expectAgents: []string{"alice"},
			expectDouble: map[string]bool{"alice": false},
			expectNoSpawn: map[string]bool{"alice": true},
		},
		{
			name:         "double interrupt no spawn",
			body:         "!!@bob! force end",
			expectAgents: []string{"bob"},
			expectDouble: map[string]bool{"bob": true},
			expectNoSpawn: map[string]bool{"bob": true},
		},
		{
			name:         "multiple interrupts",
			body:         "!@alice and !!@bob!",
			expectAgents: []string{"alice", "bob"},
			expectDouble: map[string]bool{"alice": false, "bob": true},
			expectNoSpawn: map[string]bool{"alice": false, "bob": true},
		},
		{
			name:         "mixed regular and interrupt",
			body:         "@alice regular mention and !@bob interrupt",
			expectAgents: []string{"alice", "bob"},
			expectDouble: map[string]bool{"bob": false},
			expectNoSpawn: map[string]bool{"bob": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractMentionsWithSession(tt.body, bases)

			// Check all expected agents are mentioned
			for _, agent := range tt.expectAgents {
				found := false
				for _, m := range result.Mentions {
					if m == agent {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected mention for %s", agent)
				}
			}

			// Check interrupt info
			for agent, expectedDouble := range tt.expectDouble {
				info, ok := result.Interrupts[agent]
				if !ok {
					t.Errorf("expected interrupt info for %s", agent)
					continue
				}
				if info.Double != expectedDouble {
					t.Errorf("agent %s: expected Double=%v, got %v", agent, expectedDouble, info.Double)
				}
			}

			for agent, expectedNoSpawn := range tt.expectNoSpawn {
				info, ok := result.Interrupts[agent]
				if !ok {
					continue // Already reported above
				}
				if info.NoSpawn != expectedNoSpawn {
					t.Errorf("agent %s: expected NoSpawn=%v, got %v", agent, expectedNoSpawn, info.NoSpawn)
				}
			}
		})
	}
}
