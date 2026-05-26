package dialog

import "github.com/charmbracelet/crush/internal/rewind"

type (
	// ActionRewind is a message to rewind a session to a specific sequence.
	ActionRewind struct {
		SessionID string
		Seq       int
		Mode      rewind.RewindMode
	}
	// ActionFork is a message to fork a session at a specific sequence.
	ActionFork struct {
		SessionID string
		Seq       int
	}
	// ActionEditMessage is a message to edit a specific message.
	ActionEditMessage struct {
		SessionID string
		Seq       int
		MessageID string
	}
	// ActionOpenMessageOptions is a message to open the message options dialog.
	ActionOpenMessageOptions struct {
		SessionID string
		Seq       int
		MessageID string
	}
)
