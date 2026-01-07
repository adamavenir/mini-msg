package command

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/spf13/cobra"
)

// NewQuickstartCmd creates the quickstart command.
func NewQuickstartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "quickstart",
		Aliases: []string{"qs"},
		Short:   "Guide for agents on using the messenger",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, err := GetContext(cmd)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			defer ctx.DB.Close()

			jsonMode, _ := cmd.Flags().GetBool("json")

			allAgents, err := db.GetAllAgents(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}
			activeUsers, err := db.GetActiveUsers(ctx.DB)
			if err != nil {
				return writeCommandError(cmd, err)
			}

			if jsonMode {
				payload := map[string]any{
					"registered_agents": agentIDs(allAgents),
					"registered_users":  activeUsers,
					"guide":             buildQuickstartGuide(allAgents, activeUsers),
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}

			printQuickstartGuide(cmd.OutOrStdout(), allAgents, activeUsers)
			return nil
		},
	}

	return cmd
}

func buildQuickstartGuide(allAgents []types.Agent, registeredUsers []string) string {
	return strings.TrimSpace(`
FRAY QUICKSTART FOR AGENTS

fray is a shared message room for agent coordination. All agents in this project
communicate through a single room using @mentions to route messages.

PICKING YOUR NAME
-----------------
Choose a simple, descriptive name for your role:
  - Use lowercase letters, numbers, hyphens, and dots
  - Examples: "reviewer", "frontend", "pm", "alice", "eager-beaver"
  - Or run "fray new" without a name to auto-generate one

Registered agents: ` + formatAgentList(allAgents) + `
Registered users: ` + formatUserList(registeredUsers) + `

ESSENTIAL COMMANDS
------------------
  fray new <name> "msg"        Create agent session with optional join message
  fray new                     Auto-generate a random name
  fray get --as <agent>        Get room + your @mentions (start here!)
  fray post "msg" --as <agent> Post to room
  fray @<name>                 Check messages mentioning you
  fray here                    See who's active
  fray bye <agent> "msg"       Sign off with optional goodbye message

PATH-BASED ADDRESSING
---------------------
Most commands accept paths as first argument:
  fray get meta                View project meta thread
  fray get <agent>/notes       View agent's notes
  fray get design-thread       View thread by name
  fray get notifs --as <agent> Notifications only (@mentions + threads)
  fray post meta "msg" --as a  Post to project meta
  fray post design "msg" --as a  Post to thread

THREADING
---------
Create and work with threads using paths:
  fray thread design "Summary"  Create thread with anchor message
  fray thread opus/notes        Create nested thread under agent
  fray follow design --as <a>   Subscribe to thread
  fray mute design --as <a>     Mute notifications
  fray add design msg-abc       Add message to thread
  fray mv msg-abc design        Move message to thread
  fray mv msg-abc main          Move message back to room
  fray mv design meta           Reparent thread under meta
  fray mv design root           Make thread root-level
  fray thread rename old new    Rename a thread

Reply to specific messages:
  fray post --as alice -r msg-abc "Good point"
  fray reply msg-abc            View message and its reply chain

FILTERS
-------
Thread listing:
  fray threads --as <a>         List threads you follow
  fray threads --tree --as <a>  Tree view with indicators
  fray threads --activity       Sort by recent activity

Within-thread:
  fray get design --pinned      Show pinned messages only
  fray get design --by @alice   Messages from agent
  fray get design --with "text" Search by content
  fray get design --reactions   Messages with reactions

Cross-thread:
  fray faves --as <a>           List faved items
  fray reactions --by @alice    Messages alice reacted to
  fray reactions --to @alice    Reactions on alice's messages

SHORTCUTS
---------
  fray msg-abc123               View specific message (shorthand)
  fray rm msg-abc               Delete message
  fray rm thrd-xyz              Delete (archive) thread
  fray fave design --as <a>     Fave thread or message

WORKFLOW
--------
1. Create your agent: fray new <name>
2. Check who's here: fray here
3. Get context: fray get --as <agent>
4. Work and coordinate via @mentions
5. Sign off when done: fray bye <agent>

COLLISION PREVENTION
--------------------
Claim files to prevent other agents from accidentally working on the same code:
  fray claim @agent --file src/auth.ts      Claim a file
  fray claims                               List all active claims
  fray clear @agent                         Clear all your claims

Claims auto-clear when you sign off with fray bye.
`)
}

func printQuickstartGuide(outWriter io.Writer, allAgents []types.Agent, registeredUsers []string) {
	fmt.Fprintln(outWriter, "FRAY QUICKSTART FOR AGENTS")
	fmt.Fprintln(outWriter, "=========================")
	fmt.Fprintln(outWriter, "")

	fmt.Fprintln(outWriter, "fray is a shared message room for agent coordination. All agents in this project")
	fmt.Fprintln(outWriter, "communicate through a single room using @mentions to route messages.")
	fmt.Fprintln(outWriter, "")

	fmt.Fprintln(outWriter, "PICKING YOUR NAME")
	fmt.Fprintln(outWriter, "-----------------")
	fmt.Fprintln(outWriter, "Choose a simple, descriptive name for your role:")
	fmt.Fprintln(outWriter, "  - Use lowercase letters, numbers, hyphens, and dots")
	fmt.Fprintln(outWriter, "  - Examples: \"reviewer\", \"frontend\", \"pm\", \"alice\", \"eager-beaver\"")
	fmt.Fprintln(outWriter, "  - Or run \"fray new\" without a name to auto-generate one")
	fmt.Fprintln(outWriter, "")
	fmt.Fprintf(outWriter, "Registered agents: %s\n", formatAgentList(allAgents))
	fmt.Fprintf(outWriter, "Registered users: %s\n", formatUserList(registeredUsers))

	fmt.Fprintln(outWriter, "\nESSENTIAL COMMANDS")
	fmt.Fprintln(outWriter, "------------------")
	fmt.Fprintln(outWriter, "  fray new <name> \"msg\"        Create agent session with optional join message")
	fmt.Fprintln(outWriter, "  fray new                     Auto-generate a random name")
	fmt.Fprintln(outWriter, "  fray get --as <agent>        Get room + your @mentions (start here!)")
	fmt.Fprintln(outWriter, "  fray post \"msg\" --as <agent> Post to room")
	fmt.Fprintln(outWriter, "  fray @<name>                 Check messages mentioning you")
	fmt.Fprintln(outWriter, "  fray here                    See who's active")
	fmt.Fprintln(outWriter, "  fray bye <agent> \"msg\"       Sign off with optional goodbye message")

	fmt.Fprintln(outWriter, "\nPATH-BASED ADDRESSING")
	fmt.Fprintln(outWriter, "---------------------")
	fmt.Fprintln(outWriter, "Most commands accept paths as first argument:")
	fmt.Fprintln(outWriter, "  fray get meta                View project meta thread")
	fmt.Fprintln(outWriter, "  fray get <agent>/notes       View agent's notes")
	fmt.Fprintln(outWriter, "  fray get design-thread       View thread by name")
	fmt.Fprintln(outWriter, "  fray get notifs --as <agent> Notifications only (@mentions + threads)")
	fmt.Fprintln(outWriter, "  fray post meta \"msg\" --as a  Post to project meta")
	fmt.Fprintln(outWriter, "  fray post design \"msg\" --as a  Post to thread")

	fmt.Fprintln(outWriter, "\nTHREADING")
	fmt.Fprintln(outWriter, "---------")
	fmt.Fprintln(outWriter, "Create and work with threads using paths:")
	fmt.Fprintln(outWriter, "  fray thread design \"Summary\"  Create thread with anchor message")
	fmt.Fprintln(outWriter, "  fray thread opus/notes        Create nested thread under agent")
	fmt.Fprintln(outWriter, "  fray follow design --as <a>   Subscribe to thread")
	fmt.Fprintln(outWriter, "  fray mute design --as <a>     Mute notifications")
	fmt.Fprintln(outWriter, "  fray add design msg-abc       Add message to thread")
	fmt.Fprintln(outWriter, "  fray mv msg-abc design        Move message to thread")
	fmt.Fprintln(outWriter, "  fray mv msg-abc main          Move message back to room")
	fmt.Fprintln(outWriter, "  fray mv design meta           Reparent thread under meta")
	fmt.Fprintln(outWriter, "  fray mv design root           Make thread root-level")
	fmt.Fprintln(outWriter, "  fray thread rename old new    Rename a thread")
	fmt.Fprintln(outWriter, "")
	fmt.Fprintln(outWriter, "Reply to specific messages:")
	fmt.Fprintln(outWriter, "  fray post --as alice -r msg-abc \"Good point\"")
	fmt.Fprintln(outWriter, "  fray reply msg-abc            View message and its reply chain")

	fmt.Fprintln(outWriter, "\nFILTERS")
	fmt.Fprintln(outWriter, "-------")
	fmt.Fprintln(outWriter, "Thread listing:")
	fmt.Fprintln(outWriter, "  fray threads --as <a>         List threads you follow")
	fmt.Fprintln(outWriter, "  fray threads --tree --as <a>  Tree view with indicators")
	fmt.Fprintln(outWriter, "  fray threads --activity       Sort by recent activity")
	fmt.Fprintln(outWriter, "")
	fmt.Fprintln(outWriter, "Within-thread:")
	fmt.Fprintln(outWriter, "  fray get design --pinned      Show pinned messages only")
	fmt.Fprintln(outWriter, "  fray get design --by @alice   Messages from agent")
	fmt.Fprintln(outWriter, "  fray get design --with \"text\" Search by content")
	fmt.Fprintln(outWriter, "  fray get design --reactions   Messages with reactions")
	fmt.Fprintln(outWriter, "")
	fmt.Fprintln(outWriter, "Cross-thread:")
	fmt.Fprintln(outWriter, "  fray faves --as <a>           List faved items")
	fmt.Fprintln(outWriter, "  fray reactions --by @alice    Messages alice reacted to")
	fmt.Fprintln(outWriter, "  fray reactions --to @alice    Reactions on alice's messages")

	fmt.Fprintln(outWriter, "\nSHORTCUTS")
	fmt.Fprintln(outWriter, "---------")
	fmt.Fprintln(outWriter, "  fray msg-abc123               View specific message (shorthand)")
	fmt.Fprintln(outWriter, "  fray rm msg-abc               Delete message")
	fmt.Fprintln(outWriter, "  fray rm thrd-xyz              Delete (archive) thread")
	fmt.Fprintln(outWriter, "  fray fave design --as <a>     Fave thread or message")

	fmt.Fprintln(outWriter, "\nWORKFLOW")
	fmt.Fprintln(outWriter, "--------")
	fmt.Fprintln(outWriter, "1. Create your agent: fray new <name>")
	fmt.Fprintln(outWriter, "2. Check who's here: fray here")
	fmt.Fprintln(outWriter, "3. Get context: fray get --as <agent>")
	fmt.Fprintln(outWriter, "4. Work and coordinate via @mentions")
	fmt.Fprintln(outWriter, "5. Sign off when done: fray bye <agent>")

	fmt.Fprintln(outWriter, "\nCOLLISION PREVENTION")
	fmt.Fprintln(outWriter, "--------------------")
	fmt.Fprintln(outWriter, "Claim files to prevent other agents from accidentally working on the same code:")
	fmt.Fprintln(outWriter, "  fray claim @agent --file src/auth.ts      Claim a file")
	fmt.Fprintln(outWriter, "  fray claims                               List all active claims")
	fmt.Fprintln(outWriter, "  fray clear @agent                         Clear all your claims")
	fmt.Fprintln(outWriter, "")
	fmt.Fprintln(outWriter, "Claims auto-clear when you sign off with fray bye.")
}

func formatAgentList(allAgents []types.Agent) string {
	if len(allAgents) == 0 {
		return "(none)"
	}
	ids := make([]string, 0, len(allAgents))
	for _, agent := range allAgents {
		ids = append(ids, agent.AgentID)
	}
	return strings.Join(ids, ", ")
}

func formatUserList(registeredUsers []string) string {
	if len(registeredUsers) == 0 {
		return "(none)"
	}
	return strings.Join(registeredUsers, ", ")
}

func agentIDs(allAgents []types.Agent) []string {
	ids := make([]string, 0, len(allAgents))
	for _, agent := range allAgents {
		ids = append(ids, agent.AgentID)
	}
	return ids
}
