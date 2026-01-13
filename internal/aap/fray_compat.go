package aap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// FrayInvokeConfig matches fray's types.InvokeConfig for JSONL parsing.
type FrayInvokeConfig struct {
	Driver         string         `json:"driver,omitempty"`
	Model          string         `json:"model,omitempty"`
	Trust          []string       `json:"trust,omitempty"`
	Config         map[string]any `json:"config,omitempty"`
	PromptDelivery string         `json:"prompt_delivery,omitempty"`
	SpawnTimeoutMs int64          `json:"spawn_timeout_ms,omitempty"`
	IdleAfterMs    int64          `json:"idle_after_ms,omitempty"`
	MinCheckinMs   int64          `json:"min_checkin_ms,omitempty"`
	MaxRuntimeMs   int64          `json:"max_runtime_ms,omitempty"`
}

// FrayAgent represents an agent from .fray/agents.jsonl.
type FrayAgent struct {
	Type         string            `json:"type"` // "agent" or "agent_update"
	GUID         string            `json:"guid"`
	AgentID      string            `json:"agent_id"`
	Status       *string           `json:"status,omitempty"`
	Purpose      *string           `json:"purpose,omitempty"`
	RegisteredAt int64             `json:"registered_at"`
	LastSeen     int64             `json:"last_seen"`
	Managed      bool              `json:"managed,omitempty"`
	Invoke       *FrayInvokeConfig `json:"invoke,omitempty"`
}

// LoadFrayAgents loads agents from .fray/agents.jsonl.
// Returns a map of agent_id -> FrayAgent with the latest state.
func LoadFrayAgents(path string) (map[string]*FrayAgent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open agents.jsonl: %w", err)
	}
	defer f.Close()

	agents := make(map[string]*FrayAgent)

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if line == "" {
			continue
		}

		var record map[string]interface{}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue // skip malformed lines
		}

		recordType, _ := record["type"].(string)
		switch recordType {
		case "agent":
			var agent FrayAgent
			if err := json.Unmarshal([]byte(line), &agent); err != nil {
				continue
			}
			agents[agent.AgentID] = &agent

		case "agent_update":
			// Apply partial update to existing agent
			agentID, _ := record["agent_id"].(string)
			if agentID == "" {
				continue
			}
			existing := agents[agentID]
			if existing == nil {
				continue // can't update non-existent agent
			}
			// Apply known update fields
			if status, ok := record["status"].(string); ok {
				existing.Status = &status
			}
			if lastSeen, ok := record["last_seen"].(float64); ok {
				existing.LastSeen = int64(lastSeen)
			}
			if invoke, ok := record["invoke"].(map[string]interface{}); ok {
				invokeJSON, _ := json.Marshal(invoke)
				var inv FrayInvokeConfig
				if err := json.Unmarshal(invokeJSON, &inv); err == nil {
					existing.Invoke = &inv
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan agents.jsonl: %w", err)
	}

	return agents, nil
}

// FrayAgentToIdentity converts a fray agent to an AAP identity (keyless).
func FrayAgentToIdentity(agent *FrayAgent) *Identity {
	createdAt := time.Unix(agent.RegisteredAt, 0).UTC().Format(time.RFC3339)

	record := IdentityRecord{
		Version:   Version,
		Type:      "identity",
		GUID:      agent.GUID, // Preserve fray's GUID
		Address:   "@" + agent.AgentID,
		Agent:     agent.AgentID,
		CreatedAt: createdAt,
	}

	// Add metadata from fray fields
	if agent.Purpose != nil || agent.Status != nil {
		record.Metadata = make(map[string]string)
		if agent.Purpose != nil {
			record.Metadata["purpose"] = *agent.Purpose
		}
		if agent.Status != nil {
			record.Metadata["status"] = *agent.Status
		}
	}

	return &Identity{
		Record: record,
		HasKey: false, // Legacy fray agents have no keys
	}
}
