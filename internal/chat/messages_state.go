package chat

func (m *Model) removeMessageByID(id string) bool {
	for i, msg := range m.messages {
		if msg.ID != id {
			continue
		}
		m.messages = append(m.messages[:i], m.messages[i+1:]...)
		return true
	}
	return false
}
