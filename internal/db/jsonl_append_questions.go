package db

import "github.com/adamavenir/fray/internal/types"

// AppendQuestion appends a question record to JSONL.
func AppendQuestion(projectPath string, question types.Question) error {
	record := QuestionJSONLRecord{
		Type:       "question",
		GUID:       question.GUID,
		Re:         question.Re,
		FromAgent:  question.FromAgent,
		ToAgent:    question.ToAgent,
		Status:     string(question.Status),
		ThreadGUID: question.ThreadGUID,
		AskedIn:    question.AskedIn,
		AnsweredIn: question.AnsweredIn,
		Options:    question.Options,
		CreatedAt:  question.CreatedAt,
	}
	filePath, err := sharedMachinePath(projectPath, questionsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendQuestionUpdate appends a question update record to JSONL.
func AppendQuestionUpdate(projectPath string, update QuestionUpdateJSONLRecord) error {
	update.Type = "question_update"
	filePath, err := sharedMachinePath(projectPath, questionsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}
