package chat

import (
	"fmt"
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
)

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
		m.suggestionIndex--
		if m.suggestionIndex < 0 {
			m.suggestionIndex = len(m.suggestions) - 1
		}
		return true, nil
	case tea.KeyDown:
		m.suggestionIndex++
		if m.suggestionIndex >= len(m.suggestions) {
			m.suggestionIndex = 0
		}
		return true, nil
	case tea.KeyTab, tea.KeyEnter:
		if m.suggestionIndex >= 0 && m.suggestionIndex < len(m.suggestions) {
			m.applySuggestion(m.suggestions[m.suggestionIndex])
		}
		return true, nil
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

	if strings.HasPrefix(strings.TrimSpace(value), "/") {
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
	m.suggestionIndex = 0
	m.suggestionStart = start
	m.suggestionKind = kind
	m.resize()
}

func (m *Model) clearSuggestions() {
	m.suggestions = nil
	m.suggestionIndex = 0
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
