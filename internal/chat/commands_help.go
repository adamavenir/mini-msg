package chat

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const (
	helpLabelStyle = "\x1b[1m\x1b[97m"
	helpItemStyle  = "\x1b[94m"
	helpResetStyle = "\x1b[0m"
)

func (m *Model) showHelp() {
	if m.helpMessageID != "" {
		_ = m.removeMessageByID(m.helpMessageID)
	}
	msg := newEventMessage(buildHelpText())
	m.helpMessageID = msg.ID
	m.messages = append(m.messages, msg)
	m.status = ""
	m.refreshViewport(false)
}

func buildHelpText() string {
	label := helpLabelStyle + "Shortcuts" + helpResetStyle
	lines := []string{
		label,
		formatHelpRow(
			helpItemStyle+"Ctrl-C"+helpResetStyle+" - clear text",
			helpItemStyle+"Up"+helpResetStyle+" - edit last",
			helpItemStyle+"Tab"+helpResetStyle+" - thread panel",
			25,
			22,
		),
		"",
		helpLabelStyle + "Create Threads" + helpResetStyle,
		formatHelpRow(
			helpItemStyle+"/t <name> \"anchor\""+helpResetStyle,
			helpItemStyle+"/st <name> \"anchor\""+helpResetStyle,
			"",
			24,
			22,
		),
		"",
		helpLabelStyle + "Thread Commands" + helpResetStyle + " (operate on current thread)",
		formatHelpRow(
			helpItemStyle+"/fave /unfave"+helpResetStyle,
			helpItemStyle+"/follow /unfollow"+helpResetStyle,
			helpItemStyle+"/mute /unmute"+helpResetStyle,
			18,
			22,
		),
		formatHelpRow(
			helpItemStyle+"/archive /restore"+helpResetStyle,
			helpItemStyle+"/rename <name>"+helpResetStyle,
			helpItemStyle+"/n <nick>"+helpResetStyle,
			18,
			22,
		),
		formatHelpRow(
			helpItemStyle+"/mv <dest>"+helpResetStyle+" (thread)",
			helpItemStyle+"/mv #id <dest>"+helpResetStyle+" (msg)",
			"",
			18,
			22,
		),
		"",
		helpLabelStyle + "Message Commands" + helpResetStyle,
		formatHelpRow(
			helpItemStyle+"/edit <id> <text> -m <reason>"+helpResetStyle,
			helpItemStyle+"/delete <id>"+helpResetStyle,
			"",
			35,
			22,
		),
		"",
		helpItemStyle + "Click" + helpResetStyle + " a message to reply. " +
			helpItemStyle + "Double-click" + helpResetStyle + " to copy.",
	}
	return strings.Join(lines, "\n")
}

func formatHelpRow(col1, col2, col3 string, col1Width, col2Width int) string {
	row := padHelpColumn(col1, col1Width) + padHelpColumn(col2, col2Width)
	if col3 != "" {
		row += col3
	}
	return strings.TrimRight(row, " ")
}

func padHelpColumn(value string, width int) string {
	if width <= 0 {
		return value
	}
	pad := width - ansi.StringWidth(value)
	if pad < 2 {
		pad = 2
	}
	return value + strings.Repeat(" ", pad)
}
