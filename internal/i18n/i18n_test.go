package i18n

import (
	"strings"
	"testing"
)

func TestTableComplete(t *testing.T) {
	for id := ID(0); id < idCount; id++ {
		e, ok := table[id]
		if !ok {
			t.Fatalf("message ID %d missing from table", int(id))
		}
		if strings.TrimSpace(e[0]) == "" || strings.TrimSpace(e[1]) == "" {
			t.Fatalf("message ID %d has an empty translation: %q", int(id), e)
		}
	}
}

func TestResolve(t *testing.T) {
	env := func(kv map[string]string) func(string) string {
		return func(k string) string { return kv[k] }
	}
	cases := []struct {
		cfg  string
		kv   map[string]string
		want Lang
	}{
		{"en", map[string]string{"LANG": "zh_CN.UTF-8"}, EN}, // explicit wins
		{"zh", map[string]string{"LANG": "en_US.UTF-8"}, ZH}, // explicit wins
		{"auto", map[string]string{"LANG": "zh_CN.UTF-8"}, ZH},
		{"auto", map[string]string{"LC_ALL": "zh_TW"}, ZH},
		{"auto", map[string]string{"LC_CTYPE": "zh-Hans"}, ZH},
		{"auto", map[string]string{"LANG": "en_US.UTF-8"}, EN},
		{"auto", map[string]string{}, EN},
		{"", map[string]string{"LANG": "C"}, EN},
		{"bogus", map[string]string{"LANG": "zh_CN"}, ZH}, // unknown acts as auto
	}
	for _, c := range cases {
		if got := Resolve(c.cfg, env(c.kv)); got != c.want {
			t.Errorf("Resolve(%q, %v) = %q, want %q", c.cfg, c.kv, got, c.want)
		}
	}
}

func TestT(t *testing.T) {
	if got := T(EN, ActNew); got != "new" {
		t.Fatalf("T(EN, ActNew) = %q", got)
	}
	if got := T(ZH, ActNew); got != "新建" {
		t.Fatalf("T(ZH, ActNew) = %q", got)
	}
	if got := T(ZH, KillSessionQ, "work"); got != "删除会话 'work'？" {
		t.Fatalf("T(ZH, KillSessionQ) = %q", got)
	}
	if got := T(EN, ID(9999)); !strings.Contains(got, "9999") {
		t.Fatalf("unknown ID must be visible, got %q", got)
	}
}

func TestPlural(t *testing.T) {
	if got := Plural(EN, 1, UnitWindow); got != "1 window" {
		t.Fatalf("Plural(EN, 1) = %q", got)
	}
	if got := Plural(EN, 3, UnitWindow); got != "3 windows" {
		t.Fatalf("Plural(EN, 3) = %q", got)
	}
	if got := Plural(ZH, 1, UnitWindow); got != "1 个窗口" {
		t.Fatalf("Plural(ZH, 1) = %q", got)
	}
	if got := Plural(ZH, 3, UnitWindow); got != "3 个窗口" {
		t.Fatalf("Plural(ZH, 3) = %q", got)
	}
}
