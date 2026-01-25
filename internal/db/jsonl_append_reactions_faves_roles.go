package db

import "time"

// AppendReaction appends a reaction record to JSONL.
func AppendReaction(projectPath, messageGUID, agentID, emoji string, reactedAt int64) error {
	record := ReactionJSONLRecord{
		Type:        "reaction",
		MessageGUID: messageGUID,
		AgentID:     agentID,
		Emoji:       emoji,
		ReactedAt:   reactedAt,
	}
	filePath, err := sharedMachinePath(projectPath, messagesFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendAgentFave appends a fave record to JSONL.
func AppendAgentFave(projectPath, agentID, itemType, itemGUID string, favedAt int64) error {
	record := AgentFaveJSONLRecord{
		Type:     "agent_fave",
		AgentID:  agentID,
		ItemType: itemType,
		ItemGUID: itemGUID,
		FavedAt:  favedAt,
	}
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendAgentUnfave appends an unfave record to JSONL.
func AppendAgentUnfave(projectPath, agentID, itemType, itemGUID string, unfavedAt int64) error {
	record := AgentUnfaveJSONLRecord{
		Type:      "agent_unfave",
		AgentID:   agentID,
		ItemType:  itemType,
		ItemGUID:  itemGUID,
		UnfavedAt: unfavedAt,
	}
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendFaveRemove appends a fave removal tombstone to JSONL.
func AppendFaveRemove(projectPath, agentID, itemType, itemGUID string, removedAt int64) error {
	if removedAt == 0 {
		removedAt = time.Now().UnixMilli()
	}
	record := FaveRemoveJSONLRecord{
		Type:     "fave_remove",
		AgentID:  agentID,
		ItemType: itemType,
		ItemGUID: itemGUID,
		TS:       removedAt,
	}
	if IsMultiMachineMode(projectPath) {
		seq, err := GetNextSequence(projectPath)
		if err != nil {
			return err
		}
		record.Seq = seq
	}
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendRoleHold appends a role hold (persistent assignment) record to JSONL.
func AppendRoleHold(projectPath, agentID, roleName string, assignedAt int64) error {
	record := RoleHoldJSONLRecord{
		Type:       "role_hold",
		AgentID:    agentID,
		RoleName:   roleName,
		AssignedAt: assignedAt,
	}
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendRoleDrop appends a role drop (removal) record to JSONL.
func AppendRoleDrop(projectPath, agentID, roleName string, droppedAt int64) error {
	record := RoleDropJSONLRecord{
		Type:      "role_drop",
		AgentID:   agentID,
		RoleName:  roleName,
		DroppedAt: droppedAt,
	}
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendRoleRelease appends a role release tombstone to JSONL.
func AppendRoleRelease(projectPath, agentID, roleName string, releasedAt int64) error {
	if releasedAt == 0 {
		releasedAt = time.Now().Unix()
	}
	record := RoleReleaseJSONLRecord{
		Type:     "role_release",
		AgentID:  agentID,
		RoleName: roleName,
		TS:       releasedAt,
	}
	if IsMultiMachineMode(projectPath) {
		seq, err := GetNextSequence(projectPath)
		if err != nil {
			return err
		}
		record.Seq = seq
	}
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendRolePlay appends a session-scoped role play record to JSONL.
func AppendRolePlay(projectPath, agentID, roleName string, sessionID *string, startedAt int64) error {
	record := RolePlayJSONLRecord{
		Type:      "role_play",
		AgentID:   agentID,
		RoleName:  roleName,
		SessionID: sessionID,
		StartedAt: startedAt,
	}
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendRoleStop appends a role stop record to JSONL.
func AppendRoleStop(projectPath, agentID, roleName string, stoppedAt int64) error {
	record := RoleStopJSONLRecord{
		Type:      "role_stop",
		AgentID:   agentID,
		RoleName:  roleName,
		StoppedAt: stoppedAt,
	}
	filePath, err := agentStatePath(projectPath)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}
