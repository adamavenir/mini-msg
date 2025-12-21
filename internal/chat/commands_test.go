package chat

import "testing"

func TestParseEditCommand(t *testing.T) {
	id, body, err := parseEditCommand("/edit #msg-123 hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "msg-123" {
		t.Fatalf("id: got %q want %q", id, "msg-123")
	}
	if body != "hello world" {
		t.Fatalf("body: got %q want %q", body, "hello world")
	}

	_, _, err = parseEditCommand("/edit 123")
	if err == nil {
		t.Fatalf("expected error for missing body")
	}
}

func TestParseDeleteCommand(t *testing.T) {
	id, err := parseDeleteCommand("/delete #abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "abc" {
		t.Fatalf("id: got %q want %q", id, "abc")
	}

	_, err = parseDeleteCommand("/delete")
	if err == nil {
		t.Fatalf("expected error for missing id")
	}
}

func TestParsePruneArgs(t *testing.T) {
	keep, all, err := parsePruneArgs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep != 100 || all {
		t.Fatalf("default: got keep=%d all=%v", keep, all)
	}

	keep, all, err = parsePruneArgs([]string{"--keep", "50"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep != 50 || all {
		t.Fatalf("--keep: got keep=%d all=%v", keep, all)
	}

	keep, all, err = parsePruneArgs([]string{"25", "--all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep != 25 || !all {
		t.Fatalf("args: got keep=%d all=%v", keep, all)
	}

	keep, all, err = parsePruneArgs([]string{"--keep=10", "--all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep != 10 || !all {
		t.Fatalf("--keep=: got keep=%d all=%v", keep, all)
	}

	_, _, err = parsePruneArgs([]string{"--keep"})
	if err == nil {
		t.Fatalf("expected error for missing --keep value")
	}
}
