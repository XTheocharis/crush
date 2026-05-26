package ext

// ExtensionEvent represents an event published by an extension.
type ExtensionEvent struct {
	Source    string `json:"source"`
	EventType string `json:"event_type"`
	Payload   any    `json:"payload"`
}
