package tools

import (
	"context"
	"time"
)

// MailboxMessage is a message sent between agents via the mailbox system.
type MailboxMessage struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Content   string    `json:"content"`
	Type      string    `json:"type,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

const (
	// MailboxMessageDefault is the default message type for agent-to-agent
	// communication.
	MailboxMessageDefault = ""
	// MailboxMessageSiblingError is sent when a sibling parallel branch fails.
	// Recipients can use this to cooperatively abort early.
	MailboxMessageSiblingError = "sibling_error"
)

// Mailbox is the interface for sending messages between agents.
type Mailbox interface {
	Send(msg MailboxMessage) error
	HasInbox(name string) bool
	Broadcast(msg MailboxMessage, exclude string) []error
}

// AgentHandle is the interface for interacting with a managed agent.
type AgentHandle interface {
	Name() string
	IsRunning() bool
	Stop()
	Close()
}

// AgentRegistry is the interface for discovering and accessing agents.
type AgentRegistry interface {
	Get(name string) (AgentHandle, bool)
	HasAgent(name string) bool
	List() []string
}

// TeamManager tracks team→agent membership for team creation and deletion.
type TeamManager interface {
	// CreateTeam persists a team→agents mapping. Overwrites any existing
	// team with the same name.
	CreateTeam(name string, agentIDs []string)
	// DeleteTeam removes the team entry and returns the previously tracked
	// agent IDs. Returns nil if the team does not exist.
	DeleteTeam(name string) []string
	// GetTeamAgents returns the agent IDs belonging to a team.
	GetTeamAgents(name string) ([]string, bool)
}

// ForkedMessenger is the interface for sending messages from a forked agent.
type ForkedMessenger interface {
	SendMessage(ctx context.Context, to, content string) error
}
