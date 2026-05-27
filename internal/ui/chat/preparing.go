package chat

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/anim"
	"github.com/charmbracelet/crush/internal/ui/list"
	"github.com/charmbracelet/crush/internal/ui/styles"
)

const preparingItemID = "__preparing__"

// PreparingItem is a chat [MessageItem] that shows an animated gradient
// spinner while the agent prepares context before streaming begins. It is
// injected into the chat list immediately on Enter and removed when the first
// assistant message arrives.
type PreparingItem struct {
	*list.Versioned

	sty  *styles.Styles
	anim *anim.Anim
}

// NewPreparingItem creates a new PreparingItem.
func NewPreparingItem(sty *styles.Styles) *PreparingItem {
	p := &PreparingItem{
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
func (p *PreparingItem) ID() string { return preparingItemID }

// Finished implements [list.Item]. The preparing indicator is always
// animating and should never be frozen by the list cache.
func (p *PreparingItem) Finished() bool { return false }

// StartAnimation implements [Animatable].
func (p *PreparingItem) StartAnimation() tea.Cmd {
	return p.anim.Start()
}

// Animate implements [Animatable].
func (p *PreparingItem) Animate(msg anim.StepMsg) tea.Cmd {
	return p.anim.Animate(msg)
}

// RawRender implements [MessageItem].
func (p *PreparingItem) RawRender(width int) string {
	return fmt.Sprintf("  %s", p.anim.Render())
}

// Render implements [list.Item].
func (p *PreparingItem) Render(width int) string {
	return p.RawRender(width)
}

// PreparingItemID returns the fixed ID used by the preparing indicator.
func PreparingItemID() string { return preparingItemID }
