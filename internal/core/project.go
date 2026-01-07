package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Project represents a fray project.
type Project struct {
	Root   string
	DBPath string
}

// DiscoverProject walks up from startDir to find a .fray directory.
func DiscoverProject(startDir string) (Project, error) {
	current := startDir
	if current == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return Project{}, err
		}
		current = cwd
	}
	current, err := filepath.Abs(current)
	if err != nil {
		return Project{}, err
	}

	for {
		frayDir := filepath.Join(current, ".fray")
		info, err := os.Stat(frayDir)
		if err == nil && info.IsDir() {
			dbPath := filepath.Join(frayDir, "fray.db")
			// DB file is optional - OpenDatabase will rebuild from JSONL if needed
			// Just check that either db or at least one JSONL file exists
			hasDB := false
			if _, err := os.Stat(dbPath); err == nil {
				hasDB = true
			}
			hasJSONL := false
			for _, name := range []string{"messages.jsonl", "agents.jsonl"} {
				if _, err := os.Stat(filepath.Join(frayDir, name)); err == nil {
					hasJSONL = true
					break
				}
			}
			if !hasDB && !hasJSONL {
				return Project{}, fmt.Errorf("fray database not found. Run 'fray init' first")
			}
			return Project{Root: current, DBPath: dbPath}, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return Project{}, fmt.Errorf("not initialized. Run 'fray init' first")
		}
		current = parent
	}
}

// InitProject initializes a new fray project at dir.
func InitProject(dir string, force bool) (Project, error) {
	root := dir
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return Project{}, err
		}
		root = cwd
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Project{}, err
	}

	frayDir := filepath.Join(root, ".fray")
	dbPath := filepath.Join(frayDir, "fray.db")

	if info, err := os.Stat(frayDir); err == nil && info.IsDir() && !force {
		return Project{}, fmt.Errorf("already initialized. Use --force to reinitialize")
	}

	if err := os.MkdirAll(frayDir, 0o755); err != nil {
		return Project{}, err
	}
	EnsureFrayGitignore(frayDir)

	if force {
		if err := os.Remove(dbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Project{}, err
		}
	}

	return Project{Root: root, DBPath: dbPath}, nil
}

// EnsureFrayGitignore ensures .fray/.gitignore contains sqlite ignores.
func EnsureFrayGitignore(frayDir string) {
	gitignore := filepath.Join(frayDir, ".gitignore")
	entries := []string{"*.db", "*.db-wal", "*.db-shm"}

	data, err := os.ReadFile(gitignore)
	if err != nil {
		_ = os.WriteFile(gitignore, []byte(stringsJoin(entries, "\n")+"\n"), 0o644)
		return
	}
	content := string(data)

	lines := map[string]bool{}
	for _, line := range splitLines(content) {
		lines[line] = true
	}

	missing := []string{}
	for _, entry := range entries {
		if !lines[entry] {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return
	}
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content += "\n"
	}
	content += stringsJoin(missing, "\n") + "\n"
	_ = os.WriteFile(gitignore, []byte(content), 0o644)
}

func splitLines(value string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(value); i++ {
		if value[i] == '\n' {
			lines = append(lines, value[start:i])
			start = i + 1
		}
	}
	if start < len(value) {
		lines = append(lines, value[start:])
	}
	return lines
}

func stringsJoin(values []string, sep string) string {
	if len(values) == 0 {
		return ""
	}
	if len(values) == 1 {
		return values[0]
	}

	length := 0
	for _, v := range values {
		length += len(v)
	}
	length += len(sep) * (len(values) - 1)

	buf := make([]byte, 0, length)
	for i, v := range values {
		if i > 0 {
			buf = append(buf, sep...)
		}
		buf = append(buf, v...)
	}
	return string(buf)
}
