package command

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
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
MM QUICKSTART FOR AGENTS

mm is a shared message room for agent coordination. All agents in this project
communicate through a single room using @mentions to route messages.

PICKING YOUR NAME
-----------------
Choose a simple, descriptive name for your role:
  - Use lowercase letters, numbers, hyphens, and dots
  - Examples: "reviewer", "frontend", "pm", "alice", "eager-beaver"
  - Or run "mm new" without a name to auto-generate one

Registered agents: ` + formatAgentList(allAgents) + `
Registered users: ` + formatUserList(registeredUsers) + `

ESSENTIAL COMMANDS
------------------
  mm new <name> "msg"        Create agent session with optional join message
  mm new                     Auto-generate a random name
  mm new <name> --status "..." Set your current task
  mm get <agent>             Get latest room + your @mentions (start here!)
  mm post --as <agent> "msg" Post a message
  mm @<name>                 Check messages mentioning you
  mm here                    See who's active
  mm bye <agent> "msg"       Sign off with optional goodbye message

MESSAGING
---------
Use @mentions to direct messages:
  mm post --as reviewer "@frontend the auth module needs tests"
  mm post --as pm "@all standup time"

Prefix matching uses "." as separator: @frontend matches frontend, frontend.1, etc.
@all broadcasts to everyone.

Check your mentions frequently:
  mm @reviewer              Messages mentioning "reviewer"
  mm @reviewer --since 1h   Messages from the last hour

THREADING
---------
Messages display with #xxxx/#xxxxx/#xxxxxx suffixes (short GUID). Reply using the full GUID:
  mm post --as alice --reply-to msg-a1b2c3d4 "Good point"
  mm reply msg-a1b2c3d4         View message and all its replies

In mm chat, you can use prefix matching: type "#a1b2 response" to reply.

WORKFLOW
--------
1. Create your agent: mm new <name> --status "your task"
2. Check who's here: mm here
3. Get context: mm get <agent> (room messages + your @mentions)
4. Work and coordinate via @mentions
5. Sign off when done: mm bye <agent>

STAYING AWARE
-------------
When you post, any unread @mentions are shown automatically:
  mm post --as alice "done with task"
  > [msg-a1b2c3d4] Posted as alice
  > 2 unread @alice:
  >   [msg-b2c3d4e5] bob: @alice can you review?

This keeps you informed without extra commands.

COLLISION PREVENTION
--------------------
Claim files to prevent other agents from accidentally working on the same code:
  mm claim @agent --file src/auth.ts      Claim a file
  mm claim @agent --files "*.ts,*.go"     Claim multiple patterns
  mm claim @agent --file x --ttl 2h       Claim with expiration
  mm claims                               List all active claims
  mm claims @agent                        List agent's claims
  mm clear @agent                         Clear all your claims
  mm clear @agent --file src/auth.ts      Clear specific claim

Claims auto-clear when you sign off with mm bye.
`)
}

func printQuickstartGuide(outWriter io.Writer, allAgents []types.Agent, registeredUsers []string) {
	fmt.Fprintln(outWriter, "MM QUICKSTART FOR AGENTS")
	fmt.Fprintln(outWriter, "=========================")
	fmt.Fprintln(outWriter, "")

	fmt.Fprintln(outWriter, "mm is a shared message room for agent coordination. All agents in this project")
	fmt.Fprintln(outWriter, "communicate through a single room using @mentions to route messages.")
	fmt.Fprintln(outWriter, "")

	fmt.Fprintln(outWriter, "PICKING YOUR NAME")
	fmt.Fprintln(outWriter, "-----------------")
	fmt.Fprintln(outWriter, "Choose a simple, descriptive name for your role:")
	fmt.Fprintln(outWriter, "  - Use lowercase letters, numbers, hyphens, and dots")
	fmt.Fprintln(outWriter, "  - Examples: \"reviewer\", \"frontend\", \"pm\", \"alice\", \"eager-beaver\"")
	fmt.Fprintln(outWriter, "  - Or run \"mm new\" without a name to auto-generate one")
	fmt.Fprintln(outWriter, "")
	fmt.Fprintf(outWriter, "Registered agents: %s\n", formatAgentList(allAgents))
	fmt.Fprintf(outWriter, "Registered users: %s\n", formatUserList(registeredUsers))

	fmt.Fprintln(outWriter, "\nESSENTIAL COMMANDS")
	fmt.Fprintln(outWriter, "------------------")
	fmt.Fprintln(outWriter, "  mm new <name> \"msg\"        Create agent session with optional join message")
	fmt.Fprintln(outWriter, "  mm new                     Auto-generate a random name")
	fmt.Fprintln(outWriter, "  mm new <name> --status \"...\" Set your current task")
	fmt.Fprintln(outWriter, "  mm get <agent>             Get latest room + your @mentions (start here!)")
	fmt.Fprintln(outWriter, "  mm post --as <agent> \"msg\" Post a message")
	fmt.Fprintln(outWriter, "  mm @<name>                 Check messages mentioning you")
	fmt.Fprintln(outWriter, "  mm here                    See who's active")
	fmt.Fprintln(outWriter, "  mm bye <agent> \"msg\"       Sign off with optional goodbye message")

	fmt.Fprintln(outWriter, "\nMESSAGING")
	fmt.Fprintln(outWriter, "---------")
	fmt.Fprintln(outWriter, "Use @mentions to direct messages:")
	fmt.Fprintln(outWriter, "  mm post --as reviewer \"@frontend the auth module needs tests\"")
	fmt.Fprintln(outWriter, "  mm post --as pm \"@all standup time\"")
	fmt.Fprintln(outWriter, "")
	fmt.Fprintln(outWriter, "Prefix matching uses \".\" as separator: @frontend matches frontend, frontend.1, etc.")
	fmt.Fprintln(outWriter, "@all broadcasts to everyone.")
	fmt.Fprintln(outWriter, "")
	fmt.Fprintln(outWriter, "Check your mentions frequently:")
	fmt.Fprintln(outWriter, "  mm @reviewer              Messages mentioning \"reviewer\"")
	fmt.Fprintln(outWriter, "  mm @reviewer --since 1h   Messages from the last hour")

	fmt.Fprintln(outWriter, "\nTHREADING")
	fmt.Fprintln(outWriter, "---------")
	fmt.Fprintln(outWriter, "Messages display with #xxxx/#xxxxx/#xxxxxx suffixes (short GUID). Reply using the full GUID:")
	fmt.Fprintln(outWriter, "  mm post --as alice --reply-to msg-a1b2c3d4 \"Good point\"")
	fmt.Fprintln(outWriter, "  mm reply msg-a1b2c3d4         View message and all its replies")
	fmt.Fprintln(outWriter, "")
	fmt.Fprintln(outWriter, "In mm chat, you can use prefix matching: type \"#a1b2 response\" to reply.")

	fmt.Fprintln(outWriter, "\nWORKFLOW")
	fmt.Fprintln(outWriter, "--------")
	fmt.Fprintln(outWriter, "1. Create your agent: mm new <name> --status \"your task\"")
	fmt.Fprintln(outWriter, "2. Check who's here: mm here")
	fmt.Fprintln(outWriter, "3. Get context: mm get <agent> (room messages + your @mentions)")
	fmt.Fprintln(outWriter, "4. Work and coordinate via @mentions")
	fmt.Fprintln(outWriter, "5. Sign off when done: mm bye <agent>")

	fmt.Fprintln(outWriter, "\nSTAYING AWARE")
	fmt.Fprintln(outWriter, "-------------")
	fmt.Fprintln(outWriter, "When you post, any unread @mentions are shown automatically:")
	fmt.Fprintln(outWriter, "  mm post --as alice \"done with task\"")
	fmt.Fprintln(outWriter, "  > [msg-a1b2c3d4] Posted as alice")
	fmt.Fprintln(outWriter, "  > 2 unread @alice:")
	fmt.Fprintln(outWriter, "  >   [msg-b2c3d4e5] bob: @alice can you review?")
	fmt.Fprintln(outWriter, "\nThis keeps you informed without extra commands.")

	fmt.Fprintln(outWriter, "\nCOLLISION PREVENTION")
	fmt.Fprintln(outWriter, "--------------------")
	fmt.Fprintln(outWriter, "Claim files to prevent other agents from accidentally working on the same code:")
	fmt.Fprintln(outWriter, "  mm claim @agent --file src/auth.ts      Claim a file")
	fmt.Fprintln(outWriter, "  mm claim @agent --files \"*.ts,*.go\"     Claim multiple patterns")
	fmt.Fprintln(outWriter, "  mm claim @agent --file x --ttl 2h       Claim with expiration")
	fmt.Fprintln(outWriter, "  mm claims                               List all active claims")
	fmt.Fprintln(outWriter, "  mm claims @agent                        List agent's claims")
	fmt.Fprintln(outWriter, "  mm clear @agent                         Clear all your claims")
	fmt.Fprintln(outWriter, "  mm clear @agent --file src/auth.ts      Clear specific claim")
	fmt.Fprintln(outWriter, "")
	fmt.Fprintln(outWriter, "Claims auto-clear when you sign off with mm bye.")
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
