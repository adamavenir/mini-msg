package command

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewMigrateCmd creates the migrate command.
func NewMigrateCmd() *cobra.Command {
	var fixThreads bool
	var multiMachine bool

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate fray project from v0.1.0 to v0.2.0 format, or fix thread hierarchy",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := core.DiscoverProject("")
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if multiMachine {
				return migrateMultiMachine(cmd, &project)
			}

			frayDir := filepath.Dir(project.DBPath)
			configPath := filepath.Join(frayDir, "fray-config.json")

			if _, err := os.Stat(configPath); err == nil {
				if !fixThreads {
					fmt.Fprintln(cmd.OutOrStdout(), "Project already migrated to v0.2.0.")
					fmt.Fprintln(cmd.OutOrStdout(), "Checking for legacy thread patterns...")
					fixThreads = true
				}
				if fixThreads {
					return migrateThreadHierarchy(cmd, &project)
				}
				return nil
			}

			backupDir := filepath.Join(project.Root, ".fray.bak")
			if _, err := os.Stat(backupDir); err == nil {
				return writeCommandError(cmd, fmt.Errorf("Backup already exists at .fray.bak/. Move it aside before migrating."))
			}

			defaultName := filepath.Base(project.Root)
			channelName, err := promptMigrateChannelName(defaultName)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			channelID, err := core.GenerateGUID("ch")
			if err != nil {
				return writeCommandError(cmd, err)
			}

			sourceDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", project.DBPath))
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer sourceDB.Close()

			if !tableExists(sourceDB, "fray_agents") || !tableExists(sourceDB, "fray_messages") {
				return writeCommandError(cmd, fmt.Errorf("Missing fray tables in database. Nothing to migrate."))
			}

			agentColumns, err := getColumns(sourceDB, "fray_agents")
			if err != nil {
				return writeCommandError(cmd, err)
			}
			hasAgentGUID := columnsInclude(agentColumns, "guid")
			agents, err := loadAgents(sourceDB, hasAgentGUID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			messageColumns, err := getColumns(sourceDB, "fray_messages")
			if err != nil {
				return writeCommandError(cmd, err)
			}
			hasMessageGUID := columnsInclude(messageColumns, "guid")
			hasMessageID := columnsInclude(messageColumns, "id")
			messages, err := loadMessages(sourceDB, hasMessageGUID, hasMessageID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			readReceipts, err := loadReadReceipts(sourceDB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			usedAgentGuids := map[string]struct{}{}
			knownAgents := map[string]db.ProjectKnownAgent{}
			agentsJSONL := make([]db.AgentJSONLRecord, 0, len(agents))

			for _, agent := range agents {
				guid := agent.GUID
				if guid == nil || *guid == "" || containsGUID(usedAgentGuids, *guid) {
					generated, err := generateUniqueGUID("usr", usedAgentGuids)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					guid = &generated
				} else {
					usedAgentGuids[*guid] = struct{}{}
				}

				createdAt := time.Unix(agent.RegisteredAt, 0).UTC().Format(time.RFC3339)
				status := "active"
				if agent.LeftAt != nil {
					status = "inactive"
				}
				globalName := agent.AgentID
				if channelName != "" {
					globalName = fmt.Sprintf("%s-%s", channelName, agent.AgentID)
				}

				knownAgents[*guid] = db.ProjectKnownAgent{
					Name:        &agent.AgentID,
					GlobalName:  &globalName,
					HomeChannel: &channelID,
					CreatedAt:   &createdAt,
					Status:      &status,
				}

				agentsJSONL = append(agentsJSONL, db.AgentJSONLRecord{
					Type:         "agent",
					ID:           *guid,
					Name:         agent.AgentID,
					GlobalName:   &globalName,
					HomeChannel:  &channelID,
					CreatedAt:    &createdAt,
					ActiveStatus: nil,
					AgentID:      agent.AgentID,
					Status:       &status,
					Purpose:      nil,
					Goal:         agent.Goal,
					Bio:          agent.Bio,
					RegisteredAt: agent.RegisteredAt,
					LastSeen:     agent.LastSeen,
					LeftAt:       agent.LeftAt,
				})
			}

			if !hasMessageGUID && !hasMessageID {
				return writeCommandError(cmd, fmt.Errorf("Could not locate message IDs for migration."))
			}

			usedMessageGuids := map[string]struct{}{}
			idToGuid := map[int64]string{}
			messageGuids := make([]string, len(messages))

			for i, message := range messages {
				guid := message.GUID
				if guid == nil || *guid == "" || containsGUID(usedMessageGuids, *guid) {
					generated, err := generateUniqueGUID("msg", usedMessageGuids)
					if err != nil {
						return writeCommandError(cmd, err)
					}
					messageGuids[i] = generated
				} else {
					messageGuids[i] = *guid
					usedMessageGuids[*guid] = struct{}{}
				}
				if message.ID != nil {
					idToGuid[*message.ID] = messageGuids[i]
				}
			}

			messagesJSONL := make([]db.MessageJSONLRecord, 0, len(messages))
			for i, message := range messages {
				replyTo := resolveReplyTo(message.ReplyTo, hasMessageGUID, idToGuid)
				messageType := types.MessageTypeAgent
				if message.Type != nil && *message.Type == string(types.MessageTypeUser) {
					messageType = types.MessageTypeUser
				}

				messagesJSONL = append(messagesJSONL, db.MessageJSONLRecord{
					Type:       "message",
					ID:         messageGuids[i],
					ChannelID:  &channelID,
					FromAgent:  message.FromAgent,
					Body:       message.Body,
					Mentions:   parseMentions(message.Mentions),
					MsgType:    messageType,
					ReplyTo:    replyTo,
					TS:         message.TS,
					EditedAt:   message.EditedAt,
					ArchivedAt: message.ArchivedAt,
				})
			}

			if err := copyDir(frayDir, backupDir); err != nil {
				return writeCommandError(cmd, err)
			}

			_, err = db.UpdateProjectConfig(project.DBPath, db.ProjectConfig{
				Version:     1,
				ChannelID:   channelID,
				ChannelName: channelName,
				CreatedAt:   time.Now().UTC().Format(time.RFC3339),
				KnownAgents: knownAgents,
			})
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if err := writeJSONLFile(filepath.Join(frayDir, "agents.jsonl"), agentsJSONL); err != nil {
				return writeCommandError(cmd, err)
			}
			if err := writeJSONLFile(filepath.Join(frayDir, "messages.jsonl"), messagesJSONL); err != nil {
				return writeCommandError(cmd, err)
			}

			targetDB, err := sql.Open("sqlite", project.DBPath)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			if _, err := targetDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
				_ = targetDB.Close()
				return writeCommandError(cmd, err)
			}
			if _, err := targetDB.Exec("PRAGMA journal_mode = WAL"); err != nil {
				_ = targetDB.Close()
				return writeCommandError(cmd, err)
			}
			if _, err := targetDB.Exec("PRAGMA busy_timeout = 5000"); err != nil {
				_ = targetDB.Close()
				return writeCommandError(cmd, err)
			}

			if err := db.RebuildDatabaseFromJSONL(targetDB, project.DBPath); err != nil {
				_ = targetDB.Close()
				return writeCommandError(cmd, err)
			}

			if err := restoreReadReceipts(targetDB, readReceipts, idToGuid); err != nil {
				_ = targetDB.Close()
				return writeCommandError(cmd, err)
			}
			_ = targetDB.Close()

			if _, err := core.RegisterChannel(channelID, channelName, project.Root); err != nil {
				return writeCommandError(cmd, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✓ Registered channel %s as '%s'\n", channelID, channelName)
			fmt.Fprintln(cmd.OutOrStdout(), "Migration complete. Backup at .fray.bak/")
			fmt.Fprintf(cmd.OutOrStdout(), "Migrated %d agents and %d messages.\n", len(agentsJSONL), len(messagesJSONL))

			fmt.Fprintln(cmd.OutOrStdout(), "\nChecking for legacy thread patterns...")
			return migrateThreadHierarchy(cmd, &project)
		},
	}

	cmd.Flags().BoolVar(&fixThreads, "fix-threads", false, "Fix legacy thread naming patterns only")
	cmd.Flags().BoolVar(&multiMachine, "multi-machine", false, "Migrate to multi-machine storage layout")

	return cmd
}

func promptMigrateChannelName(defaultName string) (string, error) {
	if !isTTY(os.Stdin) {
		return defaultName, nil
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stdout, "Channel name for this project? [%s]: ", defaultName)
	text, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return defaultName, nil
	}
	return trimmed, nil
}

func migrateThreadHierarchy(cmd *cobra.Command, project *core.Project) error {
	dbConn, err := sql.Open("sqlite", project.DBPath)
	if err != nil {
		return writeCommandError(cmd, err)
	}
	defer dbConn.Close()

	if _, err := dbConn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return writeCommandError(cmd, err)
	}
	if _, err := dbConn.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return writeCommandError(cmd, err)
	}
	if _, err := dbConn.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		return writeCommandError(cmd, err)
	}

	var fixes []string

	// Ensure meta/ root thread exists
	metaThread, err := db.GetThreadByName(dbConn, "meta", nil)
	if err != nil {
		return writeCommandError(cmd, fmt.Errorf("failed to check meta thread: %w", err))
	}
	var metaGUID string
	if metaThread == nil {
		newMeta := types.Thread{
			Name:   "meta",
			Type:   types.ThreadTypeKnowledge,
			Status: types.ThreadStatusOpen,
		}
		created, err := db.CreateThread(dbConn, newMeta)
		if err != nil {
			return writeCommandError(cmd, fmt.Errorf("failed to create meta thread: %w", err))
		}
		metaGUID = created.GUID
		if err := db.AppendThread(project.Root, created, nil); err != nil {
			return writeCommandError(cmd, fmt.Errorf("failed to append meta thread event: %w", err))
		}
		fixes = append(fixes, "  Created meta/ thread")
	} else {
		metaGUID = metaThread.GUID
	}

	agents, err := db.GetAllAgents(dbConn)
	if err != nil {
		return writeCommandError(cmd, fmt.Errorf("failed to get agents: %w", err))
	}

	// Migrate top-level agent threads to meta/{agent}/
	for _, agent := range agents {
		// Check for legacy top-level agent thread
		agentThread, err := db.GetThreadByName(dbConn, agent.AgentID, nil)
		if err != nil {
			continue
		}
		if agentThread != nil && (agentThread.ParentThread == nil || *agentThread.ParentThread == "") {
			// Move to meta/
			updates := db.ThreadUpdates{
				ParentThread: types.OptionalString{Set: true, Value: &metaGUID},
			}
			updated, err := db.UpdateThread(dbConn, agentThread.GUID, updates)
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("failed to move agent thread %s: %w", agent.AgentID, err))
			}
			if err := db.AppendThreadUpdate(project.Root, db.ThreadUpdateJSONLRecord{
				GUID:         updated.GUID,
				ParentThread: updated.ParentThread,
			}); err != nil {
				return writeCommandError(cmd, fmt.Errorf("failed to append thread update: %w", err))
			}
			fixes = append(fixes, fmt.Sprintf("  Migrated: %s/ -> meta/%s/", agent.AgentID, agent.AgentID))
		}

		// Also handle legacy {agent}-{suffix} format from older versions
		suffixes := []string{"notes", "meta", "jrnl"}
		for _, suffix := range suffixes {
			legacyName := fmt.Sprintf("%s-%s", agent.AgentID, suffix)
			thread, err := db.GetThreadByName(dbConn, legacyName, nil)
			if err != nil || thread == nil {
				continue
			}

			// Find or create the agent parent under meta/
			parentThread, err := db.GetThreadByName(dbConn, agent.AgentID, &metaGUID)
			var parentGUID string
			if err != nil || parentThread == nil {
				newParent := types.Thread{
					Name:         agent.AgentID,
					ParentThread: &metaGUID,
					Type:         types.ThreadTypeKnowledge,
					Status:       types.ThreadStatusOpen,
				}
				createdParent, err := db.CreateThread(dbConn, newParent)
				if err != nil {
					return writeCommandError(cmd, fmt.Errorf("failed to create agent thread %s: %w", agent.AgentID, err))
				}
				parentGUID = createdParent.GUID
				if err := db.AppendThread(project.Root, createdParent, nil); err != nil {
					return writeCommandError(cmd, fmt.Errorf("failed to append agent thread event: %w", err))
				}
				fixes = append(fixes, fmt.Sprintf("  Created: meta/%s/", agent.AgentID))
			} else {
				parentGUID = parentThread.GUID
			}

			// Rename and reparent the suffix thread
			updates := db.ThreadUpdates{
				Name:         types.OptionalString{Set: true, Value: &suffix},
				ParentThread: types.OptionalString{Set: true, Value: &parentGUID},
			}
			updated, err := db.UpdateThread(dbConn, thread.GUID, updates)
			if err != nil {
				return writeCommandError(cmd, fmt.Errorf("failed to update thread %s: %w", legacyName, err))
			}
			updatedName := updated.Name
			if err := db.AppendThreadUpdate(project.Root, db.ThreadUpdateJSONLRecord{
				GUID:         updated.GUID,
				Name:         &updatedName,
				ParentThread: updated.ParentThread,
			}); err != nil {
				return writeCommandError(cmd, fmt.Errorf("failed to append thread update: %w", err))
			}
			fixes = append(fixes, fmt.Sprintf("  Migrated: %s -> meta/%s/%s", legacyName, agent.AgentID, suffix))
		}
	}

	// Migrate roles/{role}/ to meta/role-{role}/
	allThreads, err := db.GetThreads(dbConn, nil)
	if err != nil {
		return writeCommandError(cmd, fmt.Errorf("failed to get threads: %w", err))
	}
	for _, thread := range allThreads {
		if !strings.HasPrefix(thread.Name, "roles/") {
			continue
		}
		if thread.ParentThread != nil && *thread.ParentThread != "" {
			continue // Already nested, skip
		}

		roleName := strings.TrimPrefix(thread.Name, "roles/")
		newName := fmt.Sprintf("role-%s", roleName)

		updates := db.ThreadUpdates{
			Name:         types.OptionalString{Set: true, Value: &newName},
			ParentThread: types.OptionalString{Set: true, Value: &metaGUID},
		}
		updated, err := db.UpdateThread(dbConn, thread.GUID, updates)
		if err != nil {
			return writeCommandError(cmd, fmt.Errorf("failed to update role thread %s: %w", thread.Name, err))
		}
		if err := db.AppendThreadUpdate(project.Root, db.ThreadUpdateJSONLRecord{
			GUID:         updated.GUID,
			Name:         &newName,
			ParentThread: updated.ParentThread,
		}); err != nil {
			return writeCommandError(cmd, fmt.Errorf("failed to append thread update: %w", err))
		}
		fixes = append(fixes, fmt.Sprintf("  Migrated: %s -> meta/%s", thread.Name, newName))
	}

	if len(fixes) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "✓ No legacy thread patterns found")
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout(), "✓ Fixed legacy thread patterns:")
	for _, fix := range fixes {
		fmt.Fprintln(cmd.OutOrStdout(), fix)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d threads migrated\n", len(fixes))

	return nil
}
