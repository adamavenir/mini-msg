package mcp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
)

const mcpAgentBase = "desktop"

func loadPersistedAgentID(projectPath string) string {
	configPath, err := mcpConfigPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	mapping := map[string]string{}
	if err := json.Unmarshal(data, &mapping); err != nil {
		return ""
	}

	return mapping[projectPath]
}

func persistAgentID(projectPath, agentID string) {
	configPath, err := mcpConfigPath()
	if err != nil {
		logf("Failed to persist agent ID: %v", err)
		return
	}

	mapping := map[string]string{}
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &mapping)
	}

	mapping[projectPath] = agentID

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		logf("Failed to persist agent ID: %v", err)
		return
	}

	payload, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		logf("Failed to persist agent ID: %v", err)
		return
	}
	payload = append(payload, '\n')

	if err := os.WriteFile(configPath, payload, 0o644); err != nil {
		logf("Failed to persist agent ID: %v", err)
	}
}

func mcpConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".config", "mm")
	return filepath.Join(configDir, "mcp-agents.json"), nil
}

func initializeMcpAgent(dbConn *sql.DB, project core.Project) (string, error) {
	projectRoot := project.Root
	persisted := loadPersistedAgentID(projectRoot)
	if persisted != "" {
		agent, err := db.GetAgent(dbConn, persisted)
		if err != nil {
			return "", err
		}
		if agent != nil {
			if agent.LeftAt != nil {
				if err := reactivateAgent(dbConn, persisted); err != nil {
					return "", err
				}
			} else {
				now := time.Now().Unix()
				updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
				if err := db.UpdateAgent(dbConn, persisted, updates); err != nil {
					return "", err
				}
			}
			if updated, err := db.GetAgent(dbConn, persisted); err == nil && updated != nil {
				_ = db.AppendAgent(project.DBPath, *updated)
			}
			return persisted, nil
		}
	}

	created, err := createMcpAgent(dbConn)
	if err != nil {
		return "", err
	}
	persistAgentID(projectRoot, created.AgentID)
	_ = db.AppendAgent(project.DBPath, created)
	logf("Created agent: %s", created.AgentID)
	return created.AgentID, nil
}

func createMcpAgent(dbConn *sql.DB) (types.Agent, error) {
	maxVersion, err := db.GetMaxVersion(dbConn, mcpAgentBase)
	if err != nil {
		return types.Agent{}, err
	}
	version := maxVersion + 1
	agentID, err := core.FormatAgentID(mcpAgentBase, version)
	if err != nil {
		return types.Agent{}, err
	}

	now := time.Now().Unix()
	status := "Claude Desktop MCP session"
	agent := types.Agent{
		AgentID:      agentID,
		Status:       &status,
		RegisteredAt: now,
		LastSeen:     now,
	}

	if err := db.CreateAgent(dbConn, agent); err != nil {
		return types.Agent{}, err
	}

	created, err := db.GetAgent(dbConn, agentID)
	if err != nil {
		return types.Agent{}, err
	}
	if created == nil {
		return types.Agent{}, fmt.Errorf("failed to create agent %s", agentID)
	}
	return *created, nil
}

func reactivateAgent(dbConn *sql.DB, agentID string) error {
	now := time.Now().Unix()
	updates := db.AgentUpdates{
		LeftAt:   types.OptionalInt64{Set: true, Value: nil},
		LastSeen: types.OptionalInt64{Set: true, Value: &now},
	}
	if err := db.UpdateAgent(dbConn, agentID, updates); err != nil {
		return err
	}
	logf("Reactivated agent: %s", agentID)
	return nil
}
