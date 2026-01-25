package chat

import (
	"fmt"

	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	tea "github.com/charmbracelet/bubbletea"
)

type errMsg struct {
	err error
}

func (m *Model) handleErrMsg(msg errMsg) (tea.Model, tea.Cmd) {
	m.status = msg.err.Error()
	return m, m.pollCmd()
}

func (m *Model) handleEditResultMsg(msg editResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.status = fmt.Sprintf("Edit failed: %v", msg.err)
		return m, nil
	}
	if msg.msg != nil {
		reason := "edit"
		if err := m.appendMessageEditUpdate(*msg.msg, reason); err != nil {
			m.status = fmt.Sprintf("Edit failed: %v", err)
			return m, nil
		}
		annotated, err := db.ApplyMessageEditCounts(m.projectDBPath, []types.Message{*msg.msg})
		if err == nil && len(annotated) > 0 {
			*msg.msg = annotated[0]
		}
		m.applyMessageUpdate(*msg.msg)
		m.refreshViewport(false)
		m.status = fmt.Sprintf("Edited #%s", msg.msg.ID)
	}
	return m, nil
}

func (m *Model) handleClickDebounceMsg(msg clickDebounceMsg) (tea.Model, tea.Cmd) {
	// Execute pending single-click if it matches and hasn't been superseded
	if m.pendingClick != nil &&
		m.pendingClick.messageID == msg.messageID &&
		m.pendingClick.timestamp == msg.timestamp {
		m.executePendingClick()
	}
	return m, nil
}
