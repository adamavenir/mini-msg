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

	"github.com/adamavenir/fray/internal/command/hooks"
	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
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
				suggestion, err := suggestAgentDelimiter(ctx.DB, agentID)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if suggestion != "" && !ctx.Force {
					return writeCommandError(cmd, fmt.Errorf("did you mean @%s? Re-run with --force to use @%s", suggestion, agentID))
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
					return writeCommandError(cmd, fmt.Errorf("agent @%s is currently active.\n\nOptions:\n  - Use a different name: fray new @other-name\n  - Generate a random name: fray new\n  - If this is you rejoining: fray back @%s", agentID, agentID))
				}

				isRejoin = true
				// Clear ghost cursor session acks so cursors become "unread" for new session
				if err := db.ClearGhostCursorSessionAcks(ctx.DB, agentID); err != nil {
					return writeCommandError(cmd, err)
				}
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

				// Collect used avatars from existing agents
				usedAvatars := make(map[string]struct{})
				existingAgents, _ := db.GetAgents(ctx.DB)
				for _, a := range existingAgents {
					if a.Avatar != nil && *a.Avatar != "" {
						usedAvatars[*a.Avatar] = struct{}{}
					}
				}

				// Assign avatar based on agent name
				avatar := core.AssignAvatar(agentID, usedAvatars)

				agent := types.Agent{
					GUID:         agentGUID,
					AgentID:      agentID,
					Status:       optionalString(statusOpt),
					Purpose:      optionalString(purposeOpt),
					Avatar:       &avatar,
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

			// Create agent thread hierarchy for new agents
			if !isRejoin {
				if err := ensureAgentHierarchy(ctx, agentID); err != nil {
					return writeCommandError(cmd, err)
				}
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

			// Post event message for join/rejoin
			eventBody := fmt.Sprintf("@%s joined", agentID)
			if isRejoin {
				eventBody = fmt.Sprintf("@%s rejoined", agentID)
			}
			eventMsg, err := db.CreateMessage(ctx.DB, types.Message{
				TS:        now,
				FromAgent: agentID,
				Body:      eventBody,
				Type:      types.MessageTypeEvent,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if err := db.AppendMessage(ctx.Project.DBPath, eventMsg); err != nil {
				return writeCommandError(cmd, err)
			}

			// Create default DM thread between agent and user (if username configured)
			var dmThread *types.Thread
			if !isRejoin {
				username, _ := db.GetConfig(ctx.DB, "username")
				if username != "" {
					threadName := fmt.Sprintf("dm-%s", agentID)
					subscribers := []string{agentID, username}
					dmThread, err = ensureThread(ctx, threadName, nil, subscribers)
					if err != nil {
						return writeCommandError(cmd, err)
					}
				}
			}

			// Post optional user message
			var posted *types.Message
			if message != "" {
				bases, err := db.GetAgentBases(ctx.DB)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				mentions := core.ExtractMentions(message, bases)
				mentions = core.ExpandAllMention(mentions, bases)
				userMsg, err := db.CreateMessage(ctx.DB, types.Message{
					TS:        now,
					FromAgent: agentID,
					Body:      message,
					Mentions:  mentions,
				})
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if err := db.AppendMessage(ctx.Project.DBPath, userMsg); err != nil {
					return writeCommandError(cmd, err)
				}
				posted = &userMsg
			}

			wroteEnv := hooks.WriteClaudeEnv(agentID)

			if ctx.JSONMode {
				payload := map[string]any{
					"agent_id":   agentID,
					"rejoin":     isRejoin,
					"event_id":   eventMsg.ID,
					"claude_env": wroteEnv,
				}
				if posted != nil {
					payload["message_id"] = posted.ID
				}
				if dmThread != nil {
					payload["dm_thread"] = dmThread.GUID
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
			if posted != nil {
				fmt.Fprintf(out, "  Posted: [%s] %s\n", posted.ID, message)
			}
			if dmThread != nil {
				fmt.Fprintf(out, "  DM thread: %s (%s)\n", dmThread.Name, dmThread.GUID)
			}
			if wroteEnv {
				fmt.Fprintln(out, "  Registered with Claude hooks")
			} else {
				fmt.Fprintf(out, "  Post with: fray post --as %s \"message\"\n", agentID)
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

// ensureMetaThread ensures the root meta/ thread exists.
func ensureMetaThread(ctx *CommandContext) (*types.Thread, error) {
	metaThread, err := db.GetThreadByName(ctx.DB, "meta", nil)
	if err != nil {
		return nil, err
	}

	if metaThread == nil {
		thread, err := db.CreateThread(ctx.DB, types.Thread{
			Name: "meta",
			Type: types.ThreadTypeKnowledge,
		})
		if err != nil {
			return nil, err
		}
		if err := db.AppendThread(ctx.Project.DBPath, thread, []string{"meta"}); err != nil {
			return nil, err
		}
		metaThread = &thread
	}

	return metaThread, nil
}

// ensureAgentHierarchy creates the agent thread hierarchy if it doesn't exist.
// Creates: meta/{agent}/ (knowledge), meta/{agent}/notes (system), meta/{agent}/jrnl (system)
func ensureAgentHierarchy(ctx *CommandContext, agentID string) error {
	// First ensure meta/ root thread exists
	metaThread, err := ensureMetaThread(ctx)
	if err != nil {
		return err
	}

	// Check if agent thread exists under meta/
	agentThread, err := db.GetThreadByName(ctx.DB, agentID, &metaThread.GUID)
	if err != nil {
		return err
	}

	if agentThread == nil {
		// Create agent thread as child of meta/ (knowledge type)
		thread, err := db.CreateThread(ctx.DB, types.Thread{
			Name:         agentID,
			ParentThread: &metaThread.GUID,
			Type:         types.ThreadTypeKnowledge,
		})
		if err != nil {
			return err
		}
		if err := db.AppendThread(ctx.Project.DBPath, thread, []string{agentID}); err != nil {
			return err
		}
		// Subscribe agent to their own thread
		if err := db.SubscribeThread(ctx.DB, thread.GUID, agentID, time.Now().Unix()); err != nil {
			return err
		}
		agentThread = &thread
	}

	// Check and create notes subthread
	notesThread, err := db.GetThreadByName(ctx.DB, "notes", &agentThread.GUID)
	if err != nil {
		return err
	}
	if notesThread == nil {
		thread, err := db.CreateThread(ctx.DB, types.Thread{
			Name:         "notes",
			ParentThread: &agentThread.GUID,
			Type:         types.ThreadTypeSystem,
		})
		if err != nil {
			return err
		}
		if err := db.AppendThread(ctx.Project.DBPath, thread, []string{agentID}); err != nil {
			return err
		}
		if err := db.SubscribeThread(ctx.DB, thread.GUID, agentID, time.Now().Unix()); err != nil {
			return err
		}
	}

	// Check and create jrnl subthread
	jrnlThread, err := db.GetThreadByName(ctx.DB, "jrnl", &agentThread.GUID)
	if err != nil {
		return err
	}
	if jrnlThread == nil {
		thread, err := db.CreateThread(ctx.DB, types.Thread{
			Name:         "jrnl",
			ParentThread: &agentThread.GUID,
			Type:         types.ThreadTypeSystem,
		})
		if err != nil {
			return err
		}
		if err := db.AppendThread(ctx.Project.DBPath, thread, []string{agentID}); err != nil {
			return err
		}
		if err := db.SubscribeThread(ctx.DB, thread.GUID, agentID, time.Now().Unix()); err != nil {
			return err
		}
	}

	return nil
}
