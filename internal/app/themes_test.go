package app

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestEveryBuiltinThemeRenders(t *testing.T) {
	for name, def := range builtinThemes {
		sty := def.styles(nil)
		if sty.dark != def.dark {
			t.Errorf("%s: styles.dark = %v, want %v", name, sty.dark, def.dark)
		}
		for _, render := range []string{
			sty.title().Render("title"),
			sty.selected(20).Render("row"),
			sty.meta().Render("meta"),
			sty.ok().Render("ok"),
			sty.dangerText().Render("danger"),
			sty.box().Render("box"),
		} {
			if strings.TrimSpace(render) == "" {
				t.Errorf("%s: empty render", name)
			}
		}
	}
}

func TestResolveTheme(t *testing.T) {
	// auto follows the detected background
	def, ok := resolveTheme("auto", true)
	if !ok || !def.dark {
		t.Fatal("auto on dark background must resolve to a dark theme")
	}
	def, ok = resolveTheme("", false)
	if !ok || def.dark {
		t.Fatal("empty theme on light background must resolve to a light theme")
	}
	// named themes resolve to themselves
	def, ok = resolveTheme("nord", true)
	if !ok || def != builtinThemes["nord"] {
		t.Fatal("nord must resolve to the nord palette")
	}
	// unknown names fall back to the detected default and report it
	def, ok = resolveTheme("bogus", true)
	if ok || def != builtinThemes["dark"] {
		t.Fatal("unknown theme must fall back to default-dark and report !ok")
	}
	def, ok = resolveTheme("bogus", false)
	if ok || def != builtinThemes["light"] {
		t.Fatal("unknown theme must fall back to default-light and report !ok")
	}
}

func TestThemeColorOverrides(t *testing.T) {
	def := builtinThemes["dark"]
	sty := def.styles(map[string]string{
		"accent": "1",
		"bogus":  "2", // unknown key ignored
		"muted":  "",  // empty value ignored
	})
	if sty.accent != lipgloss.Color("1") {
		t.Fatalf("accent override = %v, want 1", sty.accent)
	}
	if sty.muted != lipgloss.Color(def.muted) {
		t.Fatalf("empty override must keep theme muted %s", def.muted)
	}
	if sty.success != lipgloss.Color(def.success) {
		t.Fatal("untouched color must keep the theme value")
	}
}

func TestNextThemeCycle(t *testing.T) {
	// forward from auto walks the whole cycle and wraps back to auto
	cur := "auto"
	seen := map[string]bool{}
	for range len(themeCycle) + 1 {
		if seen[cur] {
			t.Fatalf("theme cycle repeats %q before wrapping", cur)
		}
		seen[cur] = true
		cur = nextTheme(cur, 1)
	}
	if cur != "auto" {
		t.Fatalf("cycle must wrap to auto, got %q", cur)
	}
	// backward from auto lands on the last theme
	if got := nextTheme("auto", -1); got != themeCycle[len(themeCycle)-1] {
		t.Fatalf("nextTheme(auto, -1) = %q, want %q", got, themeCycle[len(themeCycle)-1])
	}
	// unknown names count as auto
	if got := nextTheme("bogus", 1); got != nextTheme("auto", 1) {
		t.Fatalf("unknown theme must cycle like auto, got %q", got)
	}
}
