package chat

import (
	"database/sql"
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	"github.com/gen2brain/beeep"
)

//go:embed icon.icns
var notifyIconICNS []byte

//go:embed icon.png
var notifyIconPNG []byte

const (
	appBundleID       = "com.fray.notifier"
	notifierAppName   = "Fray-Notifier.app"
	notifierBinaryRel = "Contents/MacOS/terminal-notifier"
)

var (
	notifierPath     string
	notifierPathOnce sync.Once
)

// getNotifierPath returns the path to the custom Fray-Notifier.app binary if available.
func getNotifierPath() string {
	notifierPathOnce.Do(func() {
		if runtime.GOOS != "darwin" {
			return
		}

		home, err := os.UserHomeDir()
		if err != nil {
			return
		}

		// Check ~/Applications/Fray-Notifier.app
		candidate := filepath.Join(home, "Applications", notifierAppName, notifierBinaryRel)
		if _, err := os.Stat(candidate); err == nil {
			notifierPath = candidate
		}
	})
	return notifierPath
}

var directMentionRe = regexp.MustCompile(`^@([a-z][a-z0-9]*(?:[-\.][a-z0-9]+)*)`)

// IsDirectMention returns true if the message starts with a mention of the given agent.
func IsDirectMention(body, agentID string) bool {
	match := directMentionRe.FindStringSubmatch(body)
	if match == nil {
		return false
	}
	mention := match[1]
	return core.MatchesMention(agentID, mention)
}

// HasMention checks if the message mentions the given agent (direct or FYI).
func HasMention(msg types.Message, agentID string) bool {
	for _, mention := range msg.Mentions {
		if core.MatchesMention(agentID, mention) {
			return true
		}
	}
	return false
}

// IsReplyToAgent checks if the message is a reply to a message from the given agent.
func IsReplyToAgent(dbConn *sql.DB, msg types.Message, agentID string) bool {
	if msg.ReplyTo == nil {
		return false
	}
	parent, err := db.GetMessage(dbConn, *msg.ReplyTo)
	if err != nil || parent == nil {
		return false
	}
	return core.MatchesMention(agentID, parent.FromAgent)
}

// getFocusScriptPath returns the path to the focus-fray script in the app bundle.
func getFocusScriptPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	scriptPath := filepath.Join(home, "Applications", notifierAppName, "Contents", "MacOS", "focus-fray")
	if _, err := os.Stat(scriptPath); err != nil {
		return ""
	}
	return scriptPath
}

// GotoFilePath is where the focus script writes the navigation target.
const GotoFilePath = "/tmp/fray-goto"

// SendNotification sends an OS notification for a message.
func SendNotification(msg types.Message, projectName string) error {
	title := "@" + msg.FromAgent
	if projectName != "" {
		title = projectName + " · " + title
	}
	body := truncateNotification(msg.Body, 100)

	// On macOS with Fray-Notifier.app, use it directly for proper icon
	if notifier := getNotifierPath(); notifier != "" {
		args := []string{
			"-title", title,
			"-message", body,
			"-group", "fray",
		}
		// Add click-to-focus if the focus script is available
		if focusScript := getFocusScriptPath(); focusScript != "" {
			// Pass thread/message info as argument to focus script
			target := msg.ID
			if msg.Home != "" {
				target = msg.Home + "#" + msg.ID
			}
			args = append(args, "-execute", focusScript+" "+target)
		}
		cmd := exec.Command(notifier, args...)
		return cmd.Run()
	}

	// Fallback to beeep for other platforms or if notifier not installed
	return beeep.Notify(title, body, "")
}

func truncateNotification(s string, maxLen int) string {
	// Collapse whitespace for notification
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
