package app

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// styles holds the semantic palette and derived text styles. Colors use the
// 256-color palette so terminals with less support downsample gracefully.
type styles struct {
	dark bool

	accent   color.Color // selection, titles, active marks
	success  color.Color // active state, success messages
	attached color.Color // attached-session mark
	danger   color.Color // destructive actions, errors
	muted    color.Color // metadata, hints
	selFg    color.Color // foreground on the selected row
}

func newStyles(dark bool) *styles {
	if dark {
		return &styles{
			dark:     true,
			accent:   lipgloss.Color("63"),
			success:  lipgloss.Color("42"),
			attached: lipgloss.Color("214"),
			danger:   lipgloss.Color("196"),
			muted:    lipgloss.Color("245"),
			selFg:    lipgloss.Color("255"),
		}
	}
	return &styles{
		accent:   lipgloss.Color("25"),
		success:  lipgloss.Color("28"),
		attached: lipgloss.Color("166"),
		danger:   lipgloss.Color("160"),
		muted:    lipgloss.Color("240"),
		selFg:    lipgloss.Color("255"),
	}
}

func (s *styles) title() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.accent).Bold(true)
}

func (s *styles) selected(w int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.selFg).Background(s.accent).Width(w).MaxWidth(w)
}

// dropTarget highlights the hovered drop row during a drag.
func (s *styles) dropTarget(w int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.selFg).Background(s.attached).Width(w).MaxWidth(w)
}

func (s *styles) meta() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.muted)
}

func (s *styles) ok() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.success)
}

func (s *styles) dangerText() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.danger).Bold(true)
}

func (s *styles) activeDot() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.success)
}

func (s *styles) attachedDot() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.attached)
}

func (s *styles) idleDot() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.muted)
}

func (s *styles) box() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.accent).
		Padding(0, 2)
}

func (s *styles) dangerBox() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.danger).
		Padding(0, 2)
}

// trunc clips s to at most w display cells, ANSI-aware.
func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}
