package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func resolveFrayDir(projectPath string) string {
	if strings.HasSuffix(projectPath, ".db") {
		return filepath.Dir(projectPath)
	}
	if filepath.Base(projectPath) == ".fray" {
		return projectPath
	}
	return filepath.Join(projectPath, ".fray")
}

func ensureDir(dirPath string) error {
	return os.MkdirAll(dirPath, 0o755)
}

// touchDatabaseFile is a no-op. Previously it updated the db mtime,
// but that prevented rebuild detection (jsonlMtime > dbMtime check).
// The mtime comparison works naturally without touching the db.
func touchDatabaseFile(projectPath string) {
	// no-op: let jsonl mtime naturally trigger rebuild on next OpenDatabase
}

// UpdateProjectConfig merges updates into the project config.
func UpdateProjectConfig(projectPath string, updates ProjectConfig) (*ProjectConfig, error) {
	frayDir := resolveFrayDir(projectPath)
	if err := ensureDir(frayDir); err != nil {
		return nil, err
	}

	existing, err := ReadProjectConfig(projectPath)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		existing = &ProjectConfig{
			Version:     1,
			KnownAgents: map[string]ProjectKnownAgent{},
		}
	}

	if existing.KnownAgents == nil {
		existing.KnownAgents = map[string]ProjectKnownAgent{}
	}

	for id, agent := range updates.KnownAgents {
		prior := existing.KnownAgents[id]
		existing.KnownAgents[id] = mergeKnownAgent(prior, agent)
	}

	if updates.MachineAliases != nil {
		if existing.MachineAliases == nil {
			existing.MachineAliases = map[string]string{}
		}
		for id, alias := range updates.MachineAliases {
			existing.MachineAliases[id] = alias
		}
	}
	if updates.Sync != nil {
		existing.Sync = updates.Sync
	}

	if updates.Version != 0 {
		existing.Version = updates.Version
	}
	if updates.StorageVersion != 0 {
		existing.StorageVersion = updates.StorageVersion
	}
	if updates.ChannelID != "" {
		existing.ChannelID = updates.ChannelID
	}
	if updates.ChannelName != "" {
		existing.ChannelName = updates.ChannelName
	}
	if updates.CreatedAt != "" {
		existing.CreatedAt = updates.CreatedAt
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')

	configPath := filepath.Join(frayDir, projectConfigFile)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return nil, err
	}

	return existing, nil
}

func mergeKnownAgent(existing, updates ProjectKnownAgent) ProjectKnownAgent {
	merged := existing
	if updates.Name != nil {
		merged.Name = updates.Name
	}
	if updates.GlobalName != nil {
		merged.GlobalName = updates.GlobalName
	}
	if updates.HomeChannel != nil {
		merged.HomeChannel = updates.HomeChannel
	}
	if updates.CreatedAt != nil {
		merged.CreatedAt = updates.CreatedAt
	}
	if updates.FirstSeen != nil {
		merged.FirstSeen = updates.FirstSeen
	}
	if updates.Status != nil {
		merged.Status = updates.Status
	}
	if updates.Nicks != nil {
		merged.Nicks = updates.Nicks
	}
	return merged
}

// ReadProjectConfig reads the project config file.
func ReadProjectConfig(projectPath string) (*ProjectConfig, error) {
	frayDir := resolveFrayDir(projectPath)
	path := filepath.Join(frayDir, projectConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var config ProjectConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}
