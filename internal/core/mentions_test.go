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
