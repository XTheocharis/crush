package chat

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/anim"
	"github.com/charmbracelet/crush/internal/ui/list"
	"github.com/charmbracelet/crush/internal/ui/styles"
)

const processingItemID = "__processing__"

// ProcessingItem is a chat [MessageItem] that shows an animated gradient
// spinner while the agent performs post-LLM processing (autofix, compaction,
// etc.). It is injected after streaming completes and removed when the agent
// finishes its turn.
type ProcessingItem struct {
	*list.Versioned

	sty  *styles.Styles
	anim *anim.Anim
}

// NewProcessingItem creates a new ProcessingItem.
func NewProcessingItem(sty *styles.Styles) *ProcessingItem {
	p := &ProcessingItem{
		Versioned: list.NewVersioned(),
		sty:       sty,
	}
	p.anim = anim.New(anim.Settings{
		ID:          p.ID(),
		Size:        15,
		GradColorA:  sty.WorkingGradFromColor,
		GradColorB:  sty.WorkingGradToColor,
		LabelColor:  sty.WorkingLabelColor,
		CycleColors: true,
	})
	return p
}

// ID implements [MessageItem].
func (p *ProcessingItem) ID() string { return processingItemID }

// Finished implements [list.Item]. The processing indicator is always
// animating and should never be frozen by the list cache.
func (p *ProcessingItem) Finished() bool { return false }

// StartAnimation implements [Animatable].
func (p *ProcessingItem) StartAnimation() tea.Cmd {
	return p.anim.Start()
}

// Animate implements [Animatable].
func (p *ProcessingItem) Animate(msg anim.StepMsg) tea.Cmd {
	return p.anim.Animate(msg)
}

// RawRender implements [MessageItem].
func (p *ProcessingItem) RawRender(width int) string {
	return fmt.Sprintf("  %s", p.anim.Render())
}

// Render implements [list.Item].
func (p *ProcessingItem) Render(width int) string {
	return p.RawRender(width)
}

// ProcessingItemID returns the fixed ID used by the processing indicator.
func ProcessingItemID() string { return processingItemID }
