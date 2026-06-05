package chat

// XRUSH: added for message options (rewind/fork/edit).
func (m *UserMessageItem) Seq() int {
	return m.message.Seq
}

func (a *AssistantMessageItem) Seq() int {
	return a.message.Seq
}
