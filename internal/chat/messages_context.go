package chat

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) replyContext(replyTo string, prefixLength int) string {
	row := m.db.QueryRow(`
		SELECT from_agent, origin, body FROM fray_messages WHERE guid = ?
	`, replyTo)
	var fromAgent string
	var origin sql.NullString
	var body string
	if err := row.Scan(&fromAgent, &origin, &body); err != nil {
		prefix := core.GetGUIDPrefix(replyTo, prefixLength)
		return lipgloss.NewStyle().Foreground(metaColor).Render(fmt.Sprintf("↪ Reply to #%s", prefix))
	}
	display := m.displayAgentLabel(types.Message{FromAgent: fromAgent, Origin: origin.String})
	preview := truncatePreview(body, 50)
	return lipgloss.NewStyle().Foreground(metaColor).Render(fmt.Sprintf("↪ Reply to @%s: %s", display, preview))
}

func (m *Model) formatQuestionStatus(msgID string) string {
	questions, err := db.GetQuestions(m.db, &types.QuestionQueryOptions{
		AskedIn: &msgID,
	})
	if err != nil || len(questions) == 0 {
		return ""
	}

	var answered, unanswered []string
	for i, q := range questions {
		label := fmt.Sprintf("Q%d", i+1)
		if q.Status == types.QuestionStatusAnswered {
			answered = append(answered, label)
		} else {
			unanswered = append(unanswered, label)
		}
	}

	var parts []string
	if len(answered) > 0 {
		answeredStyle := lipgloss.NewStyle().Bold(true)
		parts = append(parts, answeredStyle.Render("Answered")+": "+strings.Join(answered, ", "))
	}
	if len(unanswered) > 0 {
		unansweredStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")) // yellow
		parts = append(parts, unansweredStyle.Render("Unanswered")+": "+strings.Join(unanswered, ", "))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "  ")
}
