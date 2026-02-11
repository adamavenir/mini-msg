package hostedsync

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func readJSON(path string, out any) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return true, err
	}
	return true, nil
}

func writeJSONAtomic(path string, value any) error {
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
