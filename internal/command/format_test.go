package command

import (
	"strings"
	"testing"

	"github.com/adamavenir/mini-msg/internal/types"
)

func TestFormatMessageColors(t *testing.T) {
	if noColor {
		t.Skip("NO_COLOR set; skipping ANSI color test")
	}

	msg := types.Message{
		ID:        "msg-abc123",
		FromAgent: "alice.1",
		Body:      "hello @bob",
		Type:      types.MessageTypeAgent,
	}
	bases := map[string]struct{}{"alice": {}, "bob": {}}
	output := FormatMessage(msg, "demo", bases)

	if !strings.Contains(output, "\x1b[") {
		t.Fatalf("expected ANSI output, got %q", output)
	}
	if !strings.Contains(output, "@alice.1") || !strings.Contains(output, msg.ID) {
		t.Fatalf("missing agent or id in output: %q", output)
	}
}

func TestFormatMessageTruncation(t *testing.T) {
	lines := make([]string, 0, maxDisplayLines+2)
	for i := 0; i < maxDisplayLines+2; i++ {
		lines = append(lines, "line")
	}
	body := strings.Join(lines, "\n")

	msg := types.Message{
		ID:        "msg-xyz789",
		FromAgent: "alice",
		Body:      body,
		Type:      types.MessageTypeAgent,
	}
	output := FormatMessage(msg, "demo", nil)
	if !strings.Contains(output, "mm view "+msg.ID) {
		t.Fatalf("expected truncation hint, got %q", output)
	}
}
