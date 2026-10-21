package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func keyPressMsg(s string) tea.KeyPressMsg {
	k := tea.Key{Text: s}
	if r := []rune(s); len(r) == 1 {
		k.Code = r[0]
	}
	return tea.KeyPressMsg(k)
}

func lineCountAt(view string) int {
	if view == "" {
		return 0
	}
	return strings.Count(view, "\n") + 1
}

func lineContaining(view, needle string) string {
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}
