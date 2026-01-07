package core

import (
	"testing"
)

func TestExtractQuestionSections_Basic(t *testing.T) {
	body := `Some intro text.

# Questions for @adam

1. Should we use approach A or B?
2. What's the timeline?

More text after.`

	sections, cleaned := ExtractQuestionSections(body)

	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}

	s := sections[0]
	if s.IsWondering {
		t.Error("expected IsWondering=false")
	}
	if len(s.Targets) != 1 || s.Targets[0] != "adam" {
		t.Errorf("expected targets=[adam], got %v", s.Targets)
	}
	if len(s.Questions) != 2 {
		t.Fatalf("expected 2 questions, got %d", len(s.Questions))
	}
	if s.Questions[0].Text != "Should we use approach A or B?" {
		t.Errorf("unexpected question text: %s", s.Questions[0].Text)
	}

	if !containsSubstring(cleaned, "Some intro text") {
		t.Error("cleaned should contain intro text")
	}
	if !containsSubstring(cleaned, "More text after") {
		t.Error("cleaned should contain trailing text")
	}
	if containsSubstring(cleaned, "# Questions") {
		t.Error("cleaned should not contain Questions header")
	}
}

func TestExtractQuestionSections_MultipleTargets(t *testing.T) {
	body := `# Questions for @adam @opus @designer

1. What do you all think?`

	sections, _ := ExtractQuestionSections(body)

	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if len(sections[0].Targets) != 3 {
		t.Errorf("expected 3 targets, got %v", sections[0].Targets)
	}
}

func TestExtractQuestionSections_Wondering(t *testing.T) {
	body := `# Wondering

1. Is this the right approach?
2. Should we consider alternatives?`

	sections, _ := ExtractQuestionSections(body)

	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if !sections[0].IsWondering {
		t.Error("expected IsWondering=true")
	}
	if len(sections[0].Questions) != 2 {
		t.Errorf("expected 2 questions, got %d", len(sections[0].Questions))
	}
}

func TestExtractQuestionSections_WithOptions(t *testing.T) {
	body := `# Questions for @adam

1. Which database should we use?
   a. PostgreSQL
      - Pro: Mature and reliable
      - Con: Requires more setup
   b. SQLite
      - Pro: Simple and embedded
      - Con: Limited concurrency`

	sections, _ := ExtractQuestionSections(body)

	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}

	q := sections[0].Questions[0]
	if len(q.Options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(q.Options))
	}

	opt1 := q.Options[0]
	if opt1.Label != "PostgreSQL" {
		t.Errorf("expected option label 'PostgreSQL', got '%s'", opt1.Label)
	}
	if len(opt1.Pros) != 1 || opt1.Pros[0] != "Mature and reliable" {
		t.Errorf("unexpected pros: %v", opt1.Pros)
	}
	if len(opt1.Cons) != 1 || opt1.Cons[0] != "Requires more setup" {
		t.Errorf("unexpected cons: %v", opt1.Cons)
	}

	opt2 := q.Options[1]
	if opt2.Label != "SQLite" {
		t.Errorf("expected option label 'SQLite', got '%s'", opt2.Label)
	}
}

func TestExtractQuestionSections_SkipsCodeBlocks(t *testing.T) {
	body := "```markdown\n# Questions for @adam\n\n1. This is not a real question\n```\n\nReal content here."

	sections, cleaned := ExtractQuestionSections(body)

	if len(sections) != 0 {
		t.Errorf("expected 0 sections (inside code block), got %d", len(sections))
	}
	if !containsSubstring(cleaned, "# Questions for @adam") {
		t.Error("code block content should be preserved in cleaned output")
	}
}

func TestExtractQuestionSections_HashHashHeader(t *testing.T) {
	body := `## Questions for @opus

1. Does this work with ## headers?`

	sections, _ := ExtractQuestionSections(body)

	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if len(sections[0].Questions) != 1 {
		t.Errorf("expected 1 question, got %d", len(sections[0].Questions))
	}
}

func TestExtractQuestionSections_IgnoresDeepHeaders(t *testing.T) {
	body := `### Questions for @adam

1. This should not be extracted`

	sections, cleaned := ExtractQuestionSections(body)

	if len(sections) != 0 {
		t.Errorf("expected 0 sections (### ignored), got %d", len(sections))
	}
	if !containsSubstring(cleaned, "### Questions") {
		t.Error("### header should be preserved")
	}
}

func TestExtractQuestionSections_NoTargets(t *testing.T) {
	body := `# Questions

1. General question for the user`

	sections, _ := ExtractQuestionSections(body)

	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if len(sections[0].Targets) != 0 {
		t.Errorf("expected no targets, got %v", sections[0].Targets)
	}
}

func TestExtractQuestionSections_MultipleSections(t *testing.T) {
	body := `# Questions for @adam

1. First question

# Wondering

1. Something I'm thinking about

# Questions for @opus

1. Second question for different agent`

	sections, _ := ExtractQuestionSections(body)

	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(sections))
	}

	if sections[0].Targets[0] != "adam" {
		t.Errorf("first section target wrong: %v", sections[0].Targets)
	}
	if !sections[1].IsWondering {
		t.Error("second section should be Wondering")
	}
	if sections[2].Targets[0] != "opus" {
		t.Errorf("third section target wrong: %v", sections[2].Targets)
	}
}

func TestStripQuestionSections(t *testing.T) {
	body := `Intro text.

# Questions for @adam

1. A question?

Outro text.`

	stripped := StripQuestionSections(body)

	if containsSubstring(stripped, "# Questions") {
		t.Error("stripped should not contain Questions section")
	}
	if !containsSubstring(stripped, "Intro text") {
		t.Error("stripped should contain intro")
	}
	if !containsSubstring(stripped, "Outro text") {
		t.Error("stripped should contain outro")
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
