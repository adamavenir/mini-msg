package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

// stockAgent represents a suggested agent for interactive init.
type stockAgent struct {
	Name        string
	Description string
	Driver      string // default driver
}

// stockAgents is the default set of agents to suggest during init.
var stockAgents = []stockAgent{
	{Name: "dev", Description: "development work", Driver: "claude"},
	{Name: "arch", Description: "architecture review/plans", Driver: "claude"},
	{Name: "desi", Description: "design review", Driver: "claude"},
	{Name: "pm", Description: "project coordination", Driver: "claude"},
}

// promptAndCreateAgents shows stock agents and creates selected ones.
func promptAndCreateAgents(dbPath string) []string {
	if !isTTY(os.Stdin) {
		return nil
	}

	fmt.Println("")
	fmt.Println("Suggested agents (select with numbers, e.g., 1,2,4 or 'all' or 'none'):")
	for i, agent := range stockAgents {
		fmt.Printf("  %d. %s - %s\n", i+1, agent.Name, agent.Description)
	}
	fmt.Print("Select [default=all]: ")

	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	trimmed := strings.TrimSpace(strings.ToLower(text))

	var selectedIndices []int
	if trimmed == "" || trimmed == "all" {
		for i := range stockAgents {
			selectedIndices = append(selectedIndices, i)
		}
	} else if trimmed == "none" {
		return nil
	} else {
		parts := strings.Split(trimmed, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			var idx int
			if _, err := fmt.Sscanf(part, "%d", &idx); err == nil && idx >= 1 && idx <= len(stockAgents) {
				selectedIndices = append(selectedIndices, idx-1)
			}
		}
	}

	if len(selectedIndices) == 0 {
		return nil
	}

	// Ask about driver customization
	fmt.Println("")
	fmt.Println("Default driver: claude (also supports codex, opencode)")
	if !promptYesNo("Use defaults?", true) {
		// Let user customize per-agent
		for _, idx := range selectedIndices {
			agent := &stockAgents[idx]
			fmt.Printf("Driver for %s [claude/codex/opencode, default=%s]: ", agent.Name, agent.Driver)
			driverText, _ := reader.ReadString('\n')
			driverTrimmed := strings.TrimSpace(strings.ToLower(driverText))
			if driverTrimmed == "claude" || driverTrimmed == "codex" || driverTrimmed == "opencode" {
				agent.Driver = driverTrimmed
			}
		}
	}

	// Create the agents
	var created []string
	for _, idx := range selectedIndices {
		agent := stockAgents[idx]
		if err := createManagedAgent(dbPath, agent.Name, agent.Driver); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create agent %s: %v\n", agent.Name, err)
			continue
		}
		created = append(created, agent.Name)
	}

	return created
}

// createManagedAgent creates a managed agent configuration.
func createManagedAgent(dbPath string, name string, driver string) error {
	project, err := core.DiscoverProject("")
	if err != nil {
		return err
	}

	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		return err
	}
	defer dbConn.Close()

	// Check if agent already exists
	existing, _ := db.GetAgent(dbConn, name)
	if existing != nil {
		return nil // Already exists, skip
	}

	// Create the managed agent
	agentGUID, err := core.GenerateGUID("usr")
	if err != nil {
		return err
	}

	config, err := db.ReadProjectConfig(dbPath)
	if err != nil {
		return err
	}

	channelID := ""
	if config != nil {
		channelID = config.ChannelID
	}

	now := time.Now().Unix()
	agent := types.Agent{
		GUID:         agentGUID,
		AgentID:      name,
		RegisteredAt: now,
		LastSeen:     now,
		Managed:      true,
		Presence:     types.PresenceOffline,
		Invoke: &types.InvokeConfig{
			Driver: driver,
		},
	}
	_ = channelID // used by AppendAgent internally

	return db.AppendAgent(dbPath, agent)
}
