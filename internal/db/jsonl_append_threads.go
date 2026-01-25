package db

import (
	"time"

	"github.com/adamavenir/fray/internal/types"
)

// AppendThread appends a thread record to JSONL.
func AppendThread(projectPath string, thread types.Thread, subscribed []string) error {
	record := ThreadJSONLRecord{
		Type:              "thread",
		GUID:              thread.GUID,
		Name:              thread.Name,
		ParentThread:      thread.ParentThread,
		Subscribed:        subscribed,
		Status:            string(thread.Status),
		ThreadType:        string(thread.Type),
		CreatedAt:         thread.CreatedAt,
		AnchorMessageGUID: thread.AnchorMessageGUID,
		AnchorHidden:      thread.AnchorHidden,
		LastActivityAt:    thread.LastActivityAt,
	}
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUpdate appends a thread update record to JSONL.
func AppendThreadUpdate(projectPath string, update ThreadUpdateJSONLRecord) error {
	update.Type = "thread_update"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, update); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadDelete appends a thread deletion tombstone to JSONL.
func AppendThreadDelete(projectPath, threadID string, deletedAt int64) error {
	if deletedAt == 0 {
		deletedAt = time.Now().Unix()
	}
	record := ThreadDeleteJSONLRecord{
		Type:     "thread_delete",
		ThreadID: threadID,
		TS:       deletedAt,
	}
	if IsMultiMachineMode(projectPath) {
		seq, err := GetNextSequence(projectPath)
		if err != nil {
			return err
		}
		record.Seq = seq
	}
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, record); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadSubscribe appends a thread subscribe event to JSONL.
func AppendThreadSubscribe(projectPath string, event ThreadSubscribeJSONLRecord) error {
	event.Type = "thread_subscribe"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUnsubscribe appends a thread unsubscribe event to JSONL.
func AppendThreadUnsubscribe(projectPath string, event ThreadUnsubscribeJSONLRecord) error {
	event.Type = "thread_unsubscribe"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadMessage appends a thread message membership event to JSONL.
func AppendThreadMessage(projectPath string, event ThreadMessageJSONLRecord) error {
	event.Type = "thread_message"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadMessageRemove appends a thread message removal event to JSONL.
func AppendThreadMessageRemove(projectPath string, event ThreadMessageRemoveJSONLRecord) error {
	event.Type = "thread_message_remove"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadPin appends a thread pin event to JSONL.
func AppendThreadPin(projectPath string, event ThreadPinJSONLRecord) error {
	event.Type = "thread_pin"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUnpin appends a thread unpin event to JSONL.
func AppendThreadUnpin(projectPath string, event ThreadUnpinJSONLRecord) error {
	event.Type = "thread_unpin"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadMute appends a thread mute event to JSONL.
func AppendThreadMute(projectPath string, event ThreadMuteJSONLRecord) error {
	event.Type = "thread_mute"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}

// AppendThreadUnmute appends a thread unmute event to JSONL.
func AppendThreadUnmute(projectPath string, event ThreadUnmuteJSONLRecord) error {
	event.Type = "thread_unmute"
	filePath, err := sharedMachinePath(projectPath, threadsFile)
	if err != nil {
		return err
	}
	if err := appendSharedJSONLine(projectPath, filePath, event); err != nil {
		return err
	}
	touchDatabaseFile(projectPath)
	return nil
}
