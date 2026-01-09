package chat

import "testing"

func TestParseEditCommand(t *testing.T) {
	id, body, reason, err := parseEditCommand("/edit #msg-123 hello world -m fixed typo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "msg-123" {
		t.Fatalf("id: got %q want %q", id, "msg-123")
	}
	if body != "hello world" {
		t.Fatalf("body: got %q want %q", body, "hello world")
	}
	if reason != "fixed typo" {
		t.Fatalf("reason: got %q want %q", reason, "fixed typo")
	}

	_, _, _, err = parseEditCommand("/edit 123")
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
	keep, all, target, withReact, err := parsePruneArgs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep != 20 || all || target != "" || withReact != "" {
		t.Fatalf("default: got keep=%d all=%v target=%q withReact=%q", keep, all, target, withReact)
	}

	keep, all, target, withReact, err = parsePruneArgs([]string{"--keep", "50"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep != 50 || all || target != "" || withReact != "" {
		t.Fatalf("--keep: got keep=%d all=%v target=%q withReact=%q", keep, all, target, withReact)
	}

	keep, all, target, withReact, err = parsePruneArgs([]string{"25", "--all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep != 25 || !all || target != "" || withReact != "" {
		t.Fatalf("args: got keep=%d all=%v target=%q withReact=%q", keep, all, target, withReact)
	}

	keep, all, target, withReact, err = parsePruneArgs([]string{"--keep=10", "--all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep != 10 || !all || target != "" || withReact != "" {
		t.Fatalf("--keep=: got keep=%d all=%v target=%q withReact=%q", keep, all, target, withReact)
	}

	_, _, _, _, err = parsePruneArgs([]string{"--keep"})
	if err == nil {
		t.Fatalf("expected error for missing --keep value")
	}

	// Test target argument
	keep, all, target, withReact, err = parsePruneArgs([]string{"my-thread", "--keep", "30"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep != 30 || all || target != "my-thread" || withReact != "" {
		t.Fatalf("target: got keep=%d all=%v target=%q withReact=%q", keep, all, target, withReact)
	}

	// Test --with-react flag
	keep, all, target, withReact, err = parsePruneArgs([]string{"--with-react", "📁"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keep != 20 || all || target != "" || withReact != "📁" {
		t.Fatalf("--with-react: got keep=%d all=%v target=%q withReact=%q", keep, all, target, withReact)
	}
}

func TestRewriteClickThenCommand(t *testing.T) {
	tests := []struct {
		input    string
		want     string
		wantOK   bool
	}{
		// Valid click-then-command patterns
		{"#msg-abc123 /mv meta", "/mv #msg-abc123 meta", true},
		{"#msg-abc /delete", "/delete #msg-abc", true},
		{"#abc /mv design-thread", "/mv #abc design-thread", true},
		{"#thrd-xyz /archive", "/archive #thrd-xyz", true},

		// Not click-then-command (regular slash commands)
		{"/mv meta", "", false},
		{"/delete #msg-abc", "", false},

		// Not click-then-command (regular messages)
		{"hello world", "", false},
		{"#msg-abc hello", "", false},

		// Not click-then-command (command not immediately after ID)
		{"#msg-abc this is about /edit mode", "", false},
		{"#msg-abc some text /mv thread", "", false},
	}

	for _, tt := range tests {
		got, ok := rewriteClickThenCommand(tt.input)
		if ok != tt.wantOK {
			t.Errorf("rewriteClickThenCommand(%q): ok = %v, want %v", tt.input, ok, tt.wantOK)
			continue
		}
		if ok && got != tt.want {
			t.Errorf("rewriteClickThenCommand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseThreadArgs(t *testing.T) {
	tests := []struct {
		input      string
		wantName   string
		wantAnchor string
		wantErr    bool
	}{
		{"/thread my-thread", "my-thread", "", false},
		{"/t my-thread", "my-thread", "", false},
		{"/thread my-thread \"anchor text\"", "my-thread", "anchor text", false},
		{"/t design \"This is the anchor\"", "design", "This is the anchor", false},
		{"/subthread sub-name", "sub-name", "", false},
		{"/st sub-name \"sub anchor\"", "sub-name", "sub anchor", false},
		{"/thread", "", "", true},           // no name
		{"/t", "", "", true},                 // no name
		{"/thread \"just anchor\"", "", "", true}, // no name before quote
	}

	for _, tt := range tests {
		name, anchor, err := parseThreadArgs(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseThreadArgs(%q): expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseThreadArgs(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if name != tt.wantName {
			t.Errorf("parseThreadArgs(%q): name = %q, want %q", tt.input, name, tt.wantName)
		}
		if anchor != tt.wantAnchor {
			t.Errorf("parseThreadArgs(%q): anchor = %q, want %q", tt.input, anchor, tt.wantAnchor)
		}
	}
}
