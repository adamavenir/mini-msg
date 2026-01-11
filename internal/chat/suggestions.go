package chat

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const suggestionLimit = 8

type suggestionKind int

const (
	suggestionNone suggestionKind = iota
	suggestionMention
	suggestionReply
	suggestionCommand
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

func (m *Model) handleSuggestionKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if len(m.suggestions) == 0 {
		return false, nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.clearSuggestions()
		m.resize()
		return true, nil
	case tea.KeyUp:
		if m.suggestionIndex < 0 {
			// First navigation - start from bottom
			m.suggestionIndex = len(m.suggestions) - 1
		} else {
			m.suggestionIndex--
			if m.suggestionIndex < 0 {
				m.suggestionIndex = len(m.suggestions) - 1
			}
		}
		return true, nil
	case tea.KeyDown:
		if m.suggestionIndex < 0 {
			// First navigation - start from top
			m.suggestionIndex = 0
		} else {
			m.suggestionIndex++
			if m.suggestionIndex >= len(m.suggestions) {
				m.suggestionIndex = 0
			}
		}
		return true, nil
	case tea.KeyTab:
		// Tab selects first if none selected, then applies
		if m.suggestionIndex < 0 {
			m.suggestionIndex = 0
		}
		if m.suggestionIndex < len(m.suggestions) {
			m.applySuggestion(m.suggestions[m.suggestionIndex])
		}
		return true, nil
	case tea.KeyEnter:
		// Apply suggestion but don't consume Enter - let main handler submit
		if m.suggestionIndex >= 0 && m.suggestionIndex < len(m.suggestions) {
			m.applySuggestion(m.suggestions[m.suggestionIndex])
		}
		return false, nil
	}
	return false, nil
}

func (m *Model) refreshSuggestions() {
	value := m.input.Value()
	pos := m.inputCursorPos()
	if value == m.lastInputValue && pos == m.lastInputPos {
		return
	}
	m.lastInputValue = value
	m.lastInputPos = pos
	m.updateInputStyle()
	m.dismissHelpOnInput(value)

	// Check for slash command completion
	if strings.HasPrefix(value, "/") {
		spaceIdx := strings.Index(value, " ")
		if spaceIdx == -1 || pos <= spaceIdx {
			// Still typing the command name - show command suggestions
			prefix := value[1:pos]
			if spaceIdx != -1 && pos > spaceIdx {
				prefix = value[1:spaceIdx]
			}
			suggestions := buildCommandSuggestions(prefix)
			if len(suggestions) > 0 {
				m.suggestions = suggestions
				m.suggestionIndex = -1 // No selection until user navigates
				m.suggestionStart = 0
				m.suggestionKind = suggestionCommand
				m.resize()
				return
			}
		} else {
			// Command is complete (has space) - show dynamic suggestions or usage help
			cmdName := value[:spaceIdx]
			argPrefix := strings.TrimSpace(value[spaceIdx+1:])

			// Special handling for /run - show available scripts
			if cmdName == "/run" {
				scriptSuggestions := m.buildRunScriptSuggestions(argPrefix)
				if len(scriptSuggestions) > 0 {
					m.suggestions = scriptSuggestions
					m.suggestionIndex = -1
					m.suggestionStart = spaceIdx + 1
					m.suggestionKind = suggestionCommand
					m.resize()
					return
				}
			}

			usageHelp := getCommandUsage(cmdName)
			if usageHelp != "" {
				m.suggestions = []suggestionItem{{
					Display: usageHelp,
					Insert:  "", // Not insertable
				}}
				m.suggestionIndex = -1 // No selection
				m.suggestionKind = suggestionCommand
				m.resize()
				return
			}
		}
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}

	kind, start, prefix := findSuggestionToken(value, pos)
	if kind == suggestionNone {
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}

	suggestions, err := m.buildSuggestions(kind, prefix)
	if err != nil {
		m.status = err.Error()
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}
	if len(suggestions) == 0 {
		if len(m.suggestions) > 0 {
			m.clearSuggestions()
			m.resize()
		}
		return
	}

	m.suggestions = suggestions
	m.suggestionIndex = -1 // No selection until user navigates
	m.suggestionStart = start
	m.suggestionKind = kind
	m.resize()
}

func (m *Model) clearSuggestions() {
	m.suggestions = nil
	m.suggestionIndex = -1
	m.suggestionStart = 0
	m.suggestionKind = suggestionNone
}

func (m *Model) suggestionHeight() int {
	if len(m.suggestions) == 0 {
		return 0
	}
	return lipgloss.Height(m.renderSuggestions())
}

func (m *Model) renderSuggestions() string {
	if len(m.suggestions) == 0 {
		return ""
	}
	normalStyle := lipgloss.NewStyle().Foreground(metaColor)
	selectedStyle := lipgloss.NewStyle().Foreground(userColor).Bold(true)

	lines := make([]string, 0, len(m.suggestions))
	for i, suggestion := range m.suggestions {
		prefix := "  "
		style := normalStyle
		if i == m.suggestionIndex {
			prefix = "> "
			style = selectedStyle
		}
		line := prefix + suggestion.Display
		if m.mainWidth() > 0 {
			line = truncateLine(line, m.mainWidth())
		}
		lines = append(lines, style.Render(line))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) applySuggestion(item suggestionItem) {
	value := []rune(m.input.Value())
	cursor := m.inputCursorPos()
	start := m.suggestionStart
	if start < 0 || start > len(value) {
		start = cursor
	}
	if cursor < start {
		cursor = start
	}

	// For command suggestions, replace up to first space (or cursor if typing args)
	if m.suggestionKind == suggestionCommand {
		valueStr := m.input.Value()
		spaceIdx := strings.Index(valueStr, " ")
		if spaceIdx == -1 {
			// No space - replace entire input with command + space
			m.input.SetValue(item.Insert + " ")
		} else {
			// Space exists - replace only up to space, keep args
			m.input.SetValue(item.Insert + valueStr[spaceIdx:])
		}
		m.input.CursorEnd()
		m.clearSuggestions()
		m.lastInputValue = m.input.Value()
		m.lastInputPos = m.inputCursorPos()
		m.resize()
		return
	}

	before := value[:start]
	after := value[cursor:]
	insertRunes := []rune(item.Insert)
	if len(after) == 0 {
		insertRunes = append(insertRunes, ' ')
	}

	updated := append(append(before, insertRunes...), after...)
	m.input.SetValue(string(updated))
	m.input.CursorEnd()
	m.clearSuggestions()
	m.lastInputValue = m.input.Value()
	m.lastInputPos = m.inputCursorPos()
	m.resize()
}

func findSuggestionToken(value string, cursor int) (suggestionKind, int, string) {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	for i := cursor - 1; i >= 0; i-- {
		if unicode.IsSpace(runes[i]) {
			break
		}
		if runes[i] != '@' && runes[i] != '#' {
			continue
		}
		if i > 0 && isAlphaNum(runes[i-1]) {
			return suggestionNone, 0, ""
		}
		prefix := string(runes[i+1 : cursor])
		if runes[i] == '@' {
			return suggestionMention, i, prefix
		}
		normalized := normalizePrefix(prefix)
		if len(normalized) < 2 {
			return suggestionNone, 0, ""
		}
		return suggestionReply, i, normalized
	}
	return suggestionNone, 0, ""
}

func (m *Model) buildSuggestions(kind suggestionKind, prefix string) ([]suggestionItem, error) {
	switch kind {
	case suggestionMention:
		return m.buildMentionSuggestions(prefix)
	case suggestionReply:
		return m.buildReplySuggestions(prefix)
	default:
		return nil, nil
	}
}

// buildRunScriptSuggestions returns script suggestions for /run command.
// Searches both .fray/llm/run/ (fray scripts) and llm/run/ (project scripts).
func (m *Model) buildRunScriptSuggestions(prefix string) []suggestionItem {
	// Collect scripts from both locations
	frayRunDir := filepath.Join(m.projectRoot, ".fray", "llm", "run")
	projRunDir := filepath.Join(m.projectRoot, "llm", "run")

	seen := make(map[string]bool)
	suggestions := make([]suggestionItem, 0, suggestionLimit)
	prefix = strings.ToLower(prefix)

	// Add fray scripts first
	if scripts, err := listMlldScripts(frayRunDir); err == nil {
		for _, script := range scripts {
			if prefix != "" && !strings.HasPrefix(strings.ToLower(script), prefix) {
				continue
			}
			if seen[script] {
				continue
			}
			seen[script] = true
			suggestions = append(suggestions, suggestionItem{
				Display: script,
				Insert:  script,
			})
			if len(suggestions) >= suggestionLimit {
				return suggestions
			}
		}
	}

	// Add project scripts
	if scripts, err := listMlldScripts(projRunDir); err == nil {
		for _, script := range scripts {
			if prefix != "" && !strings.HasPrefix(strings.ToLower(script), prefix) {
				continue
			}
			if seen[script] {
				continue
			}
			seen[script] = true
			suggestions = append(suggestions, suggestionItem{
				Display: script,
				Insert:  script,
			})
			if len(suggestions) >= suggestionLimit {
				return suggestions
			}
		}
	}

	return suggestions
}

func (m *Model) buildMentionSuggestions(prefix string) ([]suggestionItem, error) {
	bases, err := db.GetAgentBases(m.db)
	if err != nil {
		return nil, err
	}

	projectConfig, _ := db.ReadProjectConfig(m.projectDBPath)
	candidates := buildMentionCandidates(bases, projectConfig)

	normalized := strings.ToLower(prefix)
	suggestions := make([]suggestionItem, 0, suggestionLimit)
	for _, candidate := range candidates {
		matchingNicks := matchNickPrefix(candidate.Nicks, normalized)
		nameLower := strings.ToLower(candidate.Name)
		if normalized != "" && !strings.HasPrefix(nameLower, normalized) && len(matchingNicks) == 0 {
			continue
		}
		label := "@" + candidate.Name
		if len(matchingNicks) > 0 {
			label = fmt.Sprintf("@%s (aka %s)", candidate.Name, formatNickList(matchingNicks))
		}
		insert := "@" + candidate.Name
		suggestions = append(suggestions, suggestionItem{Display: label, Insert: insert})
		if len(suggestions) >= suggestionLimit {
			break
		}
	}
	return suggestions, nil
}

func buildMentionCandidates(bases map[string]struct{}, config *db.ProjectConfig) []mentionCandidate {
	nameToNicks := map[string][]string{}
	if config != nil && len(config.KnownAgents) > 0 {
		for _, entry := range config.KnownAgents {
			if entry.Name == nil || *entry.Name == "" {
				continue
			}
			name := core.NormalizeAgentRef(*entry.Name)
			if parsed, err := core.ParseAgentID(name); err == nil {
				name = parsed.Base
			}
			if name == "" {
				continue
			}
			nicks := normalizeNicks(entry.Nicks, name)
			if len(nicks) == 0 {
				continue
			}
			nameToNicks[name] = appendUnique(nameToNicks[name], nicks)
		}
	}

	candidates := make([]mentionCandidate, 0, len(bases)+1)
	for base := range bases {
		candidates = append(candidates, mentionCandidate{Name: base, Nicks: nameToNicks[base]})
	}
	if _, ok := bases["all"]; !ok {
		candidates = append(candidates, mentionCandidate{Name: "all"})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Name < candidates[j].Name
	})
	return candidates
}

func normalizeNicks(nicks []string, name string) []string {
	if len(nicks) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(nicks))
	for _, nick := range nicks {
		nick = core.NormalizeAgentRef(strings.TrimSpace(nick))
		if nick == "" || nick == name {
			continue
		}
		if _, ok := seen[nick]; ok {
			continue
		}
		seen[nick] = struct{}{}
		out = append(out, nick)
	}
	return out
}

func appendUnique(existing []string, added []string) []string {
	if len(added) == 0 {
		return existing
	}
	seen := map[string]struct{}{}
	for _, value := range existing {
		seen[value] = struct{}{}
	}
	for _, value := range added {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		existing = append(existing, value)
	}
	return existing
}

func matchNickPrefix(nicks []string, prefix string) []string {
	if prefix == "" || len(nicks) == 0 {
		return nil
	}
	out := make([]string, 0, len(nicks))
	for _, nick := range nicks {
		if strings.HasPrefix(strings.ToLower(nick), prefix) {
			out = append(out, nick)
		}
	}
	return out
}

func formatNickList(nicks []string) string {
	formatted := make([]string, 0, len(nicks))
	for _, nick := range nicks {
		if nick == "" {
			continue
		}
		formatted = append(formatted, "@"+nick)
	}
	return strings.Join(formatted, ", ")
}

func (m *Model) buildReplySuggestions(prefix string) ([]suggestionItem, error) {
	normalized := normalizePrefix(prefix)
	if len(normalized) < 2 {
		return nil, nil
	}

	rows, err := m.db.Query(`
		SELECT guid, from_agent, body
		FROM fray_messages
		WHERE guid LIKE ?
		ORDER BY ts DESC, guid DESC
		LIMIT ?
	`, fmt.Sprintf("msg-%s%%", normalized), suggestionLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prefixLength := core.GetDisplayPrefixLength(m.messageCount)
	suggestions := make([]suggestionItem, 0, suggestionLimit)
	for rows.Next() {
		var guid string
		var fromAgent string
		var body string
		if err := rows.Scan(&guid, &fromAgent, &body); err != nil {
			return nil, err
		}
		displayPrefix := core.GetGUIDPrefix(guid, prefixLength)
		preview := truncatePreview(body, 40)
		display := fmt.Sprintf("#%s @%s %s", displayPrefix, fromAgent, preview)
		suggestions = append(suggestions, suggestionItem{
			Display: display,
			Insert:  "#" + guid,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return suggestions, nil
}

// getCommandUsage returns the usage string for a completed command.
func getCommandUsage(cmdName string) string {
	cmdName = strings.ToLower(cmdName)
	for _, cmd := range allCommands {
		if strings.ToLower(cmd.Name) == cmdName && cmd.Usage != "" {
			return fmt.Sprintf("%s %s", cmd.Name, cmd.Usage)
		}
	}
	return ""
}

// buildCommandSuggestions returns command suggestions that match the prefix.
// Prioritizes exact prefix matches, then fuzzy matches.
func buildCommandSuggestions(prefix string) []suggestionItem {
	prefix = strings.ToLower(prefix)
	var prefixMatches []suggestionItem
	var fuzzyMatches []suggestionItem

	for _, cmd := range allCommands {
		// Command name without the leading slash for matching
		cmdName := strings.ToLower(cmd.Name[1:])
		display := fmt.Sprintf("%s  %s", cmd.Name, cmd.Desc)
		item := suggestionItem{Display: display, Insert: cmd.Name}

		if strings.HasPrefix(cmdName, prefix) {
			prefixMatches = append(prefixMatches, item)
		} else if fuzzyMatch(cmdName, prefix) {
			fuzzyMatches = append(fuzzyMatches, item)
		}
	}

	// Combine: prefix matches first, then fuzzy matches
	suggestions := append(prefixMatches, fuzzyMatches...)
	if len(suggestions) > suggestionLimit {
		suggestions = suggestions[:suggestionLimit]
	}
	return suggestions
}

// fuzzyMatch checks if needle characters appear in order within haystack.
// Empty needle matches everything.
func fuzzyMatch(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	hi := 0
	for _, ch := range needle {
		found := false
		for hi < len(haystack) {
			if rune(haystack[hi]) == ch {
				hi++
				found = true
				break
			}
			hi++
		}
		if !found {
			return false
		}
	}
	return true
}
