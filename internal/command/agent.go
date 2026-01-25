package command

import "github.com/spf13/cobra"

// NewAgentCmd creates the parent agent command.
func NewAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage daemon-controlled agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		NewAgentAddCmd(),
		NewAgentCreateCmd(),
		NewAgentRemoveCmd(),
		NewAgentUpdateCmd(),
		NewAgentStartCmd(),
		NewAgentRefreshCmd(),
		NewAgentEndCmd(),
		NewAgentListCmd(),
		NewAgentStatusCmd(),
		NewAgentCheckCmd(),
		NewAgentAvatarCmd(),
		NewAgentResolveCmd(),
		NewAgentIdentityCmd(),
		NewAgentKeygenCmd(),
	)

	return cmd
}
