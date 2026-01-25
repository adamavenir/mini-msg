package chat

const suggestionLimit = 8

type suggestionKind int

const (
	suggestionNone suggestionKind = iota
	suggestionMention
	suggestionReply
	suggestionCommand
	suggestionScript // for /run script arguments
)

// commandDef defines a slash command with its name, description, and optional usage.
type commandDef struct {
	Name  string
	Desc  string
	Usage string // Shown after command is completed (with space)
}

// allCommands is the list of available slash commands.
var allCommands = []commandDef{
	{Name: "/quit", Desc: "Exit chat"},
	{Name: "/exit", Desc: "Exit chat"},
	{Name: "/help", Desc: "Show help"},
	{Name: "/fave", Desc: "Fave current thread", Usage: "[thread]"},
	{Name: "/unfave", Desc: "Unfave current thread", Usage: "[thread]"},
	{Name: "/follow", Desc: "Follow current thread", Usage: "[thread]"},
	{Name: "/unfollow", Desc: "Unfollow current thread", Usage: "[thread]"},
	{Name: "/mute", Desc: "Mute current thread", Usage: "[thread]"},
	{Name: "/unmute", Desc: "Unmute current thread", Usage: "[thread]"},
	{Name: "/archive", Desc: "Archive current thread", Usage: "[thread]"},
	{Name: "/restore", Desc: "Restore archived thread", Usage: "<thread>"},
	{Name: "/rename", Desc: "Rename current thread", Usage: "<new-name>"},
	{Name: "/mv", Desc: "Move message or thread", Usage: "[#msg-id] <destination>"},
	{Name: "/n", Desc: "Set thread nickname", Usage: "<nickname>"},
	{Name: "/pin", Desc: "Pin a message", Usage: "<#msg-id>"},
	{Name: "/unpin", Desc: "Unpin a message", Usage: "<#msg-id>"},
	{Name: "/edit", Desc: "Edit a message", Usage: "<#msg-id> [text] [-m reason]"},
	{Name: "/delete", Desc: "Delete a message", Usage: "<#msg-id>"},
	{Name: "/rm", Desc: "Delete a message", Usage: "<#msg-id>"},
	{Name: "/prune", Desc: "Archive old messages (current thread)", Usage: "[target] [--keep N] [--with-react emoji]"},
	{Name: "/thread", Desc: "Create a new thread", Usage: "<name> [\"anchor\"]"},
	{Name: "/t", Desc: "Create a new thread", Usage: "<name> [\"anchor\"]"},
	{Name: "/subthread", Desc: "Create subthread of current", Usage: "<name> [\"anchor\"]"},
	{Name: "/st", Desc: "Create subthread of current", Usage: "<name> [\"anchor\"]"},
	{Name: "/close", Desc: "Close questions for message", Usage: "<#msg-id>"},
	{Name: "/run", Desc: "Run mlld script", Usage: "<script-name>"},
	{Name: "/bye", Desc: "Send bye for agent", Usage: "@agent [message]"},
	{Name: "/fly", Desc: "Spawn agent (fresh session)", Usage: "@agent [message]"},
	{Name: "/hop", Desc: "Spawn agent (quick task, auto-bye)", Usage: "@agent [message]"},
	{Name: "/land", Desc: "Ask agent to run /land closeout", Usage: "@agent"},
}

type suggestionItem struct {
	Display string
	Insert  string
}

type mentionCandidate struct {
	Name  string
	Nicks []string
}
