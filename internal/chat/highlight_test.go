package chat

import "testing"

func TestParseFence(t *testing.T) {
	fence, lang, ok := parseFence("```go")
	if !ok {
		t.Fatalf("expected fence")
	}
	if fence != "```" {
		t.Fatalf("fence: got %q", fence)
	}
	if lang != "go" {
		t.Fatalf("lang: got %q", lang)
	}

	fence, lang, ok = parseFence("~~~  mlld other")
	if !ok {
		t.Fatalf("expected fence")
	}
	if fence != "~~~" {
		t.Fatalf("fence: got %q", fence)
	}
	if lang != "mlld" {
		t.Fatalf("lang: got %q", lang)
	}
}

func TestHighlightCodeBlocksNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	input := "start\n```go\nfmt.Println(\"hi\")\n```\nend"
	output := highlightCodeBlocks(input)
	if output != input {
		t.Fatalf("expected output to match input when NO_COLOR set")
	}
}

func TestHighlightCodeBlocksUnclosedFence(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	input := "start\n```go\ncode\nend"
	output := highlightCodeBlocks(input)
	if output != input {
		t.Fatalf("expected output to match input when fence is unclosed")
	}
}
