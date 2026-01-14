package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/adamavenir/fray/internal/types"
	"github.com/charmbracelet/lipgloss"
)

// parseInteractiveEvent extracts interactive event data from a message body.
// Currently disabled - interactive UI for permissions has been removed.
// Permission requests now display as simple text with commands to run.
func parseInteractiveEvent(msg types.Message) *types.InteractiveEvent {
	// Interactive UI disabled - permission requests show as plain text
	return nil
}

// renderInteractiveEvent formats an interactive event with clickable action buttons.
func (m *Model) renderInteractiveEvent(msg types.Message, event *types.InteractiveEvent, width int) string {
	var b strings.Builder

	// Header with status indicator
	statusIcon := "⏳"
	statusColor := lipgloss.Color("220") // yellow for pending
	if event.Status == "resolved" || event.Status == "approved" {
		statusIcon = "✓"
		statusColor = lipgloss.Color("42") // green
	} else if event.Status == "denied" {
		statusIcon = "✗"
		statusColor = lipgloss.Color("196") // red
	}

	headerStyle := lipgloss.NewStyle().Foreground(statusColor).Bold(true)
	b.WriteString(headerStyle.Render(fmt.Sprintf("%s %s", statusIcon, event.Title)))
	b.WriteString("\n\n")

	// Body text
	bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	b.WriteString(bodyStyle.Render(event.Body))
	b.WriteString("\n\n")

	// Action buttons (only if pending)
	if event.Status == "" || event.Status == "pending" {
		b.WriteString(m.renderActionButtons(msg.ID, event.Actions))
	} else if event.ResolvedBy != nil {
		resolvedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Italic(true)
		b.WriteString(resolvedStyle.Render(fmt.Sprintf("Resolved by @%s", *event.ResolvedBy)))
	}

	return b.String()
}

// renderActionButtons renders clickable action buttons with zone markers.
// Buttons wrap to multiple lines if they don't fit in the available width.
func (m *Model) renderActionButtons(msgID string, actions []types.InteractiveAction) string {
	if len(actions) == 0 {
		return ""
	}

	width := m.mainWidth()
	if width <= 0 {
		width = 80 // fallback
	}

	// Build styled buttons first
	type styledButton struct {
		text    string
		display int // display width (visible chars)
	}
	var buttons []styledButton

	for _, action := range actions {
		zoneID := fmt.Sprintf("action-%s-%s", msgID, action.ID)

		// Style based on action type
		var style lipgloss.Style
		switch action.Style {
		case "primary":
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("27")).
				Padding(0, 1)
		case "danger":
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("160")).
				Padding(0, 1)
		case "muted":
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Padding(0, 1)
		default:
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("240")).
				Padding(0, 1)
		}

		buttonText := style.Render(action.Label)
		markedButton := m.zoneManager.Mark(zoneID, buttonText)
		// Display width: label + 2 chars of padding (1 each side)
		displayWidth := len(action.Label) + 2
		buttons = append(buttons, styledButton{text: markedButton, display: displayWidth})
	}

	// Wrap buttons into lines
	const spacing = 2 // space between buttons
	var lines []string
	var currentLine []string
	currentWidth := 0

	for _, btn := range buttons {
		neededWidth := btn.display
		if len(currentLine) > 0 {
			neededWidth += spacing
		}

		if currentWidth+neededWidth > width && len(currentLine) > 0 {
			// Start new line
			lines = append(lines, strings.Join(currentLine, "  "))
			currentLine = []string{btn.text}
			currentWidth = btn.display
		} else {
			currentLine = append(currentLine, btn.text)
			currentWidth += neededWidth
		}
	}

	// Add remaining buttons
	if len(currentLine) > 0 {
		lines = append(lines, strings.Join(currentLine, "  "))
	}

	return strings.Join(lines, "\n")
}

// pinnedPermissionsHeight is now in model.go to access db package

// renderPinnedPermissions renders a bar at the top for pending permission requests.
// Currently disabled - returns empty string to avoid layout complexity.
// Permission requests are shown inline in the viewport instead.
func (m *Model) renderPinnedPermissions() string {
	// DISABLED: Pinned permissions cause layout complexity.
	// Permission requests are still shown inline in the viewport as styled events.
	return ""
}

// renderPinnedPermissionsOld is the original implementation, kept for reference.
// nolint: unused
func (m *Model) renderPinnedPermissionsOld() string {
	pendingGUIDs := m.getPendingPermissionGUIDs()
	if len(pendingGUIDs) == 0 {
		return ""
	}

	// Find pending permission requests from messages
	var pending []struct {
		msg   types.Message
		event *types.InteractiveEvent
	}

	for _, msg := range m.messages {
		if event := parseInteractiveEvent(msg); event != nil {
			// Check actual status from permissions.jsonl, not embedded status
			if event.Kind == "permission" && pendingGUIDs[event.TargetGUID] {
				pending = append(pending, struct {
					msg   types.Message
					event *types.InteractiveEvent
				}{msg, event})
			}
		}
	}

	if len(pending) == 0 {
		return ""
	}

	width := m.mainWidth()
	if width <= 0 {
		return ""
	}

	var lines []string

	// Style for the pinned bar - use Width to ensure consistent sizing
	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("52")).
		Foreground(lipgloss.Color("231")).
		Bold(true).
		Padding(0, 1).
		Width(width)

	bodyStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("252")).
		Padding(0, 1).
		Width(width)

	buttonRowStyle := lipgloss.NewStyle().
		Width(width)

	separatorStyle := lipgloss.NewStyle().
		Width(width)

	for _, p := range pending {
		// Header line
		header := headerStyle.Render(fmt.Sprintf("⚠️  %s", p.event.Title))

		// Body line (truncate if too long)
		bodyText := strings.ReplaceAll(p.event.Body, "\n", " | ")
		maxLen := width - 10
		if maxLen > 3 && len(bodyText) > maxLen {
			bodyText = bodyText[:maxLen-3] + "..."
		}
		body := bodyStyle.Render(bodyText)

		// Action buttons (using pinned- prefix for zone IDs)
		buttons := buttonRowStyle.Render(m.renderPinnedActionButtons(p.msg.ID, p.event.Actions))

		// Empty separator line
		separator := separatorStyle.Render("")

		lines = append(lines, header, body, buttons, separator)
	}

	return strings.Join(lines, "\n")
}

// renderPinnedActionButtons renders action buttons for pinned permission requests.
// Uses pinned- prefix for zone IDs to distinguish from in-viewport buttons.
func (m *Model) renderPinnedActionButtons(msgID string, actions []types.InteractiveAction) string {
	if len(actions) == 0 {
		return ""
	}

	var parts []string
	for _, action := range actions {
		zoneID := fmt.Sprintf("pinned-action-%s-%s", msgID, action.ID)

		var style lipgloss.Style
		switch action.Style {
		case "primary":
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("27")).
				Padding(0, 1)
		case "danger":
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("231")).
				Background(lipgloss.Color("160")).
				Padding(0, 1)
		case "muted":
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Padding(0, 1)
		default:
			style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("240")).
				Padding(0, 1)
		}

		buttonText := style.Render(action.Label)
		parts = append(parts, m.zoneManager.Mark(zoneID, buttonText))
	}

	return strings.Join(parts, "  ")
}

// executeActionCommand runs a fray command in the background.
func (m *Model) executeActionCommand(command string) {
	// Parse command - expect "fray <subcommand> <args...>"
	parts := strings.Fields(command)
	if len(parts) < 2 {
		return
	}

	// Try to find fray binary - use the same binary that's running
	frayBin := parts[0]
	if exePath, err := os.Executable(); err == nil && frayBin == "fray" {
		frayBin = exePath
	}

	// Execute command
	cmd := exec.Command(frayBin, parts[1:]...)
	cmd.Dir = m.projectRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		// Log error to status (visible in TUI)
		m.status = fmt.Sprintf("Error: %v", err)
		if len(output) > 0 {
			m.status = fmt.Sprintf("Error: %s", strings.TrimSpace(string(output)))
		}
	}
}

// createPermissionEvent creates an InteractiveEvent for a permission request.
func createPermissionEvent(req types.PermissionRequest) types.InteractiveEvent {
	actions := []types.InteractiveAction{
		{
			ID:      "approve-1",
			Label:   "[1] Allow once",
			Command: fmt.Sprintf("fray approve %s 1", req.GUID),
			Style:   "primary",
		},
		{
			ID:      "approve-2",
			Label:   "[2] Allow (session)",
			Command: fmt.Sprintf("fray approve %s 2", req.GUID),
			Style:   "",
		},
		{
			ID:      "approve-3",
			Label:   "[3] Allow (project)",
			Command: fmt.Sprintf("fray approve %s 3", req.GUID),
			Style:   "danger",
			Confirm: true,
		},
		{
			ID:      "deny",
			Label:   "[x] Deny",
			Command: fmt.Sprintf("fray deny %s", req.GUID),
			Style:   "muted",
		},
	}

	status := string(req.Status)
	var resolvedBy *string
	if req.RespondedBy != nil {
		resolvedBy = req.RespondedBy
	}

	return types.InteractiveEvent{
		Kind:       "permission",
		TargetGUID: req.GUID,
		Title:      fmt.Sprintf("Permission Request from @%s", req.FromAgent),
		Body:       fmt.Sprintf("Tool: %s\nAction: %s", req.Tool, req.Action),
		Actions:    actions,
		Status:     status,
		ResolvedBy: resolvedBy,
	}
}

// embedInteractiveEvent serializes an InteractiveEvent into a message body marker.
func embedInteractiveEvent(body string, event types.InteractiveEvent) string {
	data, err := json.Marshal(event)
	if err != nil {
		return body
	}
	return body + fmt.Sprintf("\n<!--interactive:%s-->", string(data))
}
