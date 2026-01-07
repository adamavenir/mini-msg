package command

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type answerPhase int

const (
	phaseAnswering answerPhase = iota
	phaseReviewPrompt
	phaseReviewingSkipped
	phaseDone
)

type answerModel struct {
	database *sql.DB
	dbPath   string
	identity string

	sets         []questionSet
	currentSet   int
	currentQ     int
	phase        answerPhase
	answered     []qaPair
	skipped      []types.Question
	reviewIndex  int
	reviewChoice string

	input    textarea.Model
	width    int
	height   int
	quitting bool
	err      error
}

func newAnswerModel(database *sql.DB, dbPath, identity string, questions []types.Question) answerModel {
	sets := groupQuestionSets(questions)

	input := textarea.New()
	input.CharLimit = 0
	input.ShowLineNumbers = false
	input.MaxHeight = 5
	input.Placeholder = "Type your answer, or press a/b/c to select an option..."
	input.SetPromptFunc(2, func(lineIdx int) string {
		if lineIdx == 0 {
			return "› "
		}
		return "  "
	})
	input.Focus()

	return answerModel{
		database: database,
		dbPath:   dbPath,
		identity: identity,
		sets:     sets,
		input:    input,
		phase:    phaseAnswering,
	}
}

func (m answerModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m answerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(msg.Width - 4)
		return m, nil

	case tea.KeyMsg:
		switch m.phase {
		case phaseAnswering, phaseReviewingSkipped:
			return m.handleAnswerInput(msg)
		case phaseReviewPrompt:
			return m.handleReviewPrompt(msg)
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m answerModel) handleAnswerInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	q := m.currentQuestion()
	if q == nil {
		m.phase = phaseDone
		return m, tea.Quit
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit

	case tea.KeyEnter:
		value := strings.TrimSpace(m.input.Value())

		// Check for skip/quit commands
		valueLower := strings.ToLower(value)
		if valueLower == "s" || valueLower == "skip" {
			return m.skipCurrent()
		}
		if valueLower == "q" || valueLower == "quit" {
			m.quitting = true
			return m, tea.Quit
		}

		// Check for option selection (a, b, c, etc.)
		if len(valueLower) == 1 && len(q.Options) > 0 {
			idx := int(valueLower[0] - 'a')
			if idx >= 0 && idx < len(q.Options) {
				return m.answerCurrent(q.Options[idx].Label)
			}
		}

		// Empty input = skip
		if value == "" {
			return m.skipCurrent()
		}

		// Custom answer
		return m.answerCurrent(value)

	case tea.KeyEsc:
		return m.skipCurrent()
	}

	// Handle Ctrl+J for newline (like chat)
	if msg.Type == tea.KeyCtrlJ {
		m.input.InsertString("\n")
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m answerModel) handleReviewPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit

	case tea.KeyEnter:
		choice := strings.ToLower(strings.TrimSpace(m.reviewChoice))
		if choice == "y" || choice == "yes" {
			m.phase = phaseReviewingSkipped
			m.reviewIndex = 0
			m.input.Reset()
			return m, nil
		}
		m.phase = phaseDone
		return m, tea.Quit

	case tea.KeyRunes:
		m.reviewChoice += string(msg.Runes)
		return m, nil

	case tea.KeyBackspace:
		if len(m.reviewChoice) > 0 {
			m.reviewChoice = m.reviewChoice[:len(m.reviewChoice)-1]
		}
		return m, nil
	}

	return m, nil
}

func (m answerModel) currentQuestion() *types.Question {
	switch m.phase {
	case phaseAnswering:
		if m.currentSet >= len(m.sets) {
			return nil
		}
		set := m.sets[m.currentSet]
		if m.currentQ >= len(set.questions) {
			return nil
		}
		return &set.questions[m.currentQ]

	case phaseReviewingSkipped:
		if m.reviewIndex >= len(m.skipped) {
			return nil
		}
		return &m.skipped[m.reviewIndex]
	}
	return nil
}

func (m answerModel) answerCurrent(answer string) (tea.Model, tea.Cmd) {
	q := m.currentQuestion()
	if q == nil {
		return m, nil
	}

	m.answered = append(m.answered, qaPair{question: *q, answer: answer})
	m.input.Reset()
	return m.advance()
}

func (m answerModel) skipCurrent() (tea.Model, tea.Cmd) {
	q := m.currentQuestion()
	if q == nil {
		return m, nil
	}

	if m.phase == phaseAnswering {
		m.skipped = append(m.skipped, *q)
	}
	m.input.Reset()
	return m.advance()
}

func (m answerModel) advance() (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseAnswering:
		m.currentQ++
		if m.currentQ >= len(m.sets[m.currentSet].questions) {
			m.currentQ = 0
			m.currentSet++
		}
		if m.currentSet >= len(m.sets) {
			if len(m.skipped) > 0 {
				m.phase = phaseReviewPrompt
				m.reviewChoice = ""
			} else {
				m.phase = phaseDone
				return m, tea.Quit
			}
		}

	case phaseReviewingSkipped:
		m.reviewIndex++
		if m.reviewIndex >= len(m.skipped) {
			m.phase = phaseDone
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m answerModel) View() string {
	if m.quitting {
		return ""
	}

	switch m.phase {
	case phaseAnswering, phaseReviewingSkipped:
		return m.renderQuestion()
	case phaseReviewPrompt:
		return m.renderReviewPrompt()
	case phaseDone:
		return ""
	}
	return ""
}

func (m answerModel) renderQuestion() string {
	q := m.currentQuestion()
	if q == nil {
		return ""
	}

	var b strings.Builder

	// Progress header
	var progress string
	if m.phase == phaseAnswering {
		totalSets := len(m.sets)
		totalInSet := len(m.sets[m.currentSet].questions)
		if totalSets > 1 {
			progress = fmt.Sprintf("Set %d/%d, Question %d/%d", m.currentSet+1, totalSets, m.currentQ+1, totalInSet)
		} else {
			progress = fmt.Sprintf("Question %d/%d", m.currentQ+1, totalInSet)
		}
	} else {
		progress = fmt.Sprintf("Review: Question %d/%d", m.reviewIndex+1, len(m.skipped))
	}
	b.WriteString(answerHeaderStyle.Render(progress))
	b.WriteString("\n\n")

	// Question metadata
	fromTo := fmt.Sprintf("From @%s", q.FromAgent)
	if q.ToAgent != nil {
		fromTo += fmt.Sprintf(" → @%s", *q.ToAgent)
	}
	b.WriteString(answerMetaStyle.Render(fromTo))
	b.WriteString("\n")

	if q.ThreadGUID != nil {
		thread, _ := db.GetThread(m.database, *q.ThreadGUID)
		if thread != nil {
			b.WriteString(answerMetaStyle.Render(fmt.Sprintf("Thread: %s", thread.Name)))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	// Question text
	b.WriteString(answerQuestionStyle.Render(q.Re))
	b.WriteString("\n\n")

	// Options with pros/cons
	if len(q.Options) > 0 {
		for i, opt := range q.Options {
			letter := string(rune('a' + i))
			b.WriteString(fmt.Sprintf("  %s. %s\n", answerOptionStyle.Render(letter), opt.Label))

			for _, pro := range opt.Pros {
				b.WriteString(fmt.Sprintf("     %s %s\n", answerProStyle.Render("+ Pro:"), pro))
			}
			for _, con := range opt.Cons {
				b.WriteString(fmt.Sprintf("     %s %s\n", answerConStyle.Render("- Con:"), con))
			}
			if len(opt.Pros) > 0 || len(opt.Cons) > 0 {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	// Input area
	helpText := "[s]kip  [q]uit  Ctrl+J for newline"
	if len(q.Options) > 0 {
		helpText = "[a-" + string(rune('a'+len(q.Options)-1)) + "] select  " + helpText
	}
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render(helpText))
	b.WriteString("\n\n")
	b.WriteString(m.input.View())

	return b.String()
}

func (m answerModel) renderReviewPrompt() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(answerPromptStyle.Render(fmt.Sprintf("You skipped %d question(s). Review them now? [y/n]: ", len(m.skipped))))
	b.WriteString(m.reviewChoice)
	return b.String()
}

func runAnswerSessionTUI(database *sql.DB, dbPath string, identity string, questions []types.Question) error {
	model := newAnswerModel(database, dbPath, identity, questions)
	program := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return err
	}

	m := finalModel.(answerModel)
	if m.err != nil {
		return m.err
	}

	// Post all answers
	if len(m.answered) > 0 {
		if err := postAnswerSummary(database, dbPath, identity, m.answered); err != nil {
			return err
		}
	}

	// Print summary
	stillSkipped := len(m.skipped)
	if m.phase == phaseReviewingSkipped {
		stillSkipped = len(m.skipped) - m.reviewIndex
	}
	if m.phase == phaseDone && m.reviewIndex >= len(m.skipped) {
		stillSkipped = 0
	}
	printSummary(len(m.answered), stillSkipped)

	return nil
}
