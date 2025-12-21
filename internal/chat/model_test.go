package chat

import "testing"

func TestFindSuggestionToken(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		cursor int
		kind   suggestionKind
		start  int
		prefix string
	}{
		{
			name:   "mention at end",
			value:  "hello @bob",
			cursor: len("hello @bob"),
			kind:   suggestionMention,
			start:  len("hello "),
			prefix: "bob",
		},
		{
			name:   "mention in middle",
			value:  "hi @bob there",
			cursor: len("hi @bob"),
			kind:   suggestionMention,
			start:  len("hi "),
			prefix: "bob",
		},
		{
			name:   "reply prefix",
			value:  "reply #msg-12 update",
			cursor: len("reply #msg-12"),
			kind:   suggestionReply,
			start:  len("reply "),
			prefix: "12",
		},
		{
			name:   "ignore embedded at",
			value:  "email foo@bar",
			cursor: len("email foo@bar"),
			kind:   suggestionNone,
			start:  0,
			prefix: "",
		},
		{
			name:   "short reply prefix",
			value:  "#m",
			cursor: len("#m"),
			kind:   suggestionNone,
			start:  0,
			prefix: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kind, start, prefix := findSuggestionToken(tt.value, tt.cursor)
			if kind != tt.kind {
				t.Fatalf("kind: got %v want %v", kind, tt.kind)
			}
			if start != tt.start {
				t.Fatalf("start: got %d want %d", start, tt.start)
			}
			if prefix != tt.prefix {
				t.Fatalf("prefix: got %q want %q", prefix, tt.prefix)
			}
		})
	}
}
