package db

import (
	"time"

	"github.com/adamavenir/fray/internal/types"
)

// AppendWakeCondition appends a wake condition record to JSONL.
func AppendWakeCondition(projectPath string, condition types.WakeCondition) error {
	record := WakeConditionJSONLRecord{
		Type:           "wake_condition",
		GUID:           condition.GUID,
		AgentID:        condition.AgentID,
		SetBy:          condition.SetBy,
		CondType:       string(condition.Type),
		Pattern:        condition.Pattern,
		OnAgents:       condition.OnAgents,
		InThread:       condition.InThread,
		AfterMs:        condition.AfterMs,
		UseRouter:      condition.UseRouter,
		Prompt:         condition.Prompt,
		PromptText:     condition.PromptText,
		PollIntervalMs: condition.PollIntervalMs,
		PersistMode:    string(condition.PersistMode),
		Paused:         condition.Paused,
		CreatedAt:      condition.CreatedAt,
		ExpiresAt:      condition.ExpiresAt,
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionClear appends a wake condition clear record to JSONL.
func AppendWakeConditionClear(projectPath, agentID string) error {
	record := WakeConditionClearJSONLRecord{
		Type:      "wake_condition_clear",
		AgentID:   agentID,
		ClearedAt: time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionDelete appends a wake condition delete record to JSONL.
func AppendWakeConditionDelete(projectPath, guid string) error {
	record := WakeConditionDeleteJSONLRecord{
		Type:      "wake_condition_delete",
		GUID:      guid,
		DeletedAt: time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionPause appends a wake condition pause record to JSONL.
func AppendWakeConditionPause(projectPath, agentID string) error {
	record := WakeConditionPauseJSONLRecord{
		Type:     "wake_condition_pause",
		AgentID:  agentID,
		PausedAt: time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionResume appends a wake condition resume record to JSONL.
func AppendWakeConditionResume(projectPath, agentID string) error {
	record := WakeConditionResumeJSONLRecord{
		Type:      "wake_condition_resume",
		AgentID:   agentID,
		ResumedAt: time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionClearByBye appends a wake condition clear-by-bye record to JSONL.
func AppendWakeConditionClearByBye(projectPath, agentID string) error {
	record := WakeConditionClearByByeJSONLRecord{
		Type:      "wake_condition_clear_by_bye",
		AgentID:   agentID,
		ClearedAt: time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendWakeConditionReset appends a wake condition reset record to JSONL.
func AppendWakeConditionReset(projectPath, guid string, expiresAt int64) error {
	record := WakeConditionResetJSONLRecord{
		Type:      "wake_condition_reset",
		GUID:      guid,
		ExpiresAt: expiresAt,
		ResetAt:   time.Now().Unix(),
	}
	if err := appendJSONLine(runtimePath(projectPath), record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}
