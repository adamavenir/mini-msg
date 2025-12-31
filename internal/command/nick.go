package command

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/spf13/cobra"
)

// NewNickCmd creates the nick command.
func NewNickCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nick <agent>",
		Short: "Add a nickname for an agent in this channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			nick, _ := cmd.Flags().GetString("as")
			nick = core.NormalizeAgentRef(nick)
			if nick == "" {
				return writeCommandError(cmd, fmt.Errorf("--as is required"))
			}
			if !core.IsValidAgentID(nick) {
				return writeCommandError(cmd, fmt.Errorf("invalid nickname: %s", nick))
			}

			found := findKnownAgent(ctx.ProjectConfig, args[0])
			if found == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found in known_agents: %s", args[0]))
			}

			existing := found.Entry.Nicks
			next := existing
			if !containsString(existing, nick) {
				next = append(append([]string{}, existing...), nick)
			}

			update := db.ProjectConfig{
				KnownAgents: map[string]db.ProjectKnownAgent{
					found.GUID: {Nicks: next},
				},
			}
			if _, err := db.UpdateProjectConfig(ctx.Project.DBPath, update); err != nil {
				return writeCommandError(cmd, err)
			}

			if ctx.JSONMode {
				payload := map[string]any{
					"agent_id": found.GUID,
					"name":     derefName(found.Entry.Name, ""),
					"nicks":    next,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			name := derefName(found.Entry.Name, found.GUID)
			fmt.Fprintf(cmd.OutOrStdout(), "Added nickname @%s for @%s\n", nick, name)
			return nil
		},
	}

	cmd.Flags().String("as", "", "nickname to add")
	_ = cmd.MarkFlagRequired("as")
	return cmd
}

// NewNicksCmd creates the nicks command.
func NewNicksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nicks <agent>",
		Short: "Show nicknames for an agent in this channel",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			found := findKnownAgent(ctx.ProjectConfig, args[0])
			if found == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found in known_agents: %s", args[0]))
			}

			nicks := found.Entry.Nicks
			payload := map[string]any{
				"agent_id":    found.GUID,
				"name":        derefName(found.Entry.Name, ""),
				"global_name": derefName(found.Entry.GlobalName, ""),
				"nicks":       nicks,
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			name := derefName(found.Entry.Name, found.GUID)
			fmt.Fprintf(cmd.OutOrStdout(), "@%s (%s)\n", name, found.GUID)
			if len(nicks) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "  (no nicknames)")
				return nil
			}
			for _, nick := range nicks {
				fmt.Fprintf(cmd.OutOrStdout(), "  @%s\n", nick)
			}
			return nil
		},
	}

	return cmd
}

// NewWhoamiCmd creates the whoami command.
func NewWhoamiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show your known names and nicknames",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			envAgent := os.Getenv("FRAY_AGENT_ID")
			if envAgent == "" {
				return writeCommandError(cmd, fmt.Errorf("FRAY_AGENT_ID not set. Run `fray new` or `fray back` first."))
			}

			found := findKnownAgent(ctx.ProjectConfig, envAgent)
			if found == nil {
				return writeCommandError(cmd, fmt.Errorf("agent not found in known_agents: %s", envAgent))
			}

			nicks := found.Entry.Nicks
			payload := map[string]any{
				"agent_id":    found.GUID,
				"name":        derefName(found.Entry.Name, ""),
				"global_name": derefName(found.Entry.GlobalName, ""),
				"nicks":       nicks,
			}

			if ctx.JSONMode {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			name := derefName(found.Entry.Name, found.GUID)
			fmt.Fprintf(cmd.OutOrStdout(), "You are @%s (%s)\n", name, found.GUID)
			globalName := derefName(found.Entry.GlobalName, "")
			if globalName != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  global: @%s\n", globalName)
			}
			if len(nicks) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "  nicknames: (none)")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "  nicknames:")
			for _, nick := range nicks {
				fmt.Fprintf(cmd.OutOrStdout(), "    @%s\n", nick)
			}
			return nil
		},
	}

	return cmd
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
