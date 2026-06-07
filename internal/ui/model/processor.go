package model

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/extensions"
	"github.com/charmbracelet/crush/internal/processor"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
)

// processorPhaseLabel returns a human-readable label for a ProcessorPhase.
func processorPhaseLabel(p processor.ProcessorPhase) string {
	switch p {
	case processor.InputPhase:
		return "input"
	case processor.OutputStreamPhase:
		return "output_stream"
	case processor.OutputResultPhase:
		return "output_result"
	case processor.APIErrorPhase:
		return "api_error"
	default:
		return "idle"
	}
}

// processorStatusIcon returns the appropriate icon for the processor state.
func processorStatusIcon(t *styles.Styles, active bool) string {
	if active {
		return t.Resource.OnlineIcon.String()
	}
	return t.Resource.OfflineIcon.String()
}

// processorInfo renders the processor pipeline debug section showing active
// status, phase, processor names, and token budget. Only visible when
// showProcessorDebug is toggled on.
func (m *UI) processorInfo(width int) string {
	t := m.com.Styles

	title := common.Section(t, t.Resource.Heading.Render("Processor"), width)

	snap := extensions.TheProcessorExtension.GetState()

	if !snap.Active {
		list := t.Resource.AdditionalText.Render("Inactive")
		return lipgloss.NewStyle().Width(width).Render(fmt.Sprintf("%s\n\n%s", title, list))
	}

	var lines []string
	lines = append(lines, common.Status(t, common.StatusOpts{
		Icon:        processorStatusIcon(t, snap.Active),
		Title:       t.Resource.Name.Render("Pipeline"),
		Description: t.Resource.StatusText.Render("active"),
	}, width))

	phaseLabel := processorPhaseLabel(snap.LastPhase)
	if phaseLabel == "idle" {
		phaseLabel = "none"
	}
	lines = append(lines, common.Status(t, common.StatusOpts{
		Icon:        t.Resource.BusyIcon.String(),
		Title:       t.Resource.Name.Render("Last Phase"),
		Description: t.Resource.StatusText.Render(phaseLabel),
	}, width))

	if snap.TokenBudget > 0 {
		lines = append(lines, common.Status(t, common.StatusOpts{
			Icon:        t.Resource.OnlineIcon.String(),
			Title:       t.Resource.Name.Render("Token Budget"),
			Description: t.Resource.StatusText.Render(fmt.Sprintf("%d", snap.TokenBudget)),
		}, width))
	}

	if len(snap.ProcessorNames) > 0 {
		names := strings.Join(snap.ProcessorNames, ", ")
		lines = append(lines, common.Status(t, common.StatusOpts{
			Icon:        t.Resource.OnlineIcon.String(),
			Title:       t.Resource.Name.Render(fmt.Sprintf("Processors (%d)", len(snap.ProcessorNames))),
			Description: t.Resource.StatusText.Render(names),
		}, width))
	}

	list := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.NewStyle().Width(width).Render(fmt.Sprintf("%s\n\n%s", title, list))
}
