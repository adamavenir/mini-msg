package command

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
)

type joinAgentOption struct {
	AgentID          string
	DisplayName      string
	Capabilities     []string
	LastActiveAt     int64
	LastActiveOrigin string
}

type joinAgentSelection struct {
	AgentID string
	Driver  string
}

func shouldJoinExistingProject(projectRoot string) bool {
	frayDir := filepath.Join(projectRoot, ".fray")
	info, err := os.Stat(frayDir)
	if err != nil || !info.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(frayDir, "shared")); err != nil {
		return false
	}
	if db.GetLocalMachineID(projectRoot) != "" {
		return false
	}
	if db.GetStorageVersion(projectRoot) >= 2 {
		return true
	}
	return len(db.GetSharedMachinesDirs(projectRoot)) > 0
}

func joinExistingProject(projectRoot string, useDefaults, jsonMode bool, out, errOut io.Writer) error {
	config, err := db.ReadProjectConfig(projectRoot)
	if err != nil {
		return writeInitError(errOut, jsonMode, err)
	}

	machineDirs := db.GetSharedMachinesDirs(projectRoot)
	machineIDs := make([]string, 0, len(machineDirs))
	for _, dir := range machineDirs {
		machineIDs = append(machineIDs, filepath.Base(dir))
	}
	sort.Strings(machineIDs)

	defaultID := defaultMachineID()
	localID, err := promptMachineID(projectRoot, defaultID)
	if err != nil {
		return writeInitError(errOut, jsonMode, err)
	}

	frayDir := filepath.Join(projectRoot, ".fray")
	localDir := filepath.Join(frayDir, "local")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return writeInitError(errOut, jsonMode, err)
	}
	if err := writeMachineIDFile(localDir, localID); err != nil {
		return writeInitError(errOut, jsonMode, err)
	}

	sharedMachineDir := filepath.Join(frayDir, "shared", "machines", localID)
	if err := os.MkdirAll(sharedMachineDir, 0o755); err != nil {
		return writeInitError(errOut, jsonMode, err)
	}

	if err := ensureLLMRouter(projectRoot); err != nil {
		return writeInitError(errOut, jsonMode, err)
	}

	options, err := buildJoinAgentOptions(projectRoot)
	if err != nil {
		return writeInitError(errOut, jsonMode, err)
	}
	selections := promptJoinAgentSelection(options, localID, useDefaults)
	agentsCreated, err := registerJoinAgents(projectRoot, selections)
	if err != nil {
		return writeInitError(errOut, jsonMode, err)
	}

	project := core.Project{Root: projectRoot, DBPath: filepath.Join(frayDir, "fray.db")}
	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		return writeInitError(errOut, jsonMode, err)
	}
	if err := db.RebuildDatabaseFromJSONL(dbConn, project.DBPath); err != nil {
		_ = dbConn.Close()
		return writeInitError(errOut, jsonMode, err)
	}
	_ = dbConn.Close()

	result := initResult{
		Initialized:    true,
		AlreadyExisted: true,
		Path:           projectRoot,
		AgentsCreated:  agentsCreated,
	}
	if config != nil {
		result.ChannelID = config.ChannelID
		result.ChannelName = config.ChannelName
	}

	if jsonMode {
		_ = json.NewEncoder(out).Encode(result)
		return nil
	}

	fmt.Fprintln(out, "Found existing fray channel (synced from other machines)")
	if config != nil && config.ChannelName != "" && config.ChannelID != "" {
		fmt.Fprintf(out, "  Channel: %s (%s)\n", config.ChannelName, config.ChannelID)
	}
	if len(machineIDs) > 0 {
		fmt.Fprintf(out, "  Machines: %s\n", strings.Join(machineIDs, ", "))
	}
	fmt.Fprintf(out, "✓ Created .fray/local/ for %s\n", localID)
	if len(agentsCreated) > 0 {
		fmt.Fprintf(out, "✓ Registered %d agents on %s: %s\n", len(agentsCreated), localID, strings.Join(agentsCreated, ", "))
	}
	fmt.Fprintln(out, "✓ Built cache from shared machines")
	return nil
}

func writeMachineIDFile(localDir, machineID string) error {
	if machineID == "" {
		return fmt.Errorf("machine id required")
	}
	path := filepath.Join(localDir, "machine-id")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	record := map[string]any{
		"id":         machineID,
		"seq":        0,
		"created_at": time.Now().Unix(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func buildJoinAgentOptions(projectRoot string) ([]joinAgentOption, error) {
	descriptors, err := db.ReadAgentDescriptors(projectRoot)
	if err != nil {
		return nil, err
	}
	messages, err := db.ReadMessages(projectRoot)
	if err != nil {
		return nil, err
	}

	type lastSeen struct {
		ts     int64
		origin string
	}
	lastSeenByAgent := map[string]lastSeen{}
	for _, message := range messages {
		if message.FromAgent == "" {
			continue
		}
		ts := message.TS
		entry, ok := lastSeenByAgent[message.FromAgent]
		if !ok || ts > entry.ts {
			lastSeenByAgent[message.FromAgent] = lastSeen{ts: ts, origin: message.Origin}
		}
	}

	byID := map[string]joinAgentOption{}
	for _, descriptor := range descriptors {
		if descriptor.AgentID == "" {
			continue
		}
		option := joinAgentOption{
			AgentID:      descriptor.AgentID,
			DisplayName:  descriptor.AgentID,
			Capabilities: descriptor.Capabilities,
			LastActiveAt: descriptor.TS,
		}
		if descriptor.DisplayName != nil && *descriptor.DisplayName != "" {
			option.DisplayName = *descriptor.DisplayName
		}
		if seen, ok := lastSeenByAgent[descriptor.AgentID]; ok {
			option.LastActiveAt = seen.ts
			option.LastActiveOrigin = seen.origin
		}
		byID[descriptor.AgentID] = option
	}

	for agentID, seen := range lastSeenByAgent {
		if agentID == "" {
			continue
		}
		if _, ok := byID[agentID]; ok {
			continue
		}
		byID[agentID] = joinAgentOption{
			AgentID:          agentID,
			DisplayName:      agentID,
			LastActiveAt:     seen.ts,
			LastActiveOrigin: seen.origin,
		}
	}

	if len(byID) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	options := make([]joinAgentOption, 0, len(ids))
	for _, id := range ids {
		options = append(options, byID[id])
	}
	return options, nil
}

func promptJoinAgentSelection(options []joinAgentOption, machineID string, useDefaults bool) []joinAgentSelection {
	if len(options) == 0 {
		return nil
	}
	if !isTTY(os.Stdin) || useDefaults {
		return defaultJoinSelections(options)
	}

	fmt.Println("")
	fmt.Printf("Which agents do you want to run on \"%s\"?\n\n", machineID)
	for i, option := range options {
		capabilities := formatCapabilities(option.Capabilities)
		lastActive := formatLastActive(option)
		label := option.AgentID
		if option.DisplayName != "" && option.DisplayName != option.AgentID {
			label = fmt.Sprintf("%s (%s)", option.AgentID, option.DisplayName)
		}
		fmt.Printf("  %d. %s%s%s\n", i+1, label, lastActive, capabilities)
	}
	fmt.Print("Select [default=all]: ")

	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	trimmed := strings.TrimSpace(strings.ToLower(text))

	var selected []int
	switch trimmed {
	case "", "all":
		for i := range options {
			selected = append(selected, i)
		}
	case "none":
		return nil
	default:
		parts := strings.Split(trimmed, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			var idx int
			if _, err := fmt.Sscanf(part, "%d", &idx); err == nil && idx >= 1 && idx <= len(options) {
				selected = append(selected, idx-1)
			}
		}
	}

	if len(selected) == 0 {
		return nil
	}

	selections := make([]joinAgentSelection, 0, len(selected))
	for _, idx := range selected {
		selections = append(selections, joinAgentSelection{AgentID: options[idx].AgentID, Driver: "claude"})
	}

	fmt.Println("")
	fmt.Println("Default driver: claude (also supports codex, opencode)")
	if !promptYesNo("Use defaults?", true) {
		for i := range selections {
			fmt.Printf("Driver for %s [claude/codex/opencode, default=claude]: ", selections[i].AgentID)
			driverText, _ := reader.ReadString('\n')
			driverTrimmed := strings.TrimSpace(strings.ToLower(driverText))
			if driverTrimmed == "claude" || driverTrimmed == "codex" || driverTrimmed == "opencode" {
				selections[i].Driver = driverTrimmed
			}
		}
	}

	return selections
}

func defaultJoinSelections(options []joinAgentOption) []joinAgentSelection {
	selections := make([]joinAgentSelection, 0, len(options))
	for _, option := range options {
		selections = append(selections, joinAgentSelection{AgentID: option.AgentID, Driver: "claude"})
	}
	return selections
}

func formatLastActive(option joinAgentOption) string {
	if option.LastActiveAt <= 0 {
		return ""
	}
	label := formatRelative(option.LastActiveAt)
	if option.LastActiveOrigin != "" {
		label = fmt.Sprintf("%s, %s", option.LastActiveOrigin, label)
	}
	return fmt.Sprintf(" (last active: %s)", label)
}

func formatCapabilities(capabilities []string) string {
	if len(capabilities) == 0 {
		return ""
	}
	return fmt.Sprintf(" [%s]", strings.Join(capabilities, ", "))
}

func registerJoinAgents(projectRoot string, selections []joinAgentSelection) ([]string, error) {
	if len(selections) == 0 {
		return nil, nil
	}
	existingAgents, err := db.ReadAgents(projectRoot)
	if err != nil {
		return nil, err
	}
	existing := map[string]bool{}
	for _, agent := range existingAgents {
		if agent.AgentID == "" {
			continue
		}
		existing[agent.AgentID] = true
	}

	var created []string
	now := time.Now().Unix()
	for _, selection := range selections {
		if selection.AgentID == "" || existing[selection.AgentID] {
			continue
		}
		guid, err := core.GenerateGUID("usr")
		if err != nil {
			return nil, err
		}
		agent := types.Agent{
			GUID:         guid,
			AgentID:      selection.AgentID,
			RegisteredAt: now,
			LastSeen:     now,
			Managed:      true,
			Presence:     types.PresenceOffline,
			Invoke: &types.InvokeConfig{
				Driver: selection.Driver,
			},
		}
		if err := db.AppendAgent(projectRoot, agent); err != nil {
			return nil, err
		}
		created = append(created, selection.AgentID)
	}
	return created, nil
}
