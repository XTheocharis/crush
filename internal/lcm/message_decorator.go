package lcm

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/message"
)

// Compile-time check that messageDecorator implements message.Service.
var _ message.Service = (*messageDecorator)(nil)

// previewChars is the number of leading characters included in a large-output
// reference preview.
const previewChars = 2000

// fallbackTruncateChars is the character limit used when LCM storage fails
// and we fall back to inline truncation.
const fallbackTruncateChars = 40000

// messageDecorator wraps message.Service to intercept Create, Update, and List
// with LCM-aware behaviour (large-output storage, token tracking, compaction
// scheduling, and summary injection).
type messageDecorator struct {
	message.Service // embedded inner service; delegates all non-overridden methods
	mgr             Manager
	store           *Store
	querier         db.Querier
	sqlDB           *sql.DB
	initSessions    sync.Map // sessionID -> struct{} (tracks lazily initialized sessions)
}

// NewMessageDecorator wraps svc with LCM-aware behaviour.
func NewMessageDecorator(svc message.Service, mgr Manager, queries *db.Queries, sqlDB *sql.DB) message.Service {
	return &messageDecorator{
		Service: svc,
		mgr:     mgr,
		store:   newStore(queries, sqlDB),
		querier: queries,
		sqlDB:   sqlDB,
	}
}

// ensureSessionInit lazily initializes an LCM session on first access.
func (s *messageDecorator) ensureSessionInit(ctx context.Context, sessionID string) {
	if _, loaded := s.initSessions.LoadOrStore(sessionID, struct{}{}); loaded {
		return
	}
	if err := s.mgr.InitSession(ctx, sessionID); err != nil {
		slog.Warn("Failed to lazily init LCM session", "session_id", sessionID, "error", err)
		s.initSessions.Delete(sessionID)
	}
}

// Create intercepts message creation to:
//  1. Store large tool outputs in LCM and replace them with a reference+preview.
//  2. Delegate to the inner service.
//  3. Assign a monotonic sequence number and persist token counts.
//  4. Schedule async soft-threshold compaction.
func (s *messageDecorator) Create(ctx context.Context, sessionID string, params message.CreateMessageParams) (message.Message, error) {
	s.ensureSessionInit(ctx, sessionID)

	// Step 1: large-output interception for tool messages.
	if params.Role == message.Tool {
		partsText := extractPartsText(params.Parts)
		tokenCount := EstimateTokens(partsText)

		if tokenCount > LargeOutputThreshold {
			fileID, err := s.store.InsertLargeTextContent(ctx, sessionID, partsText, "")
			if err != nil {
				// Storage failed — fall back to deterministic truncation.
				slog.Warn("LCM large-output storage failed, truncating inline",
					"session_id", sessionID,
					"error", err,
				)
				truncated := truncateString(partsText, fallbackTruncateChars)
				suffix := "\n\n[LCM Warning: large output could not be stored; content truncated]"
				params.Parts = []message.ContentPart{
					message.TextContent{Text: truncated + suffix},
				}
			} else {
				preview := truncateString(partsText, previewChars)
				ref := fmt.Sprintf("[Large Tool Output Stored: %s]\nLCM File ID: %s\n\nPreview (first %d chars):\n%s",
					fileID, fileID, previewChars, preview)
				params.Parts = []message.ContentPart{
					message.TextContent{Text: ref},
				}
			}
		}
	}

	// Step 2: delegate to inner service.
	msg, err := s.Service.Create(ctx, sessionID, params)
	if err != nil {
		return message.Message{}, err
	}

	// Step 3: persist token count.
	partsText := extractPartsText(params.Parts)
	tokenCount := EstimateTokens(partsText)
	tcErr := s.querier.UpdateMessageTokenCount(ctx, db.UpdateMessageTokenCountParams{
		TokenCount: tokenCount,
		ID:         msg.ID,
	})
	if tcErr != nil {
		slog.Warn("Failed to update message token count",
			"message_id", msg.ID,
			"error", tcErr,
		)
	}

	// Step 4: insert a context-item row so the compactor can track this message.
	ciErr := s.querier.InsertLcmContextItem(ctx, db.InsertLcmContextItemParams{
		SessionID:  msg.SessionID,
		Position:   -1, // will be rewritten by compactor; -1 means "append"
		ItemType:   "message",
		MessageID:  sql.NullString{String: msg.ID, Valid: true},
		TokenCount: tokenCount,
	})
	if ciErr != nil {
		slog.Warn("Failed to insert LCM context item",
			"message_id", msg.ID,
			"error", ciErr,
		)
	}

	// Step 5: schedule async soft-threshold compaction.
	if s.mgr != nil {
		go func() {
			// Use a detached context so that the compaction can outlive the
			// request that triggered it.
			s.mgr.ScheduleCompaction(context.WithoutCancel(ctx), msg.SessionID)
		}()
	}

	return msg, nil
}

// Update intercepts message updates to refresh token counts when a message is
// finished (i.e. contains a Finish part).
func (s *messageDecorator) Update(ctx context.Context, msg message.Message) error {
	s.ensureSessionInit(ctx, msg.SessionID)

	// Delegate to inner service first.
	err := s.Service.Update(ctx, msg)
	if err != nil {
		return err
	}

	// If the message now has a Finish part, recompute and persist the token count.
	if msg.FinishPart() != nil {
		partsText := extractPartsText(msg.Parts)
		tokenCount := EstimateTokens(partsText)
		tcErr := s.querier.UpdateMessageTokenCount(ctx, db.UpdateMessageTokenCountParams{
			TokenCount: tokenCount,
			ID:         msg.ID,
		})
		if tcErr != nil {
			slog.Warn("Failed to update message token count on finish",
				"message_id", msg.ID,
				"error", tcErr,
			)
		}
	}

	return nil
}

// List intercepts message listing to inject synthetic summary messages when
// the session has LCM summaries.
func (s *messageDecorator) List(ctx context.Context, sessionID string) ([]message.Message, error) {
	s.ensureSessionInit(ctx, sessionID)

	entries, err := s.store.GetContextEntries(ctx, sessionID)
	if err != nil {
		slog.Warn("Failed to get LCM context entries, falling back to inner List",
			"session_id", sessionID,
			"error", err,
		)
		return s.Service.List(ctx, sessionID)
	}

	// If there are no summaries, fall through to the inner service.
	hasSummary := false
	for _, entry := range entries {
		if entry.ItemType == "summary" {
			hasSummary = true
			break
		}
	}
	if !hasSummary {
		return s.Service.List(ctx, sessionID)
	}

	// Fetch all messages from the inner service so we can look up by ID.
	allMessages, err := s.Service.List(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	msgByID := make(map[string]message.Message, len(allMessages))
	for _, m := range allMessages {
		msgByID[m.ID] = m
	}

	// Build the set of message IDs that appear in context entries so we can
	// include them in order.
	contextMsgIDs := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.ItemType == "message" && entry.MessageID != "" {
			contextMsgIDs[entry.MessageID] = struct{}{}
		}
	}

	// Rebuild the list following the context-entry ordering. Summaries become
	// synthetic messages; message entries are looked up from the real set.
	result := make([]message.Message, 0, len(entries))
	for _, entry := range entries {
		switch entry.ItemType {
		case "summary":
			content := formatSummaryContent(entry)
			synthetic := message.Message{
				ID:               entry.SummaryID,
				Role:             message.User,
				Parts:            []message.ContentPart{message.TextContent{Text: content}},
				SessionID:        sessionID,
				IsSummaryMessage: true,
			}
			result = append(result, synthetic)
		case "message":
			if m, ok := msgByID[entry.MessageID]; ok {
				result = append(result, m)
			}
		}
	}

	// Append any messages that are not yet tracked by context entries (e.g.
	// very recent messages added after the last compaction). This ensures the
	// tail of the conversation is always visible.
	for _, m := range allMessages {
		if _, tracked := contextMsgIDs[m.ID]; !tracked {
			result = append(result, m)
		}
	}

	return result, nil
}

// extractPartsText extracts all plain-text content from a slice of
// message.ContentPart, concatenating TextContent.Text and
// ToolResult.Content fields.
func extractPartsText(parts []message.ContentPart) string {
	var sb strings.Builder
	for _, part := range parts {
		switch p := part.(type) {
		case message.TextContent:
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(p.Text)
		case message.ToolResult:
			if p.Content != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(p.Content)
			}
		}
	}
	return sb.String()
}

// truncateString truncates s to at most maxChars runes.
func truncateString(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}
