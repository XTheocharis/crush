package agent

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/charmbracelet/crush/internal/agent/tools"
	"github.com/charmbracelet/crush/internal/message"
)

const (
	DefaultForkedMaxTurns = 10

	DefaultMailboxCapacity = 64
)

type MailboxMessage = tools.MailboxMessage

// Mailbox provides thread-safe, channel-based message passing between agents.
type Mailbox struct {
	mu       sync.RWMutex
	capacity int
	inboxes  map[string]chan MailboxMessage
}

func NewMailbox(capacity int) *Mailbox {
	if capacity <= 0 {
		capacity = DefaultMailboxCapacity
	}
	return &Mailbox{
		capacity: capacity,
		inboxes:  make(map[string]chan MailboxMessage),
	}
}

func (m *Mailbox) Register(name string) <-chan MailboxMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ch, exists := m.inboxes[name]; exists {
		return ch
	}
	ch := make(chan MailboxMessage, m.capacity)
	m.inboxes[name] = ch
	return ch
}

func (m *Mailbox) Unregister(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ch, exists := m.inboxes[name]; exists {
		delete(m.inboxes, name)
		close(ch)
	}
}

func (m *Mailbox) Send(msg MailboxMessage) error {
	m.mu.RLock()
	ch, exists := m.inboxes[msg.To]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("mailbox: agent %q not registered", msg.To)
	}

	select {
	case ch <- msg:
		return nil
	default:
		return fmt.Errorf("mailbox: inbox full for agent %q", msg.To)
	}
}

func (m *Mailbox) HasInbox(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.inboxes[name]
	return ok
}

func (m *Mailbox) Broadcast(msg MailboxMessage, exclude string) []error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errs []error
	for name, ch := range m.inboxes {
		if name == exclude {
			continue
		}
		toMsg := msg
		toMsg.To = name
		select {
		case ch <- toMsg:
		default:
			errs = append(errs, fmt.Errorf("mailbox: inbox full for agent %q", name))
		}
	}
	return errs
}

// AgentRegistry tracks named ForkedAgents for orchestration operations.
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*ForkedAgent
}

func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*ForkedAgent),
	}
}

func (r *AgentRegistry) Register(name string, agent *ForkedAgent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[name] = agent
}

func (r *AgentRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, name)
}

func (r *AgentRegistry) Get(name string) (tools.AgentHandle, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[name]
	return a, ok
}

func (r *AgentRegistry) GetForked(name string) (*ForkedAgent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.agents[name]
	return a, ok
}

func (r *AgentRegistry) HasAgent(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.agents[name]
	return ok
}

func (r *AgentRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	return names
}

// ForkedAgentParams controls per-turn execution behaviour for a ForkedAgent.
type ForkedAgentParams struct {
	CacheSafeParams fantasy.ProviderOptions

	// CanUseTool returns true if the named tool is permitted for this turn.
	// When nil, all tools are allowed.
	CanUseTool func(toolName string) bool

	// MaxOutputTokens caps the number of output tokens for the LLM response.
	// Zero means the model default.
	MaxOutputTokens int64
}

// ForkedAgentOptions configures a new ForkedAgent.
type ForkedAgentOptions struct {
	Name string

	// ParentMessages are the messages inherited from the parent agent.
	// These are deep-copied during construction.
	ParentMessages []message.Message

	SystemPrompt string

	SharedCache *SharedCache

	Mailbox *Mailbox

	Registry *AgentRegistry

	// MaxTurns limits the number of turns this agent may execute. Zero means
	// DefaultForkedMaxTurns.
	MaxTurns int

	Config AgentConfig

	Model Model

	Tools []fantasy.AgentTool

	// Params holds per-turn execution parameters. When nil,
	// ForkedAgentParams defaults are used.
	Params ForkedAgentParams
}

// ForkedAgent inherits parent context (messages and system prompt), shares a
// prompt cache, and has a maxTurns limit.
type ForkedAgent struct {
	name           string
	messages       []message.Message
	systemPrompt   string
	cache          *SharedCache
	mailbox        *Mailbox
	registry       *AgentRegistry
	maxTurns       int
	config         AgentConfig
	model          Model
	agentTools     []fantasy.AgentTool
	params         ForkedAgentParams
	inbox          <-chan MailboxMessage
	cancel         context.CancelFunc
	mu             sync.Mutex
	running        bool
	turnsCompleted int
	runningCh      chan struct{}
}

// NewForkedAgent creates a ForkedAgent that inherits parent context. Parent
// messages are deep-copied so the child does not share mutable state with the
// parent.
func NewForkedAgent(opts ForkedAgentOptions) (*ForkedAgent, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("forked agent: name is required")
	}

	maxTurns := opts.MaxTurns
	if maxTurns <= 0 {
		maxTurns = DefaultForkedMaxTurns
	}

	copied := make([]message.Message, len(opts.ParentMessages))
	for i, msg := range opts.ParentMessages {
		copied[i] = deepCopyMessage(msg)
	}

	fa := &ForkedAgent{
		name:         opts.Name,
		messages:     copied,
		systemPrompt: opts.SystemPrompt,
		cache:        opts.SharedCache,
		mailbox:      opts.Mailbox,
		registry:     opts.Registry,
		maxTurns:     maxTurns,
		config:       opts.Config,
		model:        opts.Model,
		agentTools:   opts.Tools,
		params:       opts.Params,
		runningCh:    make(chan struct{}),
	}

	if fa.mailbox != nil {
		fa.inbox = fa.mailbox.Register(opts.Name)
	}
	if fa.registry != nil {
		fa.registry.Register(opts.Name, fa)
	}

	return fa, nil
}

func (fa *ForkedAgent) Name() string {
	return fa.name
}

func (fa *ForkedAgent) MaxTurns() int {
	return fa.maxTurns
}

func (fa *ForkedAgent) TurnsCompleted() int {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	return fa.turnsCompleted
}

func (fa *ForkedAgent) IsRunning() bool {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	return fa.running
}

// WaitRunning blocks until the agent's Run method has set running=true,
// or returns immediately if already running. Returns false if the context
// expires before the agent starts.
func (fa *ForkedAgent) WaitRunning(ctx context.Context) bool {
	fa.mu.Lock()
	ch := fa.runningCh
	running := fa.running
	fa.mu.Unlock()
	if running {
		return true
	}
	select {
	case <-ch:
		return true
	case <-ctx.Done():
		return false
	}
}

func (fa *ForkedAgent) Messages() []message.Message {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	out := make([]message.Message, len(fa.messages))
	for i, msg := range fa.messages {
		out[i] = deepCopyMessage(msg)
	}
	return out
}

func (fa *ForkedAgent) SystemPrompt() string {
	return fa.systemPrompt
}

func (fa *ForkedAgent) Inbox() <-chan MailboxMessage {
	return fa.inbox
}

func (fa *ForkedAgent) SendMessage(ctx context.Context, to, content string) error {
	if fa.mailbox == nil {
		return fmt.Errorf("forked agent %q: no mailbox configured", fa.name)
	}
	return fa.mailbox.Send(MailboxMessage{
		From:      fa.name,
		To:        to,
		Content:   content,
		Timestamp: time.Now(),
	})
}

func (fa *ForkedAgent) ReceiveMessage(ctx context.Context) (MailboxMessage, error) {
	if fa.inbox == nil {
		return MailboxMessage{}, fmt.Errorf("forked agent %q: no inbox", fa.name)
	}
	select {
	case msg, ok := <-fa.inbox:
		if !ok {
			return MailboxMessage{}, fmt.Errorf("forked agent %q: inbox closed", fa.name)
		}
		return msg, nil
	case <-ctx.Done():
		return MailboxMessage{}, ctx.Err()
	}
}

// Run executes the forked agent's turn loop. Each iteration checks for mailbox
// messages and processes one turn. The agent stops when maxTurns is reached or
// the context is cancelled.
func (fa *ForkedAgent) Run(ctx context.Context) error {
	fa.mu.Lock()
	if fa.running {
		fa.mu.Unlock()
		return fmt.Errorf("forked agent %q: already running", fa.name)
	}
	fa.running = true
	fa.mu.Unlock()
	close(fa.runningCh)

	runCtx, cancel := context.WithCancel(ctx)
	fa.cancel = cancel

	defer func() {
		cancel()
		fa.mu.Lock()
		fa.running = false
		fa.runningCh = make(chan struct{})
		fa.mu.Unlock()
	}()

	for {
		fa.mu.Lock()
		turns := fa.turnsCompleted
		fa.mu.Unlock()

		if turns >= fa.maxTurns {
			return nil
		}

		select {
		case <-runCtx.Done():
			return runCtx.Err()
		default:
		}

		var turnPrompt string
		if fa.inbox != nil {
			select {
			case msg, ok := <-fa.inbox:
				if !ok {
					return nil
				}
				turnPrompt = msg.Content
			case <-runCtx.Done():
				return runCtx.Err()
			default:
				runtime.Gosched()
				continue
			}
		}

		if turnPrompt == "" {
			runtime.Gosched()
			continue
		}

		if err := fa.executeTurn(runCtx, turnPrompt); err != nil {
			return err
		}

		fa.mu.Lock()
		fa.turnsCompleted++
		fa.mu.Unlock()
	}
}

func (fa *ForkedAgent) executeTurn(ctx context.Context, prompt string) error {
	agentTools := fa.filteredTools()

	faAgent := fantasy.NewAgent(
		fa.model.Model,
		fantasy.WithSystemPrompt(fa.systemPrompt),
		fantasy.WithTools(agentTools...),
	)

	history := fa.messagesToHistory()
	var maxTokens *int64
	if fa.params.MaxOutputTokens > 0 {
		maxTokens = &fa.params.MaxOutputTokens
	}

	result, err := faAgent.Stream(ctx, fantasy.AgentStreamCall{
		Prompt:          prompt,
		Messages:        history,
		MaxOutputTokens: maxTokens,
		ProviderOptions: fa.params.CacheSafeParams,
	})
	if err != nil {
		return fmt.Errorf("forked agent %q: LLM call failed: %w", fa.name, err)
	}

	fa.mu.Lock()
	fa.messages = append(fa.messages, message.Message{
		Role: message.User,
		Parts: []message.ContentPart{
			message.TextContent{Text: prompt},
		},
	})
	if result != nil && result.Response.Content != nil {
		fa.messages = append(fa.messages, message.Message{
			Role: message.Assistant,
			Parts: []message.ContentPart{
				message.TextContent{Text: result.Response.Content.Text()},
			},
		})
	}
	fa.mu.Unlock()

	if fa.mailbox != nil && result != nil && result.Response.Content != nil {
		responseText := result.Response.Content.Text()
		if responseText != "" {
			// XRUSH: log error before discarding
			if err := fa.mailbox.Send(MailboxMessage{
				From:      fa.name,
				To:        "parent",
				Content:   responseText,
				Timestamp: time.Now(),
			}); err != nil {
				slog.Warn("ForkedAgent: failed to send mailbox message", "error", err)
			}
		}
	}

	return nil
}

func (fa *ForkedAgent) filteredTools() []fantasy.AgentTool {
	if fa.params.CanUseTool == nil || len(fa.agentTools) == 0 {
		return fa.agentTools
	}
	var filtered []fantasy.AgentTool
	for _, tool := range fa.agentTools {
		if fa.params.CanUseTool(tool.Info().Name) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func (fa *ForkedAgent) messagesToHistory() []fantasy.Message {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	var history []fantasy.Message
	for _, msg := range fa.messages {
		history = append(history, msg.ToAIMessage()...)
	}
	return history
}

// Stop cancels the running agent. It is safe to call multiple times.
func (fa *ForkedAgent) Stop() {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if fa.cancel != nil {
		fa.cancel()
	}
}

// Close unregisters the agent from the mailbox and registry, releasing
// resources.
func (fa *ForkedAgent) Close() {
	if fa.mailbox != nil {
		fa.mailbox.Unregister(fa.name)
	}
	if fa.registry != nil {
		fa.registry.Unregister(fa.name)
	}
}

func deepCopyMessage(msg message.Message) message.Message {
	return msg.Clone()
}

var (
	_ interface {
		Name() string
		Run(context.Context) error
		Stop()
		Close()
	} = (*ForkedAgent)(nil)

	_ tools.AgentHandle   = (*ForkedAgent)(nil)
	_ tools.AgentRegistry = (*AgentRegistry)(nil)
)
