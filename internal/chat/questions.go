package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

const questionStaleSeconds = 7 * 24 * 3600

type pseudoThreadKind string

const (
	pseudoThreadOpen   pseudoThreadKind = "open-qs"
	pseudoThreadClosed pseudoThreadKind = "closed-qs"
	pseudoThreadWonder pseudoThreadKind = "wondering"
	pseudoThreadStale  pseudoThreadKind = "stale-qs"
)

func (m *Model) renderQuestions() string {
	if len(m.pseudoQuestions) == 0 {
		return "No questions"
	}
	lines := make([]string, 0, len(m.pseudoQuestions))
	for _, question := range m.pseudoQuestions {
		threadLabel := "room"
		if question.ThreadGUID != nil {
			thread, _ := db.GetThread(m.db, *question.ThreadGUID)
			if thread != nil {
				if path, err := threadPath(m.db, thread); err == nil && path != "" {
					threadLabel = path
				} else {
					threadLabel = thread.GUID
				}
			} else {
				threadLabel = *question.ThreadGUID
			}
		}
		toAgent := "--"
		if question.ToAgent != nil {
			toAgent = "@" + *question.ToAgent
		}
		lines = append(lines, fmt.Sprintf("[%s] %s @%s → %s (%s)", question.GUID, question.Status, question.FromAgent, toAgent, threadLabel))
		lines = append(lines, fmt.Sprintf("  %s", question.Re))
	}
	return strings.Join(lines, "\n\n")
}

func (m *Model) refreshQuestionCounts() {
	if m.questionCounts == nil {
		m.questionCounts = make(map[pseudoThreadKind]int)
	}
	threadGUID, roomOnly := m.questionScope()

	openQuestions, err := db.GetQuestions(m.db, &types.QuestionQueryOptions{
		Statuses:   []types.QuestionStatus{types.QuestionStatusOpen},
		ThreadGUID: threadGUID,
		RoomOnly:   roomOnly,
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
		Statuses:   []types.QuestionStatus{types.QuestionStatusAnswered},
		ThreadGUID: threadGUID,
		RoomOnly:   roomOnly,
	})
	if err == nil {
		m.questionCounts[pseudoThreadClosed] = len(answeredQuestions)
	}

	unaskedQuestions, err := db.GetQuestions(m.db, &types.QuestionQueryOptions{
		Statuses:   []types.QuestionStatus{types.QuestionStatusUnasked},
		ThreadGUID: threadGUID,
		RoomOnly:   roomOnly,
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
	threadGUID, roomOnly := m.questionScope()
	options := types.QuestionQueryOptions{
		ThreadGUID: threadGUID,
		RoomOnly:   roomOnly,
	}

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

func (m *Model) questionScope() (*string, bool) {
	if m.currentThread != nil {
		return &m.currentThread.GUID, false
	}
	return nil, true
}
