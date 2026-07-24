package app

import (
	"github.com/DKmiyan/tmuxgo/internal/i18n"
)

// tr formats a user-visible message in the model's language.
func (m model) tr(id i18n.ID, args ...any) string {
	return i18n.T(m.lang, id, args...)
}

// plural formats a counted unit ("1 window" / "2 windows"; Chinese has no
// plural form).
func (m model) plural(n int, unit i18n.ID) string {
	return i18n.Plural(m.lang, n, unit)
}

// nextLanguage cycles the configured language: auto -> en -> zh -> auto
// (delta -1 walks backward).
func nextLanguage(cur string, delta int) string {
	order := []string{"auto", "en", "zh"}
	i := 0
	for j, l := range order {
		if l == cur {
			i = j
			break
		}
	}
	return order[(i+delta%len(order)+len(order))%len(order)]
}
