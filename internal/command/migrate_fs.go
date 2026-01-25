package command

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/adamavenir/fray/internal/db"
)

func writeJSONLFile(filePath string, records any) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}

	var lines []string
	switch v := records.(type) {
	case []db.AgentJSONLRecord:
		lines = make([]string, 0, len(v))
		for _, record := range v {
			row, err := json.Marshal(record)
			if err != nil {
				return err
			}
			lines = append(lines, string(row))
		}
	case []db.MessageJSONLRecord:
		lines = make([]string, 0, len(v))
		for _, record := range v {
			row, err := json.Marshal(record)
			if err != nil {
				return err
			}
			lines = append(lines, string(row))
		}
	default:
		return fmt.Errorf("unsupported JSONL record type")
	}

	if len(lines) == 0 {
		return os.WriteFile(filePath, []byte(""), 0o644)
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(filePath, []byte(content), 0o644)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
