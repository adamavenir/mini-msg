package command

import "testing"

func TestParseDuration(t *testing.T) {
	seconds, err := parseDuration("30m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seconds != 1800 {
		t.Fatalf("expected 1800, got %d", seconds)
	}

	seconds, err = parseDuration("2h")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seconds != 7200 {
		t.Fatalf("expected 7200, got %d", seconds)
	}

	seconds, err = parseDuration("1d")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seconds != 86400 {
		t.Fatalf("expected 86400, got %d", seconds)
	}

	if _, err := parseDuration("10x"); err == nil {
		t.Fatalf("expected error for invalid duration")
	}
}

func TestSplitCommaList(t *testing.T) {
	items := splitCommaList("a, b, ,c")
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[1] != "b" || items[2] != "c" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestRewriteMentionArgs(t *testing.T) {
	// @alice → get notifs --as alice
	args := []string{"fray", "--project", "foo", "@alice", "--last", "5"}
	updated := rewriteMentionArgs(args)
	// Original: fray --project foo @alice --last 5
	// Expected: fray --project foo get notifs --as alice --last 5
	expected := []string{"fray", "--project", "foo", "get", "notifs", "--as", "alice", "--last", "5"}
	if len(updated) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, updated)
	}
	for i, exp := range expected {
		if updated[i] != exp {
			t.Fatalf("at index %d: expected %q, got %q (full: %v)", i, exp, updated[i], updated)
		}
	}

	// No @mention - no rewrite
	args = []string{"fray", "get", "--last", "5"}
	updated = rewriteMentionArgs(args)
	if len(updated) != len(args) {
		t.Fatalf("unexpected rewrite: %v", updated)
	}
}
