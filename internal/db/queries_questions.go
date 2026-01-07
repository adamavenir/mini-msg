package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/types"
)

// QuestionUpdates represents partial question updates.
type QuestionUpdates struct {
	Status     types.OptionalString
	ToAgent    types.OptionalString
	ThreadGUID types.OptionalString
	AskedIn    types.OptionalString
	AnsweredIn types.OptionalString
}

// CreateQuestion inserts a new question.
func CreateQuestion(db *sql.DB, question types.Question) (types.Question, error) {
	guid := question.GUID
	if guid == "" {
		var err error
		guid, err = generateUniqueGUIDForTable(db, "fray_questions", "qstn")
		if err != nil {
			return types.Question{}, err
		}
	}

	status := question.Status
	if status == "" {
		status = types.QuestionStatusUnasked
	}
	createdAt := question.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}

	optionsJSON := "[]"
	if len(question.Options) > 0 {
		optBytes, err := json.Marshal(question.Options)
		if err != nil {
			return types.Question{}, err
		}
		optionsJSON = string(optBytes)
	}

	_, err := db.Exec(`
		INSERT INTO fray_questions (guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, options, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, guid, question.Re, question.FromAgent, question.ToAgent, string(status), question.ThreadGUID, question.AskedIn, question.AnsweredIn, optionsJSON, createdAt)
	if err != nil {
		return types.Question{}, err
	}

	question.GUID = guid
	question.Status = status
	question.CreatedAt = createdAt
	return question, nil
}

// UpdateQuestion updates question fields.
func UpdateQuestion(db *sql.DB, guid string, updates QuestionUpdates) (*types.Question, error) {
	var fields []string
	var args []any

	if updates.Status.Set {
		fields = append(fields, "status = ?")
		args = append(args, nullableValue(updates.Status.Value))
	}
	if updates.ToAgent.Set {
		fields = append(fields, "to_agent = ?")
		args = append(args, nullableValue(updates.ToAgent.Value))
	}
	if updates.ThreadGUID.Set {
		fields = append(fields, "thread_guid = ?")
		args = append(args, nullableValue(updates.ThreadGUID.Value))
	}
	if updates.AskedIn.Set {
		fields = append(fields, "asked_in = ?")
		args = append(args, nullableValue(updates.AskedIn.Value))
	}
	if updates.AnsweredIn.Set {
		fields = append(fields, "answered_in = ?")
		args = append(args, nullableValue(updates.AnsweredIn.Value))
	}

	if len(fields) == 0 {
		return GetQuestion(db, guid)
	}

	args = append(args, guid)
	query := fmt.Sprintf("UPDATE fray_questions SET %s WHERE guid = ?", strings.Join(fields, ", "))
	if _, err := db.Exec(query, args...); err != nil {
		return nil, err
	}
	return GetQuestion(db, guid)
}

// GetQuestion returns a question by GUID.
func GetQuestion(db *sql.DB, guid string) (*types.Question, error) {
	row := db.QueryRow(`
		SELECT guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, options, created_at
		FROM fray_questions WHERE guid = ?
	`, guid)

	question, err := scanQuestion(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &question, nil
}

// GetQuestionByPrefix returns the first question matching a GUID prefix.
func GetQuestionByPrefix(db *sql.DB, prefix string) (*types.Question, error) {
	rows, err := db.Query(`
		SELECT guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, options, created_at
		FROM fray_questions
		WHERE guid = ? OR guid LIKE ?
		ORDER BY created_at DESC
	`, prefix, fmt.Sprintf("%s%%", prefix))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}
	question, err := scanQuestion(rows)
	if err != nil {
		return nil, err
	}
	return &question, nil
}

// GetQuestionsByRe returns questions matching the provided text.
func GetQuestionsByRe(db *sql.DB, re string) ([]types.Question, error) {
	rows, err := db.Query(`
		SELECT guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, options, created_at
		FROM fray_questions
		WHERE lower(re) = lower(?)
		ORDER BY created_at ASC
	`, strings.TrimSpace(re))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanQuestions(rows)
}

// GetQuestions returns questions filtered by options.
func GetQuestions(db *sql.DB, opts *types.QuestionQueryOptions) ([]types.Question, error) {
	query := `
		SELECT guid, re, from_agent, to_agent, status, thread_guid, asked_in, answered_in, options, created_at
		FROM fray_questions
	`
	var conditions []string
	var args []any

	if opts != nil {
		if len(opts.Statuses) > 0 {
			placeholders := make([]string, 0, len(opts.Statuses))
			for _, status := range opts.Statuses {
				placeholders = append(placeholders, "?")
				args = append(args, string(status))
			}
			conditions = append(conditions, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ", ")))
		}
		if opts.ThreadGUID != nil {
			conditions = append(conditions, "thread_guid = ?")
			args = append(args, *opts.ThreadGUID)
		} else if opts.RoomOnly {
			conditions = append(conditions, "thread_guid IS NULL")
		}
		if opts.ToAgent != nil && *opts.ToAgent != "" {
			conditions = append(conditions, "to_agent = ?")
			args = append(args, *opts.ToAgent)
		}
		if opts.AskedIn != nil {
			conditions = append(conditions, "asked_in = ?")
			args = append(args, *opts.AskedIn)
		}
		if opts.NoTargetOnly {
			conditions = append(conditions, "(to_agent IS NULL OR to_agent = '')")
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanQuestions(rows)
}

func scanQuestion(scanner interface{ Scan(dest ...any) error }) (types.Question, error) {
	var row questionRow
	if err := scanner.Scan(&row.GUID, &row.Re, &row.FromAgent, &row.ToAgent, &row.Status, &row.ThreadGUID, &row.AskedIn, &row.AnsweredIn, &row.Options, &row.CreatedAt); err != nil {
		return types.Question{}, err
	}
	return row.toQuestion(), nil
}

func scanQuestions(rows *sql.Rows) ([]types.Question, error) {
	var questions []types.Question
	for rows.Next() {
		question, err := scanQuestion(rows)
		if err != nil {
			return nil, err
		}
		questions = append(questions, question)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return questions, nil
}

type questionRow struct {
	GUID       string
	Re         string
	FromAgent  string
	ToAgent    sql.NullString
	Status     sql.NullString
	ThreadGUID sql.NullString
	AskedIn    sql.NullString
	AnsweredIn sql.NullString
	Options    sql.NullString
	CreatedAt  int64
}

func (row questionRow) toQuestion() types.Question {
	status := types.QuestionStatusUnasked
	if row.Status.Valid && row.Status.String != "" {
		status = types.QuestionStatus(row.Status.String)
	}
	var options []types.QuestionOption
	if row.Options.Valid && row.Options.String != "" && row.Options.String != "[]" {
		_ = json.Unmarshal([]byte(row.Options.String), &options)
	}
	return types.Question{
		GUID:       row.GUID,
		Re:         row.Re,
		FromAgent:  row.FromAgent,
		ToAgent:    nullStringPtr(row.ToAgent),
		Status:     status,
		ThreadGUID: nullStringPtr(row.ThreadGUID),
		AskedIn:    nullStringPtr(row.AskedIn),
		AnsweredIn: nullStringPtr(row.AnsweredIn),
		Options:    options,
		CreatedAt:  row.CreatedAt,
	}
}
