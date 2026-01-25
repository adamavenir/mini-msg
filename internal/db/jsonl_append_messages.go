package db

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/types"
)

// AppendMessage appends a message record to JSONL.
func AppendMessage(projectPath string, message types.Message) error {
	home := message.Home
	if home == "" {
		home = "room"
	}
	// Convert new reactions format to legacy format for JSONL compatibility.
	// Reactions are now stored in separate reaction records, so this is usually empty.
	legacyReactions := ConvertToLegacyReactions(message.Reactions)
	record := MessageJSONLRecord{
		Type:             "message",
		ID:               message.ID,
		ChannelID:        message.ChannelID,
		Home:             home,
		FromAgent:        message.FromAgent,
		SessionID:        message.SessionID,
		Body:             message.Body,
		Mentions:         message.Mentions,
		ForkSessions:     message.ForkSessions,
		Reactions:        legacyReactions,
		MsgType:          message.Type,
		References:       message.References,
		SurfaceMessage:   message.SurfaceMessage,
		ReplyTo:          message.ReplyTo,
		QuoteMessageGUID: message.QuoteMessageGUID,
		TS:               message.TS,
		EditedAt:         message.EditedAt,
		ArchivedAt:       message.ArchivedAt,
	}

	if IsMultiMachineMode(projectPath) {
		config, err := ReadProjectConfig(projectPath)
		if err != nil {
			return err
		}
		var aliases map[string]string
		if config != nil {
			aliases = config.MachineAliases
		}
		origin := GetLocalMachineID(projectPath)
		if origin == "" {
			return fmt.Errorf("local machine id not set")
		}
		if err := ensureAgentDescriptor(projectPath, message.FromAgent, message.TS); err != nil {
			return err
		}
		seq, err := GetNextSequence(projectPath)
		if err != nil {
			return err
		}
		record.Origin = origin
		record.Seq = seq
		record.Mentions = core.EncodeMentions(record.Mentions, origin, aliases)
		record.ForkSessions = core.EncodeForkSessions(record.ForkSessions, origin, aliases)
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

// AppendMessageUpdate appends an update record to JSONL.
func AppendMessageUpdate(projectPath string, update MessageUpdateJSONLRecord) error {
	update.Type = "message_update"
	filePath, err := sharedMachinePath(projectPath, messagesFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessageDelete appends a message deletion tombstone to JSONL.
func AppendMessageDelete(projectPath, messageID string, deletedBy *string, deletedAt int64) error {
	if deletedAt == 0 {
		deletedAt = time.Now().Unix()
	}
	record := MessageDeleteJSONLRecord{
		Type:      "message_delete",
		ID:        messageID,
		DeletedBy: deletedBy,
		TS:        deletedAt,
	}
	if IsMultiMachineMode(projectPath) {
		seq, err := GetNextSequence(projectPath)
		if err != nil {
			return err
		}
		record.Seq = seq
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

// AppendMessagePin appends a message pin event to JSONL.
func AppendMessagePin(projectPath string, event MessagePinJSONLRecord) error {
	event.Type = "message_pin"
	var filePath string
	var err error
	if IsMultiMachineMode(projectPath) {
		filePath, err = sharedMachinePath(projectPath, threadsFile)
	} else {
		frayDir := resolveFrayDir(projectPath)
		filePath = filepath.Join(frayDir, messagesFile)
	}
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessageUnpin appends a message unpin event to JSONL.
func AppendMessageUnpin(projectPath string, event MessageUnpinJSONLRecord) error {
	event.Type = "message_unpin"
	var filePath string
	var err error
	if IsMultiMachineMode(projectPath) {
		filePath, err = sharedMachinePath(projectPath, threadsFile)
	} else {
		frayDir := resolveFrayDir(projectPath)
		filePath = filepath.Join(frayDir, messagesFile)
	}
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendMessageMove appends a message move event to JSONL.
func AppendMessageMove(projectPath string, event MessageMoveJSONLRecord) error {
	event.Type = "message_move"
	filePath, err := sharedMachinePath(projectPath, messagesFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}
