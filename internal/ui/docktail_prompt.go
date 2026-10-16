package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/mouse"
)

var docktailPromptKeys = struct {
	Retry  key.Binding
	Keep   key.Binding
	Delete key.Binding
	Next   key.Binding
	Prev   key.Binding
	Enter  key.Binding
	Esc    key.Binding
}{
	Retry:  NewKeyBinding("r"),
	Keep:   NewKeyBinding("k"),
	Delete: NewKeyBinding("d"),
	Next:   NewKeyBinding("tab"),
	Prev:   NewKeyBinding("shift+tab"),
	Enter:  NewKeyBinding("enter"),
	Esc:    NewKeyBinding("esc"),
}

type docktailDecision int

const (
	docktailRetry docktailDecision = iota
	docktailKeep
	docktailRollback
)

type docktailDecisionMsg struct {
	decision docktailDecision
}

// DocktailPrompt asks how to handle an app whose Tailscale Service hasn't been
// advertised. Focus defaults to keeping the app, since deleting it also
// deletes its data volume.
type DocktailPrompt struct {
	message string
	focused int // 0 = retry, 1 = keep, 2 = delete
}

func NewDocktailPrompt(message string) DocktailPrompt {
	return DocktailPrompt{
		message: message,
		focused: 1,
	}
}

func (m DocktailPrompt) Update(msg tea.Msg) (DocktailPrompt, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, docktailPromptKeys.Retry):
			return m, m.decide(docktailRetry)
		case key.Matches(msg, docktailPromptKeys.Keep):
			return m, m.decide(docktailKeep)
		case key.Matches(msg, docktailPromptKeys.Delete):
			return m, m.decide(docktailRollback)
		case key.Matches(msg, docktailPromptKeys.Next):
			m.focused = (m.focused + 1) % 3
		case key.Matches(msg, docktailPromptKeys.Prev):
			m.focused = (m.focused + 2) % 3
		case key.Matches(msg, docktailPromptKeys.Enter):
			return m, m.decide(docktailDecision(m.focused))
		case key.Matches(msg, docktailPromptKeys.Esc):
			return m, m.decide(docktailKeep)
		}

	case MouseEvent:
		if msg.IsClick {
			switch msg.Target {
			case "docktail-retry":
				return m, m.decide(docktailRetry)
			case "docktail-keep":
				return m, m.decide(docktailKeep)
			case "docktail-delete":
				return m, m.decide(docktailRollback)
			}
		}
	}

	return m, nil
}

func (m DocktailPrompt) View() string {
	messageView := lipgloss.NewStyle().Bold(true).Render(m.message)

	buttons := []struct {
		label string
		tag   string
	}{
		{"Retry", "docktail-retry"},
		{"Keep app", "docktail-keep"},
		{"Delete app", "docktail-delete"},
	}

	rendered := make([]string, len(buttons))
	for i, button := range buttons {
		style := Styles.Button
		if i == m.focused {
			style = style.BorderForeground(Colors.Focused)
		}
		rendered[i] = mouse.Mark(button.tag, style.Render(button.label))
	}

	return lipgloss.JoinVertical(lipgloss.Center,
		messageView,
		"",
		lipgloss.JoinHorizontal(lipgloss.Center, rendered...),
	)
}

// Private

func (m DocktailPrompt) decide(decision docktailDecision) tea.Cmd {
	return func() tea.Msg { return docktailDecisionMsg{decision: decision} }
}
