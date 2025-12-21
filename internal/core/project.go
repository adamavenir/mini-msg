package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Project represents an mm project.
type Project struct {
	Root   string
	DBPath string
}

// DiscoverProject walks up from startDir to find a .mm directory.
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
		mmDir := filepath.Join(current, ".mm")
		info, err := os.Stat(mmDir)
		if err == nil && info.IsDir() {
			dbPath := filepath.Join(mmDir, "mm.db")
			if _, err := os.Stat(dbPath); err != nil {
				return Project{}, fmt.Errorf("mm database not found. Run 'mm init' first")
			}
			return Project{Root: current, DBPath: dbPath}, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return Project{}, fmt.Errorf("not initialized. Run 'mm init' first")
		}
		current = parent
	}
}

// InitProject initializes a new mm project at dir.
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

	mmDir := filepath.Join(root, ".mm")
	dbPath := filepath.Join(mmDir, "mm.db")

	if info, err := os.Stat(mmDir); err == nil && info.IsDir() && !force {
		return Project{}, fmt.Errorf("already initialized. Use --force to reinitialize")
	}

	if err := os.MkdirAll(mmDir, 0o755); err != nil {
		return Project{}, err
	}
	EnsureMMGitignore(mmDir)

	if force {
		if err := os.Remove(dbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Project{}, err
		}
	}

	return Project{Root: root, DBPath: dbPath}, nil
}

// EnsureMMGitignore ensures .mm/.gitignore contains sqlite ignores.
func EnsureMMGitignore(mmDir string) {
	gitignore := filepath.Join(mmDir, ".gitignore")
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
