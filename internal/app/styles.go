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

// newStyles builds the default palette for the detected background; named
// themes and per-color overrides are layered on by model.applyTheme.
func newStyles(dark bool) *styles {
	def, _ := resolveTheme("auto", dark)
	return def.styles(nil)
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

// frame is the shared panel frame (main list, footer bar): border only, no
// padding, so the content gets the full inner width.
func (s *styles) frame() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(s.accent)
}

// button renders one footer hint's label in the theme accent: no
// background fill, so the text stays readable on the terminal's own
// background. Only the hover state fills.
func (s *styles) button() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.accent)
}

// buttonHover renders the hovered footer button's interior (shadow
// switches to the theme accent).
func (s *styles) buttonHover() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.selFg).Background(s.accent).Bold(true)
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
