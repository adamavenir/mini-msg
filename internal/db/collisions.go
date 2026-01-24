package db

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CollisionEntry captures a single machine's record for a colliding GUID.
type CollisionEntry struct {
	Machine string `json:"machine"`
	TS      int64  `json:"ts"`
	Preview string `json:"preview,omitempty"`
}

// GUIDCollision captures collisions for a GUID across machines.
type GUIDCollision struct {
	Type    string           `json:"type"`
	GUID    string           `json:"guid"`
	Entries []CollisionEntry `json:"entries"`
}

// CollisionLog stores GUID collision data for CLI access.
type CollisionLog struct {
	UpdatedAt  int64           `json:"updated_at"`
	Collisions []GUIDCollision `json:"collisions"`
}

func updateCollisionLog(projectPath string) error {
	if !IsMultiMachineMode(projectPath) {
		return nil
	}

	collisions, err := detectGUIDCollisions(projectPath)
	if err != nil {
		return err
	}

	for _, collision := range collisions {
		machines := make([]string, 0, len(collision.Entries))
		for _, entry := range collision.Entries {
			machines = append(machines, entry.Machine)
		}
		sort.Strings(machines)
		log.Printf("warning: GUID collision: %s %s from %s", collision.Type, collision.GUID, strings.Join(machines, ", "))
	}

	return writeCollisionLog(projectPath, collisions)
}

func detectGUIDCollisions(projectPath string) ([]GUIDCollision, error) {
	type collisionKey struct {
		typ  string
		guid string
	}

	seen := make(map[collisionKey]map[string]CollisionEntry)
	collisions := make(map[collisionKey]map[string]CollisionEntry)

	addEntry := func(typ, guid, machine, preview string, ts int64) {
		if guid == "" || machine == "" {
			return
		}
		key := collisionKey{typ: typ, guid: guid}
		entries := seen[key]
		if entries == nil {
			entries = make(map[string]CollisionEntry)
			seen[key] = entries
		}
		if _, ok := entries[machine]; ok {
			return
		}
		entries[machine] = CollisionEntry{
			Machine: machine,
			TS:      ts,
			Preview: truncatePreview(preview),
		}
		if len(entries) > 1 {
			collisions[key] = entries
		}
	}

	messageLines, err := readSharedJSONLLines(projectPath, messagesFile)
	if err != nil {
		return nil, err
	}
	for _, entry := range messageLines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil || typ != "message" {
			continue
		}
		var record struct {
			ID        string `json:"id"`
			FromAgent string `json:"from_agent"`
			Body      string `json:"body"`
		}
		if err := json.Unmarshal([]byte(entry.Line), &record); err != nil {
			continue
		}
		preview := strings.TrimSpace(record.Body)
		if record.FromAgent != "" {
			preview = fmt.Sprintf("%s: %s", record.FromAgent, preview)
		}
		addEntry("message", record.ID, entry.Machine, preview, messageEventTimestamp(typ, raw))
	}

	threadLines, err := readSharedJSONLLines(projectPath, threadsFile)
	if err != nil {
		return nil, err
	}
	for _, entry := range threadLines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil || typ != "thread" {
			continue
		}
		var record struct {
			GUID string `json:"guid"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(entry.Line), &record); err != nil {
			continue
		}
		addEntry("thread", record.GUID, entry.Machine, record.Name, threadEventTimestamp(typ, raw))
	}

	questionLines, err := readSharedJSONLLines(projectPath, questionsFile)
	if err != nil {
		return nil, err
	}
	for _, entry := range questionLines {
		raw, typ := parseRawEnvelope(entry.Line)
		if raw == nil || typ != "question" {
			continue
		}
		var record struct {
			GUID string `json:"guid"`
			Re   string `json:"re"`
		}
		if err := json.Unmarshal([]byte(entry.Line), &record); err != nil {
			continue
		}
		addEntry("question", record.GUID, entry.Machine, record.Re, questionEventTimestamp(typ, raw))
	}

	result := make([]GUIDCollision, 0, len(collisions))
	for key, entriesMap := range collisions {
		entries := make([]CollisionEntry, 0, len(entriesMap))
		for _, entry := range entriesMap {
			entries = append(entries, entry)
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Machine < entries[j].Machine
		})
		result = append(result, GUIDCollision{
			Type:    key.typ,
			GUID:    key.guid,
			Entries: entries,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type == result[j].Type {
			return result[i].GUID < result[j].GUID
		}
		return result[i].Type < result[j].Type
	})

	return result, nil
}

// ReadCollisionLog reads the local collision log (if any).
func ReadCollisionLog(projectPath string) (*CollisionLog, error) {
	path := collisionLogPath(projectPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CollisionLog{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return &CollisionLog{}, nil
	}
	var logData CollisionLog
	if err := json.Unmarshal(data, &logData); err != nil {
		return nil, err
	}
	return &logData, nil
}

// ClearCollisionLog removes the local collision log (if present).
func ClearCollisionLog(projectPath string) error {
	path := collisionLogPath(projectPath)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeCollisionLog(projectPath string, collisions []GUIDCollision) error {
	path := collisionLogPath(projectPath)
	if len(collisions) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	payload := CollisionLog{
		UpdatedAt:  time.Now().Unix(),
		Collisions: collisions,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func collisionLogPath(projectPath string) string {
	frayDir := resolveFrayDir(projectPath)
	return filepath.Join(frayDir, "local", "collisions.json")
}

func truncatePreview(preview string) string {
	preview = strings.TrimSpace(preview)
	preview = strings.ReplaceAll(preview, "\n", " ")
	if len(preview) > 120 {
		return preview[:117] + "..."
	}
	return preview
}
