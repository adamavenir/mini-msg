package chat

import (
	"fmt"
	"time"

	"github.com/adamavenir/fray/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) handleMouseClick(msg tea.MouseMsg) (bool, tea.Cmd) {
	debugLog(fmt.Sprintf("handleMouseClick: action=%v button=%v x=%d y=%d replyToID=%q", msg.Action, msg.Button, msg.X, msg.Y, m.replyToID))
	// Check for reply cancel click first (anywhere on screen)
	if m.replyToID != "" && m.zoneManager.Get("reply-cancel").InBounds(msg) {
		debugLog("handleMouseClick: clearing reply via cancel zone")
		m.clearReplyTo()
		return true, nil
	}

	// Check for pinned permission request button clicks (at top of screen)
	for _, message := range m.messages {
		if event := parseInteractiveEvent(message); event != nil {
			if event.Kind == "permission" && (event.Status == "" || event.Status == "pending") {
				for _, action := range event.Actions {
					zoneID := fmt.Sprintf("pinned-action-%s-%s", message.ID, action.ID)
					if m.zoneManager.Get(zoneID).InBounds(msg) {
						debugLog(fmt.Sprintf("handleMouseClick: pinned button clicked: %s", zoneID))
						if action.Command != "" {
							go m.executeActionCommand(action.Command)
							m.status = fmt.Sprintf("Executing: %s", action.Label)
						}
						return true, nil
					}
				}
			}
		}
	}

	threadWidth := 0
	if m.threadPanelOpen {
		threadWidth = m.threadPanelWidth()
		if msg.X < threadWidth {
			// Check for job cluster zone clicks (activity panel)
			for _, agent := range m.managedAgents {
				if agent.JobID != nil && *agent.JobID != "" {
					zoneID := "job-cluster-" + *agent.JobID
					if m.zoneManager.Get(zoneID).InBounds(msg) {
						// Toggle cluster expansion
						m.expandedJobClusters[*agent.JobID] = !m.expandedJobClusters[*agent.JobID]
						return true, nil
					}
				}
			}

			// Check for agent zone clicks (activity panel at bottom)
			for _, agent := range m.managedAgents {
				zoneID := "agent-" + agent.AgentID
				if m.zoneManager.Get(zoneID).InBounds(msg) {
					// Navigate to agent's last posted thread
					m.navigateToAgentThread(agent.AgentID)
					return true, nil
				}
			}

			if msg.Y < lipgloss.Height(m.renderThreadPanel()) {
				if index := m.threadIndexAtLine(msg.Y); index >= 0 {
					if index == m.threadIndex {
						// Clicking same index
						if m.isPeeking() {
							// If peeking, click confirms selection (gives panel focus)
							m.threadPanelFocus = true
							m.sidebarFocus = false
							m.updateInputFocus()
							m.selectThreadEntry()
						} else {
							// If not peeking (already in this thread), drill in
							m.threadPanelFocus = true
							m.sidebarFocus = false
							m.updateInputFocus()
							m.drillInAction()
						}
					} else {
						// Clicking different index - peek it without grabbing focus
						// This allows the user to keep typing while previewing content
						m.threadIndex = index
						m.peekThreadEntry(peekSourceClick)
					}
					return true, nil
				}
				// Click on header when drilled in -> drill out
				// Header is at Y = pinnedPermissionsHeight (first line after top padding)
				if msg.Y == m.pinnedPermissionsHeight() && m.drillDepth() > 0 {
					m.drillOutAction()
					return true, nil
				}
				// Clicked elsewhere in thread panel - give it focus for navigation
				m.threadPanelFocus = true
				m.sidebarFocus = false
				m.updateInputFocus()
				return true, nil
			}
			// Clicked below content area in thread panel
			m.threadPanelFocus = true
			m.sidebarFocus = false
			m.updateInputFocus()
			return true, nil
		}
	}

	if m.sidebarOpen {
		sidebarStart := threadWidth
		if msg.X >= sidebarStart && msg.X < sidebarStart+m.sidebarWidth() {
			// Clicked in sidebar - give it focus
			m.sidebarFocus = true
			m.threadPanelFocus = false
			m.updateInputFocus()
			if msg.Y < lipgloss.Height(m.renderSidebar()) {
				if index := m.sidebarIndexAtLine(msg.Y); index >= 0 {
					m.channelIndex = index
					return true, m.selectChannelCmd()
				}
				return true, nil
			}
			return true, nil
		}
	}

	// Clicked in main area - focus textarea
	m.threadPanelFocus = false
	m.sidebarFocus = false
	m.updateInputFocus()

	// Account for peek statusline and pinned permissions at top when calculating viewport Y
	topOffset := 0
	if m.isPeeking() {
		topOffset = 1 // peek statusline takes 1 row
	}
	topOffset += m.pinnedPermissionsHeight()
	viewportY := msg.Y - topOffset

	if viewportY < 0 || viewportY >= m.viewport.Height {
		// Clicking outside viewport (peek statusline or textarea area) - just focus input, keep peek state
		return false, nil
	}

	line := m.viewport.YOffset + viewportY

	// Check for subthread zone clicks before checking messages
	if m.currentThread != nil {
		children, _ := db.GetChildThreadsWithStats(m.db, m.currentThread.GUID)
		for _, child := range children {
			zoneID := "subthread-" + child.GUID
			if m.zoneManager.Get(zoneID).InBounds(msg) {
				m.navigateToThread(child.GUID)
				return true, nil
			}
		}
	}

	message, ok := m.messageAtLine(line)
	if !ok || message == nil {
		return ok, nil
	}

	// Check for interactive action button clicks first
	if event := parseInteractiveEvent(*message); event != nil {
		debugLog(fmt.Sprintf("handleMouseClick: message %s is interactive event with %d actions", message.ID, len(event.Actions)))
		for _, action := range event.Actions {
			zoneID := fmt.Sprintf("action-%s-%s", message.ID, action.ID)
			zone := m.zoneManager.Get(zoneID)
			debugLog(fmt.Sprintf("handleMouseClick: checking zone %s, InBounds=%v", zoneID, zone.InBounds(msg)))
			if zone.InBounds(msg) {
				if action.Command != "" {
					debugLog(fmt.Sprintf("handleMouseClick: executing command: %s", action.Command))
					go m.executeActionCommand(action.Command)
					m.status = fmt.Sprintf("Executing: %s", action.Label)
				}
				return true, nil
			}
		}
	}

	now := time.Now()
	isDoubleClick := m.lastClickID == message.ID && now.Sub(m.lastClickAt) <= doubleClickInterval

	// Check if clicked on the GUID zone (footer message ID)
	guidZone := fmt.Sprintf("guid-%s", message.ID)
	clickedOnGUID := m.zoneManager.Get(guidZone).InBounds(msg)

	if isDoubleClick {
		// Clear double-click tracking and cancel any pending single-click
		m.lastClickID = ""
		m.lastClickAt = time.Time{}
		m.pendingClick = nil

		// Commit peek on double-click action
		if m.isPeeking() {
			m.commitPeek()
		}
		if clickedOnGUID {
			// Double-click on footer ID: set reply reference (no clipboard copy)
			m.setReplyTo(*message)
		} else {
			// Double-click elsewhere: copy from zone (text sections)
			m.copyFromZone(msg, *message)
		}
		return true, nil
	}

	// Single-click - use debounce for GUID clicks
	m.lastClickID = message.ID
	m.lastClickAt = now

	if clickedOnGUID {
		// Queue single-click on GUID for debounce (will copy ID after delay)
		m.pendingClick = &pendingClick{
			messageID: message.ID,
			zone:      "guid",
			text:      message.ID,
			timestamp: now,
		}
		// Start debounce timer
		return true, m.clickDebounceCmd(message.ID, now)
	}

	// Non-GUID single-click: handle immediately (no debounce needed)
	if m.isPeeking() {
		// Single-click on message content while peeking: commit peek (switch to this view)
		m.commitPeek()
	}
	// Single-click elsewhere when not peeking: do nothing (wait for possible double-click)
	return true, nil
}
