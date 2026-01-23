package command

import (
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	approvalsHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("111"))
	approvalsToolStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	approvalsActionStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	approvalsOptionStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("157"))
	approvalsWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	approvalsMetaStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	approvalsHelpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	approvalsSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	approvalsDeniedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	approvalsSkippedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
)

// approvalResult tracks what happened to each permission.
type approvalResult struct {
	perm        types.PermissionRequest
	approved    bool
	denied      bool
	skipped     bool
	chosenIndex int
}

// runApprovalsSession runs the interactive TUI for reviewing permissions.
func runApprovalsSession(projectRoot string, pending []types.PermissionRequest) error {
	model := newApprovalsModel(projectRoot, pending)
	return runApprovalsTUI(model)
}

// formatApprovalAge returns a human-readable age string.
func formatApprovalAge(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

type approvalsPhase int

const (
	approvalsPhaseReviewing approvalsPhase = iota
	approvalsPhaseDone
)

type approvalsModel struct {
	projectRoot string
	pending     []types.PermissionRequest
	current     int
	phase       approvalsPhase
	results     []approvalResult
	width       int
	height      int
	quitting    bool
	err         error
}

func newApprovalsModel(projectRoot string, pending []types.PermissionRequest) approvalsModel {
	return approvalsModel{
		projectRoot: projectRoot,
		pending:     pending,
		results:     make([]approvalResult, 0, len(pending)),
	}
}

func (m approvalsModel) Init() tea.Cmd {
	return nil
}

func (m approvalsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.phase == approvalsPhaseReviewing {
			return m.handleInput(msg)
		}
	}

	return m, nil
}

func (m approvalsModel) handleInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.current >= len(m.pending) {
		m.phase = approvalsPhaseDone
		return m, tea.Quit
	}

	perm := m.pending[m.current]

	switch msg.Type {
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit

	case tea.KeyRunes:
		key := string(msg.Runes)
		switch key {
		case "1", "2", "3":
			idx := int(key[0] - '1')
			if idx < len(perm.Options) {
				return m.approveWithOption(perm, idx)
			}
		case "s", "S":
			return m.skipCurrent(perm)
		case "d", "D":
			return m.denyCurrent(perm)
		case "q", "Q":
			m.quitting = true
			return m, tea.Quit
		}

	case tea.KeyEsc:
		return m.skipCurrent(perm)
	}

	return m, nil
}

func (m approvalsModel) approveWithOption(perm types.PermissionRequest, optionIdx int) (tea.Model, tea.Cmd) {
	now := time.Now().Unix()
	respondedBy := "user"
	update := db.PermissionUpdateJSONLRecord{
		GUID:        perm.GUID,
		Status:      string(types.PermissionStatusApproved),
		ChosenIndex: &optionIdx,
		RespondedBy: respondedBy,
		RespondedAt: now,
	}

	if err := db.AppendPermissionUpdate(m.projectRoot, update); err != nil {
		m.err = fmt.Errorf("approve permission: %w", err)
		return m, tea.Quit
	}

	// If scope is project, add to settings
	if optionIdx < len(perm.Options) && perm.Options[optionIdx].Scope == types.PermissionScopeProject {
		if err := addPermissionsToSettings(m.projectRoot, perm.Options[optionIdx].Patterns); err != nil {
			// Non-fatal: log but continue
		}
	}

	m.results = append(m.results, approvalResult{
		perm:        perm,
		approved:    true,
		chosenIndex: optionIdx,
	})

	return m.advance()
}

func (m approvalsModel) denyCurrent(perm types.PermissionRequest) (tea.Model, tea.Cmd) {
	now := time.Now().Unix()
	respondedBy := "user"
	update := db.PermissionUpdateJSONLRecord{
		GUID:        perm.GUID,
		Status:      string(types.PermissionStatusDenied),
		RespondedBy: respondedBy,
		RespondedAt: now,
	}

	if err := db.AppendPermissionUpdate(m.projectRoot, update); err != nil {
		m.err = fmt.Errorf("deny permission: %w", err)
		return m, tea.Quit
	}

	m.results = append(m.results, approvalResult{
		perm:   perm,
		denied: true,
	})

	return m.advance()
}

func (m approvalsModel) skipCurrent(perm types.PermissionRequest) (tea.Model, tea.Cmd) {
	m.results = append(m.results, approvalResult{
		perm:    perm,
		skipped: true,
	})
	return m.advance()
}

func (m approvalsModel) advance() (tea.Model, tea.Cmd) {
	m.current++
	if m.current >= len(m.pending) {
		m.phase = approvalsPhaseDone
		return m, tea.Quit
	}
	return m, nil
}

func (m approvalsModel) View() string {
	if m.quitting {
		return ""
	}

	switch m.phase {
	case approvalsPhaseReviewing:
		return m.renderPermission()
	case approvalsPhaseDone:
		return ""
	}
	return ""
}

func (m approvalsModel) renderPermission() string {
	if m.current >= len(m.pending) {
		return ""
	}

	perm := m.pending[m.current]
	var b strings.Builder

	// Progress header
	progress := fmt.Sprintf("Permission %d/%d", m.current+1, len(m.pending))
	b.WriteString(approvalsHeaderStyle.Render(progress))
	b.WriteString("\n\n")

	// Request metadata
	agentLine := fmt.Sprintf("From: @%s", perm.FromAgent)
	if perm.SessionID != "" {
		agentLine += fmt.Sprintf("  (session: %s...)", perm.SessionID[:8])
	}
	b.WriteString(approvalsMetaStyle.Render(agentLine))
	b.WriteString("\n")

	age := time.Now().Unix() - perm.CreatedAt
	b.WriteString(approvalsMetaStyle.Render(formatApprovalAge(age)))
	b.WriteString("\n\n")

	// Tool and Action
	b.WriteString(approvalsToolStyle.Render(perm.Tool))
	b.WriteString("\n")

	// Action (wrapped if needed)
	actionStyle := approvalsActionStyle
	if m.width > 4 {
		actionStyle = actionStyle.Width(m.width - 4)
	}
	b.WriteString(actionStyle.Render(perm.Action))
	b.WriteString("\n\n")

	// Options
	b.WriteString("Options:\n")
	for i, opt := range perm.Options {
		label := fmt.Sprintf("[%d] %s", i+1, opt.Label)
		b.WriteString(fmt.Sprintf("  %s\n", approvalsOptionStyle.Render(label)))

		if opt.Warning != "" {
			b.WriteString(fmt.Sprintf("      %s\n", approvalsWarningStyle.Render("⚠️  "+opt.Warning)))
		}
		if opt.Note != "" {
			b.WriteString(fmt.Sprintf("      %s\n", approvalsMetaStyle.Render(opt.Note)))
		}
	}
	b.WriteString("\n")

	// Help
	helpText := "[1-3] approve  [d]eny  [s]kip  [q]uit"
	b.WriteString(approvalsHelpStyle.Render(helpText))
	b.WriteString("\n")

	return b.String()
}

func runApprovalsTUI(model approvalsModel) error {
	program := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := program.Run()
	if err != nil {
		return err
	}

	m := finalModel.(approvalsModel)
	if m.err != nil {
		return m.err
	}

	// Print summary
	printApprovalsSummary(m.results)
	return nil
}

func printApprovalsSummary(results []approvalResult) {
	if len(results) == 0 {
		fmt.Println("\nNo permissions reviewed.")
		return
	}

	var approved, denied, skipped int
	for _, r := range results {
		switch {
		case r.approved:
			approved++
		case r.denied:
			denied++
		case r.skipped:
			skipped++
		}
	}

	fmt.Println()
	if approved > 0 {
		fmt.Println(approvalsSuccessStyle.Render(fmt.Sprintf("✓ Approved: %d", approved)))
	}
	if denied > 0 {
		fmt.Println(approvalsDeniedStyle.Render(fmt.Sprintf("✗ Denied: %d", denied)))
	}
	if skipped > 0 {
		fmt.Println(approvalsSkippedStyle.Render(fmt.Sprintf("- Skipped: %d", skipped)))
	}
}

