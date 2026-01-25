package db

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func readJSONLLines(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	truncated := false
	if info, err := file.Stat(); err == nil && info.Size() > 0 {
		buf := make([]byte, 1)
		if _, err := file.ReadAt(buf, info.Size()-1); err == nil {
			truncated = buf[0] != '\n'
		}
	}

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if truncated && len(lines) > 0 {
		log.Printf("warning: truncated JSONL line skipped in %s", filePath)
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

func readJSONLFile[T any](filePath string) ([]T, error) {
	lines, err := readJSONLLines(filePath)
	if err != nil {
		return nil, err
	}

	records := make([]T, 0, len(lines))
	for _, line := range lines {
		var record T
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		records = append(records, record)
	}

	return records, nil
}

type jsonlLine struct {
	Line    string
	Machine string
	Index   int
}

type orderedJSONLEvent struct {
	Line    string
	Machine string
	Seq     int64
	TS      int64
	Index   int
}

func readSharedJSONLLines(projectPath, fileName string) ([]jsonlLine, error) {
	dirs := GetSharedMachinesDirs(projectPath)
	if len(dirs) == 0 {
		return nil, nil
	}
	lines := make([]jsonlLine, 0)
	for _, dir := range dirs {
		machine := filepath.Base(dir)
		filePath := filepath.Join(dir, fileName)
		fileLines, err := readJSONLLines(filePath)
		if err != nil {
			return nil, err
		}
		for idx, line := range fileLines {
			lines = append(lines, jsonlLine{
				Line:    line,
				Machine: machine,
				Index:   idx,
			})
		}
	}
	return lines, nil
}

func readLocalRuntimeLines(projectPath string) ([]string, error) {
	return readJSONLLines(GetLocalRuntimePath(projectPath))
}

func parseRawEnvelope(line string) (map[string]json.RawMessage, string) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, ""
	}
	var typ string
	if rawType, ok := raw["type"]; ok {
		_ = json.Unmarshal(rawType, &typ)
	}
	return raw, typ
}

func parseSeq(raw map[string]json.RawMessage, fallback int64) int64 {
	if raw == nil {
		return fallback
	}
	if rawSeq, ok := raw["seq"]; ok {
		var seq int64
		if err := json.Unmarshal(rawSeq, &seq); err == nil {
			return seq
		}
	}
	return fallback
}

func parseTimestamp(raw map[string]json.RawMessage, fields []string) int64 {
	for _, field := range fields {
		value, ok := raw[field]
		if !ok {
			continue
		}
		if string(value) == "null" {
			continue
		}
		var ts int64
		if err := json.Unmarshal(value, &ts); err == nil {
			return ts
		}
	}
	return 0
}

func sortOrderedEvents(events []orderedJSONLEvent) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].TS != events[j].TS {
			return events[i].TS < events[j].TS
		}
		if events[i].Machine != events[j].Machine {
			return events[i].Machine < events[j].Machine
		}
		if events[i].Seq != events[j].Seq {
			return events[i].Seq < events[j].Seq
		}
		return events[i].Index < events[j].Index
	})
}
