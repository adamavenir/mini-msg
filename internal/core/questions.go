package core

import (
	"regexp"
	"strings"
)

// QuestionOption represents a proposed answer option with pros/cons.
type QuestionOption struct {
	Label string   `json:"label"`
	Pros  []string `json:"pros,omitempty"`
	Cons  []string `json:"cons,omitempty"`
}

// ExtractedQuestion represents a question parsed from markdown.
type ExtractedQuestion struct {
	Text    string           `json:"text"`
	Options []QuestionOption `json:"options,omitempty"`
}

// QuestionSection represents a parsed # Questions or # Wondering section.
type QuestionSection struct {
	IsWondering bool                `json:"is_wondering"`
	Targets     []string            `json:"targets,omitempty"`
	Questions   []ExtractedQuestion `json:"questions"`
	StartLine   int                 `json:"start_line"`
	EndLine     int                 `json:"end_line"`
}

var (
	// Match # or ## Questions/Wondering headers
	questionHeaderRe = regexp.MustCompile(`(?i)^(#{1,2})\s+(questions|wondering)(?:\s+for\s+(.+))?$`)
	// Match numbered items: 1. 2. 3. etc
	numberedItemRe = regexp.MustCompile(`^(\d+)\.\s+(.+)$`)
	// Match lettered options: a. b. c. etc
	letteredOptionRe = regexp.MustCompile(`^([a-z])\.\s+(.+)$`)
	// Match pro/con bullets
	proConRe = regexp.MustCompile(`(?i)^-\s*(pro|con)s?:\s*(.+)$`)
	// Match code fence start/end
	codeFenceRe = regexp.MustCompile("^```")
)

// ExtractQuestionSections parses markdown and extracts question/wondering sections.
// Returns sections found and the body with those sections removed.
func ExtractQuestionSections(body string) ([]QuestionSection, string) {
	lines := strings.Split(body, "\n")
	var sections []QuestionSection
	var cleanedLines []string

	inCodeFence := false
	inSection := false
	var currentSection *QuestionSection
	var currentQuestion *ExtractedQuestion
	var currentOption *QuestionOption

	for lineNum, line := range lines {
		// Track code fences
		if codeFenceRe.MatchString(strings.TrimSpace(line)) {
			inCodeFence = !inCodeFence
			if !inSection {
				cleanedLines = append(cleanedLines, line)
			}
			continue
		}

		// Skip parsing inside code fences
		if inCodeFence {
			if !inSection {
				cleanedLines = append(cleanedLines, line)
			}
			continue
		}

		trimmed := strings.TrimSpace(line)

		// Check for section header
		if match := questionHeaderRe.FindStringSubmatch(trimmed); match != nil {
			// Save previous section if any
			if currentSection != nil {
				finishQuestion(currentSection, currentQuestion, currentOption)
				currentSection.EndLine = lineNum - 1
				sections = append(sections, *currentSection)
			}

			headerType := strings.ToLower(match[2])
			currentSection = &QuestionSection{
				IsWondering: headerType == "wondering",
				StartLine:   lineNum,
			}

			// Parse targets from "for @x @y @z"
			if match[3] != "" {
				currentSection.Targets = extractMentionsFromText(match[3])
			}

			currentQuestion = nil
			currentOption = nil
			inSection = true
			continue
		}

		// Check for next top-level header (ends current section)
		if inSection && strings.HasPrefix(trimmed, "#") {
			finishQuestion(currentSection, currentQuestion, currentOption)
			currentSection.EndLine = lineNum - 1
			sections = append(sections, *currentSection)
			currentSection = nil
			currentQuestion = nil
			currentOption = nil
			inSection = false
			cleanedLines = append(cleanedLines, line)
			continue
		}

		if !inSection {
			cleanedLines = append(cleanedLines, line)
			continue
		}

		// Inside a section - parse content
		indent := countLeadingSpaces(line)

		// Numbered item (question)
		if match := numberedItemRe.FindStringSubmatch(trimmed); match != nil {
			finishQuestion(currentSection, currentQuestion, currentOption)
			currentQuestion = &ExtractedQuestion{Text: match[2]}
			currentOption = nil
			continue
		}

		// Lettered option (requires being in a question, indent >= 2)
		if currentQuestion != nil && indent >= 2 {
			if match := letteredOptionRe.FindStringSubmatch(trimmed); match != nil {
				finishOption(currentQuestion, currentOption)
				currentOption = &QuestionOption{Label: match[2]}
				continue
			}
		}

		// Pro/con bullet (requires being in an option, indent >= 4)
		if currentOption != nil && indent >= 4 {
			if match := proConRe.FindStringSubmatch(trimmed); match != nil {
				proConType := strings.ToLower(match[1])
				text := match[2]
				if proConType == "pro" {
					currentOption.Pros = append(currentOption.Pros, text)
				} else {
					currentOption.Cons = append(currentOption.Cons, text)
				}
				continue
			}
		}

		// Empty line - continue section but don't append
		if trimmed == "" {
			continue
		}

		// Non-empty, non-matching line at indent 0 ends the section
		// (regular text that doesn't belong to questions)
		if indent == 0 && currentQuestion != nil {
			finishQuestion(currentSection, currentQuestion, currentOption)
			currentSection.EndLine = lineNum - 1
			sections = append(sections, *currentSection)
			currentSection = nil
			currentQuestion = nil
			currentOption = nil
			inSection = false
			cleanedLines = append(cleanedLines, line)
			continue
		}

		// Continuation line - append to current context
		if currentOption != nil && indent >= 4 {
			// Continuation of option label
			currentOption.Label += " " + trimmed
		} else if currentQuestion != nil && indent >= 2 && currentOption == nil {
			// Could be option continuation or question continuation
			currentQuestion.Text += " " + trimmed
		} else if currentQuestion != nil {
			// Question text continuation
			currentQuestion.Text += " " + trimmed
		}
	}

	// Finish final section
	if currentSection != nil {
		finishQuestion(currentSection, currentQuestion, currentOption)
		currentSection.EndLine = len(lines) - 1
		sections = append(sections, *currentSection)
	}

	return sections, strings.Join(cleanedLines, "\n")
}

// StripQuestionSections removes question/wondering sections from body for display.
func StripQuestionSections(body string) string {
	_, cleaned := ExtractQuestionSections(body)
	// Trim trailing whitespace that might be left over
	return strings.TrimRight(cleaned, "\n") + "\n"
}

func finishOption(q *ExtractedQuestion, opt *QuestionOption) {
	if opt != nil && q != nil {
		q.Options = append(q.Options, *opt)
	}
}

func finishQuestion(s *QuestionSection, q *ExtractedQuestion, opt *QuestionOption) {
	if q != nil && s != nil {
		finishOption(q, opt)
		s.Questions = append(s.Questions, *q)
	}
}

func countLeadingSpaces(line string) int {
	count := 0
	for _, r := range line {
		if r == ' ' {
			count++
		} else if r == '\t' {
			count += 4
		} else {
			break
		}
	}
	return count
}

func extractMentionsFromText(text string) []string {
	matches := mentionRe.FindAllStringSubmatch(text, -1)
	var mentions []string
	for _, match := range matches {
		if len(match) >= 2 {
			mentions = append(mentions, match[1])
		}
	}
	return mentions
}
