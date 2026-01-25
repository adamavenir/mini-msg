package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) selectChannelCmd() tea.Cmd {
	if len(m.channels) == 0 {
		return nil
	}
	entry := m.channels[m.channelIndex]
	if samePath(entry.Path, m.projectRoot) {
		m.sidebarFocus = false
		m.sidebarOpen = false
		m.resetSidebarFilter()
		m.resize()
		return nil
	}
	if err := m.switchChannel(entry); err != nil {
		m.status = err.Error()
		return nil
	}
	m.sidebarFocus = false
	m.sidebarOpen = false
	m.resetSidebarFilter()
	m.resize()
	return m.pollCmd()
}

func (m *Model) switchChannel(entry channelEntry) error {
	project, err := projectFromRoot(entry.Path)
	if err != nil {
		return err
	}
	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		return err
	}
	if err := db.InitSchema(dbConn); err != nil {
		_ = dbConn.Close()
		return err
	}

	if m.db != nil {
		_ = m.db.Close()
	}
	m.db = dbConn
	m.projectRoot = project.Root
	m.projectDBPath = project.DBPath
	m.projectName = filepath.Base(project.Root)

	// Reset thread navigation state - threads from old channel don't exist here
	m.currentThread = nil
	m.currentPseudo = ""
	m.threadMessages = nil
	m.drillPath = nil
	m.threadScrollOffset = 0

	// Reset edit mode if active
	if m.editingMessageID != "" {
		m.exitEditMode()
	}

	// Clear pending input (user was composing for old channel)
	if m.input.Value() != "" {
		m.input.Reset()
		m.clearSuggestions()
		m.lastInputValue = ""
		m.lastInputPos = 0
	}

	// Reload threads for new channel
	threads, threadIndex := loadThreads(dbConn, m.username)
	m.threads = threads
	m.threadIndex = threadIndex

	// Refresh thread-related state
	m.refreshUnreadCounts()
	m.refreshFavedThreads()
	m.refreshSubscribedThreads()
	m.refreshMutedThreads()
	m.refreshThreadNicknames()
	m.refreshAvatars()
	m.refreshQuestionCounts()

	if err := m.reloadMessages(); err != nil {
		return err
	}
	m.status = fmt.Sprintf("Switched to %s", formatChannelLabel(entry))
	return nil
}

func loadChannels(currentRoot string) ([]channelEntry, int) {
	config, err := core.ReadGlobalConfig()
	if err != nil || config == nil || len(config.Channels) == 0 {
		return nil, 0
	}

	entries := make([]channelEntry, 0, len(config.Channels))
	for id, ref := range config.Channels {
		entries = append(entries, channelEntry{ID: id, Name: ref.Name, Path: ref.Path})
	}
	sort.Slice(entries, func(i, j int) bool {
		left := strings.ToLower(entries[i].Name)
		right := strings.ToLower(entries[j].Name)
		if left == "" {
			left = strings.ToLower(entries[i].ID)
		}
		if right == "" {
			right = strings.ToLower(entries[j].ID)
		}
		return left < right
	})

	index := 0
	for i, entry := range entries {
		if samePath(entry.Path, currentRoot) {
			index = i
			break
		}
	}
	return entries, index
}

func formatChannelLabel(entry channelEntry) string {
	name := entry.Name
	if name == "" {
		name = entry.ID
	}
	return "#" + name
}

func samePath(left, right string) bool {
	normalizedLeft, errLeft := filepath.Abs(left)
	normalizedRight, errRight := filepath.Abs(right)
	if errLeft == nil && errRight == nil {
		return normalizedLeft == normalizedRight
	}
	return left == right
}

func projectFromRoot(rootPath string) (core.Project, error) {
	dbPath := filepath.Join(rootPath, ".fray", "fray.db")
	if _, err := os.Stat(dbPath); err != nil {
		return core.Project{}, fmt.Errorf("channel database not found at %s", dbPath)
	}
	return core.Project{Root: rootPath, DBPath: dbPath}, nil
}
