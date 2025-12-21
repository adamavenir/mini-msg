package command

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

var (
	adjectives = []string{"eager", "cosmic", "brave", "quiet", "swift", "curious", "bright", "gentle", "bold", "merry"}
	animals    = []string{"beaver", "dolphin", "fox", "otter", "owl", "panda", "falcon", "tiger", "wolf", "sparrow"}
)

// NewNewCmd creates the new command.
func NewNewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new [name] [message]",
		Short: "Create new agent session",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			statusOpt, _ := cmd.Flags().GetString("status")
			purposeOpt, _ := cmd.Flags().GetString("purpose")

			var name string
			var message string
			if len(args) > 0 {
				name = args[0]
			}
			if len(args) > 1 {
				message = args[1]
			}

			staleHours := 4
			if value, err := db.GetConfig(ctx.DB, "stale_hours"); err == nil && value != "" {
				parsed := parseNumeric(value)
				if parsed > 0 {
					staleHours = parsed
				}
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

			var agentID string
			isRejoin := false

			if name == "" {
				agentID, err = generateUniqueName(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
			} else {
				agentID = core.NormalizeAgentRef(name)
				if !core.IsValidAgentID(agentID) {
					return writeCommandError(cmd, fmt.Errorf("invalid agent name: %s\nNames must start with a lowercase letter and contain only lowercase letters, numbers, and hyphens.\nExamples: alice, pm, eager-beaver, frontend-dev", agentID))
				}
			}

			existing, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if existing != nil {
				active, err := db.IsAgentActive(ctx.DB, agentID, staleHours)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if active {
					return writeCommandError(cmd, fmt.Errorf("agent @%s is currently active.\n\nOptions:\n  - Use a different name: mm new @other-name\n  - Generate a random name: mm new\n  - If this is you rejoining: mm back @%s", agentID, agentID))
				}

				isRejoin = true
				now := time.Now().Unix()
				updates := db.AgentUpdates{
					LastSeen: types.OptionalInt64{Set: true, Value: &now},
					LeftAt:   types.OptionalInt64{Set: true, Value: nil},
				}
				if statusOpt != "" {
					updates.Status = types.OptionalString{Set: true, Value: &statusOpt}
				} else {
					updates.Status = types.OptionalString{Set: true, Value: existing.Status}
				}
				if purposeOpt != "" {
					updates.Purpose = types.OptionalString{Set: true, Value: &purposeOpt}
				} else {
					updates.Purpose = types.OptionalString{Set: true, Value: existing.Purpose}
				}
				if err := db.UpdateAgent(ctx.DB, agentID, updates); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			now := time.Now().Unix()
			if !isRejoin {
				var agentGUID string
				knownMatch := findKnownAgentByName(projectConfig, agentID)
				if knownMatch != nil {
					reuse := promptReuseKnownAgent(agentID, knownMatch.GUID)
					if reuse {
						agentGUID = knownMatch.GUID
					}
				}

				if agentGUID == "" {
					known := map[string]struct{}{}
					if projectConfig != nil {
						for guid := range projectConfig.KnownAgents {
							known[guid] = struct{}{}
						}
					}
					for {
						generated, err := core.GenerateGUID("usr")
						if err != nil {
							return writeCommandError(cmd, err)
						}
						if _, exists := known[generated]; !exists {
							agentGUID = generated
							break
						}
					}
				}

				agent := types.Agent{
					GUID:         agentGUID,
					AgentID:      agentID,
					Status:       optionalString(statusOpt),
					Purpose:      optionalString(purposeOpt),
					RegisteredAt: now,
					LastSeen:     now,
					LeftAt:       nil,
				}
				if err := db.CreateAgent(ctx.DB, agent); err != nil {
					return writeCommandError(cmd, err)
				}
			}

			agentRecord, err := db.GetAgent(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if agentRecord == nil {
				return writeCommandError(cmd, fmt.Errorf("failed to load agent after creation: %s", agentID))
			}

			if err := db.AppendAgent(ctx.Project.DBPath, *agentRecord); err != nil {
				return writeCommandError(cmd, err)
			}

			var existingKnown *db.ProjectKnownAgent
			if projectConfig != nil {
				if entry, ok := projectConfig.KnownAgents[agentRecord.GUID]; ok {
					existingKnown = &entry
				}
			}
			createdAt := time.Unix(agentRecord.RegisteredAt, 0).UTC().Format(time.RFC3339)
			if existingKnown != nil {
				if existingKnown.CreatedAt != nil {
					createdAt = *existingKnown.CreatedAt
				} else if existingKnown.FirstSeen != nil {
					createdAt = *existingKnown.FirstSeen
				}
			}

			globalName := agentRecord.AgentID
			if channelName != "" {
				globalName = fmt.Sprintf("%s-%s", channelName, agentRecord.AgentID)
			}

			status := "active"
			if agentRecord.LeftAt != nil {
				status = "inactive"
			}

			update := db.ProjectConfig{
				KnownAgents: map[string]db.ProjectKnownAgent{
					agentRecord.GUID: {
						Name:        &agentRecord.AgentID,
						GlobalName:  &globalName,
						HomeChannel: optionalString(channelID),
						CreatedAt:   &createdAt,
						Status:      &status,
					},
				},
			}
			if _, err := db.UpdateProjectConfig(ctx.Project.DBPath, update); err != nil {
				return writeCommandError(cmd, err)
			}

			joinMessage := message
			if joinMessage == "" {
				if isRejoin {
					joinMessage = "rejoined"
				} else {
					joinMessage = "joined"
				}
			}

			bases, err := db.GetAgentBases(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			mentions := core.ExtractMentions(joinMessage, bases)
			posted, err := db.CreateMessage(ctx.DB, types.Message{
				TS:        now,
				FromAgent: agentID,
				Body:      joinMessage,
				Mentions:  mentions,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			wroteEnv := WriteClaudeEnv(agentID)

			if ctx.JSONMode {
				payload := map[string]any{
					"agent_id":   agentID,
					"rejoin":     isRejoin,
					"message_id": posted.ID,
					"claude_env": wroteEnv,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			if isRejoin {
				fmt.Fprintf(out, "Rejoined as @%s\n", agentID)
			} else {
				fmt.Fprintf(out, "Joined as @%s\n", agentID)
			}
			if statusOpt != "" {
				fmt.Fprintf(out, "  Status: %s\n", statusOpt)
			}
			if purposeOpt != "" {
				fmt.Fprintf(out, "  Purpose: %s\n", purposeOpt)
			}
			fmt.Fprintf(out, "  Posted: [%s] %s\n", posted.ID, joinMessage)
			if wroteEnv {
				fmt.Fprintln(out, "  Registered with Claude hooks")
			} else {
				fmt.Fprintf(out, "  Post with: mm post --as %s \"message\"\n", agentID)
			}

			return nil
		},
	}

	cmd.Flags().String("status", "", "current task/focus")
	cmd.Flags().String("purpose", "", "agent role/identity")

	return cmd
}

type knownAgentMatch struct {
	GUID  string
	Entry db.ProjectKnownAgent
}

func findKnownAgentByName(config *db.ProjectConfig, name string) *knownAgentMatch {
	if config == nil {
		return nil
	}
	for guid, entry := range config.KnownAgents {
		if entry.Name != nil && *entry.Name == name {
			return &knownAgentMatch{GUID: guid, Entry: entry}
		}
	}
	return nil
}

func promptReuseKnownAgent(name, guid string) bool {
	if !isTTY(os.Stdin) {
		return true
	}
	fmt.Fprintf(os.Stdout, "Use existing @%s (%s)? [Y/n] ", name, guid)
	var response string
	fmt.Fscanln(os.Stdin, &response)
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "" || response == "y" || response == "yes"
}

func generateUniqueName(dbConn *sql.DB) (string, error) {
	for i := 0; i < 10; i++ {
		name, err := generateRandomName()
		if err != nil {
			return "", err
		}
		existing, err := db.GetAgent(dbConn, name)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return name, nil
		}
	}
	name, err := generateRandomName()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%d", name, time.Now().Unix()%10000), nil
}

func generateRandomName() (string, error) {
	adjIdx, err := randIndex(len(adjectives))
	if err != nil {
		return "", err
	}
	animalIdx, err := randIndex(len(animals))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", adjectives[adjIdx], animals[animalIdx]), nil
}

func randIndex(max int) (int, error) {
	if max <= 0 {
		return 0, fmt.Errorf("invalid random range")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func parseNumeric(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	num := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0
		}
		num = num*10 + int(r-'0')
	}
	return num
}
