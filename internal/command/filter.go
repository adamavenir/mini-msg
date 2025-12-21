package command

import (
	"fmt"
	"os"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	"github.com/spf13/cobra"
)

// NewFilterCmd creates the filter command.
func NewFilterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "filter",
		Short: "Manage message filter preferences",
	}

	cmd.AddCommand(newFilterSetCmd())
	cmd.AddCommand(newFilterShowCmd())
	cmd.AddCommand(newFilterClearCmd())

	return cmd
}

func newFilterSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set filter preferences",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := os.Getenv("MM_AGENT_ID")
			if agentID == "" {
				return writeCommandError(cmd, fmt.Errorf("MM_AGENT_ID not set. Run mm new first."))
			}

			mentions, _ := cmd.Flags().GetString("mentions")
			var mentionsPtr *string
			if mentions != "" {
				mentionsPtr = &mentions
			}

			if err := db.SetFilter(ctx.DB, types.Filter{
				AgentID:         agentID,
				MentionsPattern: mentionsPtr,
			}); err != nil {
				return writeCommandError(cmd, err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Filter updated")
			return nil
		},
	}

	cmd.Flags().String("mentions", "", `Mention pattern (e.g., "claude" or "claude,pm")`)
	return cmd
}

func newFilterShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show current filter",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := os.Getenv("MM_AGENT_ID")
			if agentID == "" {
				return writeCommandError(cmd, fmt.Errorf("MM_AGENT_ID not set"))
			}

			filter, err := db.GetFilter(ctx.DB, agentID)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if filter == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "No filter set")
				return nil
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Current filter:")
			if filter.MentionsPattern != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  Mentions: %s\n", *filter.MentionsPattern)
			}
			return nil
		},
	}
}

func newFilterClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Clear all filters",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			agentID := os.Getenv("MM_AGENT_ID")
			if agentID == "" {
				return writeCommandError(cmd, fmt.Errorf("MM_AGENT_ID not set"))
			}

			if err := db.ClearFilter(ctx.DB, agentID); err != nil {
				return writeCommandError(cmd, err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Filter cleared")
			return nil
		},
	}
}
