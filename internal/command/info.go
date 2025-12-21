package command

import (
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

type agentInfo struct {
	GUID         string  `json:"guid"`
	AgentID      string  `json:"agent_id"`
	Status       *string `json:"status"`
	Purpose      *string `json:"purpose"`
	RegisteredAt string  `json:"registered_at"`
	LastSeen     string  `json:"last_seen"`
	LeftAt       *string `json:"left_at"`
	MessageCount int64   `json:"message_count"`
	ClaimCount   int64   `json:"claim_count"`
}

type channelInfo struct {
	Initialized  bool              `json:"initialized"`
	ChannelID    string            `json:"channel_id,omitempty"`
	ChannelName  string            `json:"channel_name,omitempty"`
	Path         string            `json:"path,omitempty"`
	CreatedAt    string            `json:"created_at,omitempty"`
	LastActivity string            `json:"last_activity,omitempty"`
	MessageCount int64             `json:"message_count,omitempty"`
	AgentCount   int               `json:"agent_count,omitempty"`
	Config       map[string]string `json:"config,omitempty"`
	Agents       []agentInfo       `json:"agents,omitempty"`
}

// NewInfoCmd creates the info command.
func NewInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show channel information and roster",
		RunE: func(cmd *cobra.Command, args []string) error {
			showGlobal, _ := cmd.Flags().GetBool("global")
			jsonMode, _ := cmd.Flags().GetBool("json")

			if showGlobal {
				config, err := core.ReadGlobalConfig()
				if err != nil {
					return writeCommandError(cmd, err)
				}
				if config == nil || len(config.Channels) == 0 {
					if jsonMode {
						return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"channels": []channelInfo{}})
					}
					fmt.Fprintln(cmd.OutOrStdout(), "No channels registered")
					return nil
				}

				channels := make([]channelInfo, 0, len(config.Channels))
				for _, channel := range config.Channels {
					info := getChannelInfo(channel.Path)
					if info.Initialized {
						channels = append(channels, info)
					}
				}

				if jsonMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"channels": channels})
				}

				fmt.Fprintf(cmd.OutOrStdout(), "Channels (%d):\n\n", len(channels))
				for _, info := range channels {
					formatChannelInfo(cmd.OutOrStdout(), info, true)
					fmt.Fprintln(cmd.OutOrStdout())
				}
				return nil
			}

			project, err := core.DiscoverProject("")
			if err != nil {
				if jsonMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(channelInfo{Initialized: false})
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Not initialized")
				fmt.Fprintln(cmd.OutOrStdout(), "Run: mm init")
				return nil
			}

			info := getChannelInfo(project.Root)
			if !info.Initialized {
				if jsonMode {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(channelInfo{Initialized: false})
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Not initialized")
				fmt.Fprintln(cmd.OutOrStdout(), "Run: mm init")
				return nil
			}

			if jsonMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(info)
			}
			formatChannelInfo(cmd.OutOrStdout(), info, false)
			return nil
		},
	}

	cmd.Flags().Bool("global", false, "show info for all registered channels")
	return cmd
}

func getChannelInfo(projectRoot string) channelInfo {
	mmDir := filepath.Join(projectRoot, ".mm")
	if _, err := os.Stat(mmDir); err != nil {
		return channelInfo{Initialized: false}
	}

	dbPath := filepath.Join(mmDir, "mm.db")
	if _, err := os.Stat(dbPath); err != nil {
		return channelInfo{Initialized: false}
	}

	project := core.Project{Root: projectRoot, DBPath: dbPath}
	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		return channelInfo{Initialized: false}
	}
	defer dbConn.Close()
	if err := db.InitSchema(dbConn); err != nil {
		return channelInfo{Initialized: false}
	}

	entries, err := db.GetAllConfig(dbConn)
	if err != nil {
		return channelInfo{Initialized: false}
	}
	config := map[string]string{}
	for _, entry := range entries {
		config[entry.Key] = entry.Value
	}

	agents, err := db.GetAllAgents(dbConn)
	if err != nil {
		return channelInfo{Initialized: false}
	}
	claimCounts, err := db.GetClaimCountsByAgent(dbConn)
	if err != nil {
		return channelInfo{Initialized: false}
	}
	messageCounts, err := getMessageCounts(dbConn)
	if err != nil {
		return channelInfo{Initialized: false}
	}

	totalMessages, err := getTotalMessageCount(dbConn)
	if err != nil {
		return channelInfo{Initialized: false}
	}

	lastTS, err := getLastMessageTS(dbConn)
	if err != nil {
		return channelInfo{Initialized: false}
	}

	projectConfig, _ := db.ReadProjectConfig(dbPath)
	createdAt := ""
	if projectConfig != nil {
		createdAt = projectConfig.CreatedAt
	}

	infos := make([]agentInfo, 0, len(agents))
	for _, agent := range agents {
		infos = append(infos, toAgentInfo(agent, messageCounts, claimCounts))
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].LastSeen > infos[j].LastSeen
	})

	info := channelInfo{
		Initialized:  true,
		ChannelID:    config["channel_id"],
		ChannelName:  config["channel_name"],
		Path:         projectRoot,
		CreatedAt:    createdAt,
		MessageCount: totalMessages,
		AgentCount:   len(agents),
		Config:       config,
		Agents:       infos,
	}
	if lastTS != nil {
		info.LastActivity = time.Unix(*lastTS, 0).UTC().Format(time.RFC3339)
	}
	return info
}

func toAgentInfo(agent types.Agent, messageCounts map[string]int64, claimCounts map[string]int64) agentInfo {
	registered := time.Unix(agent.RegisteredAt, 0).UTC().Format(time.RFC3339)
	lastSeen := time.Unix(agent.LastSeen, 0).UTC().Format(time.RFC3339)
	var leftAt *string
	if agent.LeftAt != nil {
		value := time.Unix(*agent.LeftAt, 0).UTC().Format(time.RFC3339)
		leftAt = &value
	}
	return agentInfo{
		GUID:         agent.GUID,
		AgentID:      agent.AgentID,
		Status:       agent.Status,
		Purpose:      agent.Purpose,
		RegisteredAt: registered,
		LastSeen:     lastSeen,
		LeftAt:       leftAt,
		MessageCount: messageCounts[agent.AgentID],
		ClaimCount:   claimCounts[agent.AgentID],
	}
}

func formatChannelInfo(out io.Writer, info channelInfo, showPath bool) {
	if !info.Initialized {
		fmt.Fprintln(out, "Not initialized")
		fmt.Fprintln(out, "Run: mm init")
		return
	}

	fmt.Fprintf(out, "Channel: %s (%s)\n", info.ChannelName, info.ChannelID)
	if showPath && info.Path != "" {
		fmt.Fprintf(out, "  path: %s\n", info.Path)
	}
	if info.CreatedAt != "" {
		createdTs, err := time.Parse(time.RFC3339, info.CreatedAt)
		if err == nil {
			fmt.Fprintf(out, "  created: %s\n", formatRelative(createdTs.Unix()))
		}
	}
	if info.LastActivity != "" {
		lastTs, err := time.Parse(time.RFC3339, info.LastActivity)
		if err == nil {
			fmt.Fprintf(out, "  last activity: %s\n", formatRelative(lastTs.Unix()))
		}
	}
	fmt.Fprintf(out, "  messages: %d\n", info.MessageCount)
	fmt.Fprintf(out, "  agents: %d\n", info.AgentCount)

	if info.Config != nil {
		custom := map[string]string{}
		for key, value := range info.Config {
			if key == "channel_id" || key == "channel_name" {
				continue
			}
			custom[key] = value
		}
		if len(custom) > 0 {
			fmt.Fprintln(out, "  config:")
			keys := make([]string, 0, len(custom))
			for key := range custom {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Fprintf(out, "    %s: %s\n", key, custom[key])
			}
		}
	}

	if len(info.Agents) > 0 {
		fmt.Fprintf(out, "  roster (%d):\n", len(info.Agents))
		for _, agent := range info.Agents {
			status := ""
			if agent.LeftAt != nil {
				status = " (left)"
			}
			lastTs, err := time.Parse(time.RFC3339, agent.LastSeen)
			if err != nil {
				continue
			}
			fmt.Fprintf(out, "    @%s%s - %d msgs, last seen %s\n", agent.AgentID, status, agent.MessageCount, formatRelative(lastTs.Unix()))
		}
	}
}
