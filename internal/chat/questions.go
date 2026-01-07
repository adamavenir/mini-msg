package chat

import (
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

const questionStaleSeconds = 7 * 24 * 3600

// Legacy type alias for backwards compatibility
type pseudoThreadKind = messageCollectionView

const (
	pseudoThreadOpen   = messageCollectionOpenQuestions
	pseudoThreadClosed = messageCollectionClosedQuestions
	pseudoThreadWonder = messageCollectionWondering
	pseudoThreadStale  = messageCollectionStaleQuestions
)

// questionSourceMessages returns the source messages for questions in the current pseudo-thread.
// This allows questions to be displayed using the standard message rendering.
// Messages with multiple questions are deduplicated - each message appears once.
func (m *Model) questionSourceMessages() []types.Message {
	if len(m.pseudoQuestions) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	messages := make([]types.Message, 0, len(m.pseudoQuestions))
	for _, q := range m.pseudoQuestions {
		if q.AskedIn != nil {
			if _, exists := seen[*q.AskedIn]; exists {
				continue
			}
			// Fetch the actual source message
			msg, err := db.GetMessage(m.db, *q.AskedIn)
			if err == nil && msg != nil {
				seen[*q.AskedIn] = struct{}{}
				messages = append(messages, *msg)
			}
		} else {
			// For questions without a source message (e.g., wondering), create a synthetic one
			if _, exists := seen[q.GUID]; exists {
				continue
			}
			seen[q.GUID] = struct{}{}
			messages = append(messages, types.Message{
				ID:        q.GUID,
				TS:        q.CreatedAt,
				FromAgent: q.FromAgent,
				Body:      q.Re,
				Type:      types.MessageTypeAgent,
				Home:      "room",
			})
		}
	}
	return messages
}

func (m *Model) refreshQuestionCounts() {
	if m.questionCounts == nil {
		m.questionCounts = make(map[pseudoThreadKind]int)
	}
	// Question counts are global (not thread-scoped) so sidebar doesn't jump around
	openQuestions, err := db.GetQuestions(m.db, &types.QuestionQueryOptions{
		Statuses: []types.QuestionStatus{types.QuestionStatusOpen},
	})
	if err == nil {
		m.questionCounts[pseudoThreadOpen] = len(openQuestions)
		stale := 0
		cutoff := time.Now().Unix() - questionStaleSeconds
		for _, question := range openQuestions {
			if question.CreatedAt > 0 && question.CreatedAt < cutoff {
				stale++
			}
		}
		m.questionCounts[pseudoThreadStale] = stale
	}

	answeredQuestions, err := db.GetQuestions(m.db, &types.QuestionQueryOptions{
		Statuses: []types.QuestionStatus{types.QuestionStatusAnswered},
	})
	if err == nil {
		m.questionCounts[pseudoThreadClosed] = len(answeredQuestions)
	}

	unaskedQuestions, err := db.GetQuestions(m.db, &types.QuestionQueryOptions{
		Statuses: []types.QuestionStatus{types.QuestionStatusUnasked},
	})
	if err == nil {
		m.questionCounts[pseudoThreadWonder] = len(unaskedQuestions)
	}
}

func (m *Model) refreshPseudoQuestions() {
	if m.currentPseudo == "" {
		m.pseudoQuestions = nil
		return
	}
	// Query globally to match sidebar counts
	options := types.QuestionQueryOptions{}

	switch m.currentPseudo {
	case pseudoThreadOpen:
		options.Statuses = []types.QuestionStatus{types.QuestionStatusOpen}
	case pseudoThreadClosed:
		options.Statuses = []types.QuestionStatus{types.QuestionStatusAnswered}
	case pseudoThreadWonder:
		options.Statuses = []types.QuestionStatus{types.QuestionStatusUnasked}
	case pseudoThreadStale:
		options.Statuses = []types.QuestionStatus{types.QuestionStatusOpen}
	}

	questions, err := db.GetQuestions(m.db, &options)
	if err != nil {
		m.status = err.Error()
		return
	}
	if m.currentPseudo == pseudoThreadStale {
		cutoff := time.Now().Unix() - questionStaleSeconds
		filtered := make([]types.Question, 0, len(questions))
		for _, question := range questions {
			if question.CreatedAt > 0 && question.CreatedAt < cutoff {
				filtered = append(filtered, question)
			}
		}
		m.pseudoQuestions = filtered
		return
	}
	m.pseudoQuestions = questions
}
