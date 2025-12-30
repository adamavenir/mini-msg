package command

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
)

func resolveQuestionRef(dbConn *sql.DB, ref string) (*types.Question, error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(ref, "#"))
	if trimmed == "" {
		return nil, fmt.Errorf("question reference is required")
	}

	question, err := db.GetQuestion(dbConn, trimmed)
	if err != nil {
		return nil, err
	}
	if question != nil {
		return question, nil
	}

	if !strings.HasPrefix(strings.ToLower(trimmed), "qstn-") {
		question, err = db.GetQuestion(dbConn, "qstn-"+trimmed)
		if err != nil {
			return nil, err
		}
		if question != nil {
			return question, nil
		}
	}

	question, err = db.GetQuestionByPrefix(dbConn, trimmed)
	if err != nil {
		return nil, err
	}
	if question == nil {
		return nil, fmt.Errorf("question not found: %s", ref)
	}
	return question, nil
}

func matchQuestionForAnswer(dbConn *sql.DB, ref string) (*types.Question, []types.Question, error) {
	question, err := resolveQuestionRef(dbConn, ref)
	if err == nil {
		return question, nil, nil
	}
	if !strings.Contains(err.Error(), "not found") {
		return nil, nil, err
	}

	matches, err := db.GetQuestionsByRe(dbConn, ref)
	if err != nil {
		return nil, nil, err
	}
	if len(matches) == 1 {
		return &matches[0], nil, nil
	}
	if len(matches) > 1 {
		return nil, matches, nil
	}
	return nil, nil, fmt.Errorf("question not found: %s", ref)
}
