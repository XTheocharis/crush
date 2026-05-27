package tools

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"charm.land/fantasy"
)

const (
	SendMessageToolName = "send_message"
)

//go:embed send_message.md
var sendMessageDescription []byte

type SendMessageParams struct {
	AgentName string `json:"agent_name" description:"Name of the target agent to send the message to"`
	Content   string `json:"content" description:"Message content to send"`
}

type SendMessageResponseMetadata struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Timestamp string `json:"timestamp"`
}

func NewSendMessageTool(registry AgentRegistry, mailbox Mailbox) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SendMessageToolName,
		string(sendMessageDescription),
		func(ctx context.Context, params SendMessageParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if params.AgentName == "" {
				return fantasy.NewTextErrorResponse("missing agent_name"), nil
			}
			if params.Content == "" {
				return fantasy.NewTextErrorResponse("missing content"), nil
			}

			sessionID := GetSessionFromContext(ctx)
			senderName := "coder"
			if sessionID != "" {
				senderName = sessionID
			}

			if registry != nil && !registry.HasAgent(params.AgentName) {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("agent %q not found in registry", params.AgentName)), nil
			}

			if mailbox == nil {
				return fantasy.NewTextErrorResponse("mailbox not configured"), nil
			}

			msg := MailboxMessage{
				From:      senderName,
				To:        params.AgentName,
				Content:   params.Content,
				Timestamp: time.Now(),
			}

			if err := mailbox.Send(msg); err != nil {
				return fantasy.NewTextErrorResponse(fmt.Sprintf("failed to send message: %v", err)), nil
			}

			result := fmt.Sprintf("Message sent to agent %q", params.AgentName)
			metadata := SendMessageResponseMetadata{
				From:      msg.From,
				To:        msg.To,
				Timestamp: msg.Timestamp.Format(time.RFC3339),
			}
			return fantasy.WithResponseMetadata(fantasy.NewTextResponse(result), metadata), nil
		},
	)
}
