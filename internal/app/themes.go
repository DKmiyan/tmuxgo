package app

import "charm.land/lipgloss/v2"

// themeDef is a named palette: the six semantic colors as xterm-256 values
// so terminals with less support downsample gracefully.
type themeDef struct {
	dark     bool
	accent   string // selection, titles, active marks
	success  string // active state, success messages
	attached string // attached-session mark
	danger   string // destructive actions, errors
	muted    string // metadata, hints
	selFg    string // foreground on the selected row (accent background)
}

// builtinThemes maps theme name -> palette. "dark" and "light" are the
// original default palettes; "auto" is not a theme, it picks one of them
// from the detected terminal background.
var builtinThemes = map[string]themeDef{
	"dark":  {dark: true, accent: "63", success: "42", attached: "214", danger: "196", muted: "245", selFg: "255"},
	"light": {dark: false, accent: "25", success: "28", attached: "166", danger: "160", muted: "240", selFg: "255"},

	"catppuccin-mocha": {dark: true, accent: "183", success: "151", attached: "216", danger: "211", muted: "243", selFg: "16"},
	"catppuccin-latte": {dark: false, accent: "92", success: "34", attached: "202", danger: "160", muted: "244", selFg: "255"},
	"nord":             {dark: true, accent: "110", success: "144", attached: "222", danger: "131", muted: "242", selFg: "235"},
	"gruvbox-dark":     {dark: true, accent: "109", success: "142", attached: "208", danger: "203", muted: "244", selFg: "235"},
	"gruvbox-light":    {dark: false, accent: "24", success: "64", attached: "166", danger: "124", muted: "243", selFg: "230"},
	"dracula":          {dark: true, accent: "141", success: "84", attached: "215", danger: "203", muted: "61", selFg: "236"},
	"solarized-dark":   {dark: true, accent: "32", success: "100", attached: "166", danger: "160", muted: "240", selFg: "230"},
	"solarized-light":  {dark: false, accent: "32", success: "100", attached: "166", danger: "160", muted: "242", selFg: "230"},
	"tokyo-night":      {dark: true, accent: "111", success: "149", attached: "215", danger: "204", muted: "61", selFg: "234"},
}

// themeCycle orders theme names for the settings screen picker; "auto" is
// prepended by nextTheme.
var themeCycle = []string{
	"dark", "light",
	"catppuccin-mocha", "catppuccin-latte",
	"nord",
	"gruvbox-dark", "gruvbox-light",
	"dracula",
	"solarized-dark", "solarized-light",
	"tokyo-night",
}

// resolveTheme maps a configured theme name to a palette. ""/"auto" follow
// the detected background. An unknown name falls back to the detected
// default and reports ok=false.
func resolveTheme(name string, detectedDark bool) (themeDef, bool) {
	fallback := builtinThemes["light"]
	if detectedDark {
		fallback = builtinThemes["dark"]
	}
	if name == "" || name == "auto" {
		return fallback, true
	}
	if def, ok := builtinThemes[name]; ok {
		return def, true
	}
	return fallback, false
}

// nextTheme returns the theme delta steps after cur in the settings cycle
// (auto, dark, light, then the named themes). An unknown cur counts as auto.
func nextTheme(cur string, delta int) string {
	order := make([]string, 0, len(themeCycle)+1)
	order = append(order, "auto")
	order = append(order, themeCycle...)
	i := 0
	for j, n := range order {
		if n == cur {
			i = j
			break
		}
	}
	return order[(i+delta%len(order)+len(order))%len(order)]
}

// styles builds the semantic palette, applying per-color overrides (keys:
// accent, success, attached, danger, muted, selFg). Unknown keys and empty
// values are ignored.
func (d themeDef) styles(overrides map[string]string) *styles {
	c := map[string]string{
		"accent":   d.accent,
		"success":  d.success,
		"attached": d.attached,
		"danger":   d.danger,
		"muted":    d.muted,
		"selFg":    d.selFg,
	}
	for k, v := range overrides {
		if v == "" {
			continue
		}
		if _, ok := c[k]; ok {
			c[k] = v
		}
	}
	return &styles{
		dark:     d.dark,
		accent:   lipgloss.Color(c["accent"]),
		success:  lipgloss.Color(c["success"]),
		attached: lipgloss.Color(c["attached"]),
		danger:   lipgloss.Color(c["danger"]),
		muted:    lipgloss.Color(c["muted"]),
		selFg:    lipgloss.Color(c["selFg"]),
	}
}
