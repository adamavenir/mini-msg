package command

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

type rosterAgent struct {
	GUID         string  `json:"guid"`
	AgentID      string  `json:"agent_id"`
	Status       *string `json:"status"`
	Purpose      *string `json:"purpose"`
	Registered   string  `json:"registered_at"`
	LastSeen     string  `json:"last_seen"`
	LeftAt       *string `json:"left_at"`
	MessageCount int64   `json:"message_count"`
	ClaimCount   int64   `json:"claim_count"`
	ChannelID    *string `json:"channel_id,omitempty"`
	ChannelName  *string `json:"channel_name,omitempty"`
}

// NewRosterCmd creates the roster command.
func NewRosterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "roster",
		Short: "List registered agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			showGlobal, _ := cmd.Flags().GetBool("global")
			jsonMode, _ := cmd.Flags().GetBool("json")

			agents := []rosterAgent{}

			if showGlobal {
				config, err := core.ReadGlobalConfig()
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if config == nil || len(config.Channels) == 0 {
					return writeRosterEmpty(cmd, jsonMode)
				}

				for channelID, channel := range config.Channels {
					project := core.Project{Root: channel.Path, DBPath: filepath.Join(channel.Path, ".mm", "mm.db")}
					if _, err := os.Stat(project.DBPath); err != nil {
						continue
					}
					dbConn, err := db.OpenDatabase(project)
					if err != nil {
						continue
					}
					if err := db.InitSchema(dbConn); err != nil {
						_ = dbConn.Close()
						continue
					}
					channelAgents, err := buildRosterAgents(dbConn, &channelID, &channel.Name)
					_ = dbConn.Close()
					if err != nil {
						continue
					}
					agents = append(agents, channelAgents...)
				}
			} else {
				ctx, err := GetContext(cmd)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				defer ctx.DB.Close()

				channelAgents, err := buildRosterAgents(ctx.DB, nil, nil)
				if err != nil {
					return writeCommandError(cmd, err)
				}
				agents = append(agents, channelAgents...)
			}

			sort.Slice(agents, func(i, j int) bool {
				return agents[i].LastSeen > agents[j].LastSeen
			})

			if jsonMode {
				payload := map[string]any{"agents": agents, "total": len(agents)}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			out := cmd.OutOrStdout()
			if len(agents) == 0 {
				fmt.Fprintln(out, "No agents registered")
				return nil
			}

			label := "REGISTERED AGENTS"
			if showGlobal {
				label = "REGISTERED AGENTS (global)"
			}
			fmt.Fprintf(out, "%s (%d):\n", label, len(agents))
			for _, agent := range agents {
				formatRosterAgent(out, agent, showGlobal)
			}
			return nil
		},
	}

	cmd.Flags().Bool("local", false, "show agents from local channel only (default)")
	cmd.Flags().Bool("global", false, "show agents from all registered channels")
	return cmd
}

func buildRosterAgents(dbConn *sql.DB, channelID, channelName *string) ([]rosterAgent, error) {
	agents, err := db.GetAllAgents(dbConn)
	if err != nil {
		return nil, err
	}
	messageCounts, err := getMessageCounts(dbConn)
	if err != nil {
		return nil, err
	}
	claimCounts, err := db.GetClaimCountsByAgent(dbConn)
	if err != nil {
		return nil, err
	}

	items := make([]rosterAgent, 0, len(agents))
	for _, agent := range agents {
		items = append(items, toRosterAgent(agent, messageCounts, claimCounts, channelID, channelName))
	}
	return items, nil
}

func toRosterAgent(agent types.Agent, messageCounts map[string]int64, claimCounts map[string]int64, channelID, channelName *string) rosterAgent {
	registered := time.Unix(agent.RegisteredAt, 0).UTC().Format(time.RFC3339)
	lastSeen := time.Unix(agent.LastSeen, 0).UTC().Format(time.RFC3339)
	var leftAt *string
	if agent.LeftAt != nil {
		value := time.Unix(*agent.LeftAt, 0).UTC().Format(time.RFC3339)
		leftAt = &value
	}

	return rosterAgent{
		GUID:         agent.GUID,
		AgentID:      agent.AgentID,
		Status:       agent.Status,
		Purpose:      agent.Purpose,
		Registered:   registered,
		LastSeen:     lastSeen,
		LeftAt:       leftAt,
		MessageCount: messageCounts[agent.AgentID],
		ClaimCount:   claimCounts[agent.AgentID],
		ChannelID:    channelID,
		ChannelName:  channelName,
	}
}

func formatRosterAgent(out io.Writer, agent rosterAgent, showChannel bool) {
	leftStatus := ""
	if agent.LeftAt != nil {
		leftStatus = " (left)"
	}
	claimInfo := ""
	if agent.ClaimCount > 0 {
		plural := "s"
		if agent.ClaimCount == 1 {
			plural = ""
		}
		claimInfo = fmt.Sprintf(" (%d claim%s)", agent.ClaimCount, plural)
	}

	fmt.Fprintf(out, "  @%s%s%s\n", agent.AgentID, leftStatus, claimInfo)
	if showChannel && agent.ChannelName != nil {
		fmt.Fprintf(out, "    channel: %s\n", *agent.ChannelName)
	}
	if agent.Status != nil && *agent.Status != "" {
		fmt.Fprintf(out, "    status: %s\n", *agent.Status)
	}
	if agent.Purpose != nil && *agent.Purpose != "" {
		fmt.Fprintf(out, "    purpose: %s\n", *agent.Purpose)
	}

	lastSeenTs, _ := time.Parse(time.RFC3339, agent.LastSeen)
	registeredTs, _ := time.Parse(time.RFC3339, agent.Registered)
	fmt.Fprintf(out, "    last seen: %s\n", formatRelative(lastSeenTs.Unix()))
	fmt.Fprintf(out, "    registered: %s\n", formatRelative(registeredTs.Unix()))
	fmt.Fprintf(out, "    messages: %d\n", agent.MessageCount)
}

func writeRosterEmpty(cmd *cobra.Command, jsonMode bool) error {
	if jsonMode {
		payload := map[string]any{"agents": []rosterAgent{}, "total": 0}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "No channels registered")
	return nil
}
