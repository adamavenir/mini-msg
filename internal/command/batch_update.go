package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

type batchUpdatePayload struct {
	Agents []batchUpdateAgent `json:"agents"`
}

type batchUpdateAgent struct {
	AgentID string    `json:"agent_id"`
	Status  *string   `json:"status,omitempty"`
	Purpose *string   `json:"purpose,omitempty"`
	Nicks   *[]string `json:"nicks,omitempty"`
}

type batchUpdateResult struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
}

// NewBatchUpdateCmd creates the batch-update command.
func NewBatchUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "batch-update",
		Short:   "Batch register or update agents using JSON input",
		Example: "  fray batch-update --file agents.json\n  echo '{\"agents\":[{\"agent_id\":\"devrel\",\"status\":\"working on docs\",\"purpose\":\"developer relations\",\"nicks\":[\"dr\",\"devrel-alias\"]}]}' | fray batch-update",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			payload, err := readBatchUpdatePayload(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if len(payload.Agents) == 0 {
				return writeCommandError(cmd, fmt.Errorf("no agents provided"))
			}

			projectConfig := ctx.ProjectConfig
			channelName := ""
			channelID := ""
			if projectConfig != nil {
				channelName = projectConfig.ChannelName
				channelID = projectConfig.ChannelID
			}
			if channelName == "" {
				channelName = GetProjectName(ctx.Project.Root)
			}

			result := batchUpdateResult{}
			for _, update := range payload.Agents {
				if strings.TrimSpace(update.AgentID) == "" {
					return writeCommandError(cmd, fmt.Errorf("agent_id is required"))
				}
				agentID := core.NormalizeAgentRef(update.AgentID)
				if !core.IsValidAgentID(agentID) {
					return writeCommandError(cmd, fmt.Errorf("invalid agent_id: %s", agentID))
				}

				existing, err := db.GetAgent(ctx.DB, agentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}

				if existing == nil {
					now := time.Now().Unix()
					if err := db.CreateAgent(ctx.DB, types.Agent{
						AgentID:      agentID,
						Status:       update.Status,
						Purpose:      update.Purpose,
						RegisteredAt: now,
						LastSeen:     now,
					}); err != nil {
						return writeCommandError(cmd, err)
					}

					createdAgent, err := db.GetAgent(ctx.DB, agentID)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					if createdAgent == nil {
						return writeCommandError(cmd, fmt.Errorf("failed to load created agent: %s", agentID))
					}
					if err := db.AppendAgent(ctx.Project.DBPath, *createdAgent); err != nil {
						return writeCommandError(cmd, err)
					}

					if err := updateKnownAgent(ctx.Project.DBPath, *createdAgent, update.Nicks, channelName, channelID); err != nil {
						return writeCommandError(cmd, err)
					}

					result.Created++
					continue
				}

				changed := false
				updates := db.AgentUpdates{}
				if update.Status != nil {
					if normalizeOptionalValue(existing.Status) != *update.Status {
						updates.Status = types.OptionalString{Set: true, Value: update.Status}
						changed = true
					}
				}
				if update.Purpose != nil {
					if normalizeOptionalValue(existing.Purpose) != *update.Purpose {
						updates.Purpose = types.OptionalString{Set: true, Value: update.Purpose}
						changed = true
					}
				}

				nicksChanged, err := updateKnownNicks(ctx.Project.DBPath, existing.GUID, agentID, update.Nicks)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if nicksChanged {
					changed = true
				}

				if updates.Status.Set || updates.Purpose.Set {
					if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
						return writeCommandError(cmd, err)
					}
					updatedAgent, err := db.GetAgent(ctx.DB, agentID)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					if updatedAgent != nil {
						if err := db.AppendAgent(ctx.Project.DBPath, *updatedAgent); err != nil {
							return writeCommandError(cmd, err)
						}
					}
				}

				if changed {
					result.Updated++
				} else {
					result.Unchanged++
				}
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created: %d, updated: %d, unchanged: %d\n", result.Created, result.Updated, result.Unchanged)
			return nil
		},
	}

	cmd.Flags().String("file", "", "JSON file to read agents from")
	return cmd
}

func readBatchUpdatePayload(cmd *cobra.Command) (*batchUpdatePayload, error) {
	filePath, _ := cmd.Flags().GetString("file")
	var data []byte
	var err error

	if filePath != "" {
		data, err = os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
	} else {
		if isTTY(os.Stdin) {
			return nil, fmt.Errorf("no input provided; use --file or pipe JSON")
		}
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("input is empty")
	}

	var payload batchUpdatePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func updateKnownAgent(dbPath string, agent types.Agent, nicks *[]string, channelName, channelID string) error {
	projectConfig, _ := db.ReadProjectConfig(dbPath)
	existingKnown := db.ProjectKnownAgent{}
	if projectConfig != nil {
		if entry, ok := projectConfig.KnownAgents[agent.GUID]; ok {
			existingKnown = entry
		}
	}

	createdAt := time.Unix(agent.RegisteredAt, 0).UTC().Format(time.RFC3339)
	if existingKnown.CreatedAt != nil {
		createdAt = *existingKnown.CreatedAt
	} else if existingKnown.FirstSeen != nil {
		createdAt = *existingKnown.FirstSeen
	}

	globalName := agent.AgentID
	if channelName != "" {
		globalName = fmt.Sprintf("%s-%s", channelName, agent.AgentID)
	}

	status := "active"
	if agent.LeftAt != nil {
		status = "inactive"
	}

	knownUpdate := db.ProjectKnownAgent{
		Name:        &agent.AgentID,
		GlobalName:  &globalName,
		HomeChannel: optionalString(channelID),
		CreatedAt:   &createdAt,
		Status:      &status,
	}
	if nicks != nil {
		normalized, err := normalizeBatchNicks(*nicks, agent.AgentID)
		if err != nil {
			return err
		}
		knownUpdate.Nicks = normalized
	}

	_, err := db.UpdateProjectConfig(dbPath, db.ProjectConfig{
		KnownAgents: map[string]db.ProjectKnownAgent{
			agent.GUID: knownUpdate,
		},
	})
	return err
}

func updateKnownNicks(dbPath, guid, agentID string, nicks *[]string) (bool, error) {
	if nicks == nil {
		return false, nil
	}
	normalized, err := normalizeBatchNicks(*nicks, agentID)
	if err != nil {
		return false, err
	}
	projectConfig, _ := db.ReadProjectConfig(dbPath)
	existing := agentNicksForGUID(projectConfig, guid)
	if stringSliceEqual(existing, normalized) {
		return false, nil
	}
	_, err = db.UpdateProjectConfig(dbPath, db.ProjectConfig{
		KnownAgents: map[string]db.ProjectKnownAgent{
			guid: {Nicks: normalized},
		},
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func normalizeBatchNicks(nicks []string, agentID string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(nicks))
	for _, nick := range nicks {
		nick = core.NormalizeAgentRef(strings.TrimSpace(nick))
		if nick == "" || nick == agentID {
			continue
		}
		if !core.IsValidAgentID(nick) {
			return nil, fmt.Errorf("invalid nickname: %s", nick)
		}
		if _, ok := seen[nick]; ok {
			continue
		}
		seen[nick] = struct{}{}
		out = append(out, nick)
	}
	return out, nil
}

func stringSliceEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
