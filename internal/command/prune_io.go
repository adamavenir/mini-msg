package command

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// createTombstone generates a tombstone message for pruned messages.
func createTombstone(prunedMessages []db.MessageJSONLRecord, home string) *db.MessageJSONLRecord {
	if len(prunedMessages) == 0 {
		return nil
	}

	// Collect unique participants
	participants := make(map[string]struct{})
	for _, msg := range prunedMessages {
		if msg.FromAgent != "" && msg.FromAgent != "system" {
			participants[msg.FromAgent] = struct{}{}
		}
	}
	var participantList []string
	for p := range participants {
		participantList = append(participantList, "@"+p)
	}

	// Find first and last message IDs (messages are already in chronological order)
	firstID := prunedMessages[0].ID
	lastID := prunedMessages[len(prunedMessages)-1].ID

	// Format: "pruned: N messages between @agent1, @agent2 from #msg-xxx to #msg-yyy"
	body := fmt.Sprintf("pruned: %d messages between %s from #%s to #%s",
		len(prunedMessages),
		strings.Join(participantList, ", "),
		firstID,
		lastID,
	)

	now := time.Now().Unix()
	msgID, err := core.GenerateGUID("msg")
	if err != nil {
		// Fallback to timestamp-based ID if GUID generation fails
		msgID = fmt.Sprintf("msg-%d", now)
	}
	return &db.MessageJSONLRecord{
		Type:      "message",
		ID:        msgID,
		Home:      home,
		FromAgent: "system",
		Body:      body,
		MsgType:   types.MessageTypeTombstone,
		TS:        now,
	}
}

// readJSONLLines reads all non-empty lines from a JSONL file.
func readJSONLLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

// writeMessagesWithEvents writes messages and their related events to the JSONL file.
func writeMessagesWithEvents(path string, messages []db.MessageJSONLRecord, keepIDs map[string]struct{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// Read original JSONL lines to preserve events
	originalLines, err := readJSONLLines(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var builder strings.Builder

	// Write messages first
	for _, record := range messages {
		record.Type = "message"
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		builder.Write(data)
		builder.WriteByte('\n')
	}

	// Write events for kept messages
	for _, line := range originalLines {
		var envelope struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "message":
			// Already written above
			continue
		case "message_update":
			// Check if the updated message is being kept
			if _, ok := keepIDs[envelope.ID]; ok {
				builder.WriteString(line)
				builder.WriteByte('\n')
			}
		case "message_pin", "message_unpin":
			// These use message_guid instead of id
			var pinEvent struct {
				MessageGUID string `json:"message_guid"`
			}
			if err := json.Unmarshal([]byte(line), &pinEvent); err != nil {
				continue
			}
			if _, ok := keepIDs[pinEvent.MessageGUID]; ok {
				builder.WriteString(line)
				builder.WriteByte('\n')
			}
		case "message_move":
			var moveEvent struct {
				MessageGUID string `json:"message_guid"`
			}
			if err := json.Unmarshal([]byte(line), &moveEvent); err != nil {
				continue
			}
			if _, ok := keepIDs[moveEvent.MessageGUID]; ok {
				builder.WriteString(line)
				builder.WriteByte('\n')
			}
		}
	}

	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func writeMessages(path string, records []db.MessageJSONLRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var builder strings.Builder
	for _, record := range records {
		record.Type = "message"
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		builder.Write(data)
		builder.WriteByte('\n')
	}

	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func appendFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	return err
}
