package ui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var tailscaleHelpKeys = struct {
	Close key.Binding
}{
	Close: NewKeyBinding("enter", "esc", "f1", "q"),
}

type TailscaleHelpCloseMsg struct{}

// TailscaleHelpPanel scrolls through the full reference for every docktail
// label ONCE can publish on a Tailscale Service container. The panel hides
// the form behind it and treats every keystroke as panel input, so opening
// it can't accidentally submit or navigate away.
type TailscaleHelpPanel struct {
	viewport viewport.Model
}

func NewTailscaleHelpPanel(termWidth, termHeight int) TailscaleHelpPanel {
	popupWidth := min(termWidth-4, 84)
	boxStyle := tailscaleHelpBoxStyle()
	innerWidth := popupWidth - boxStyle.GetHorizontalFrameSize()

	chrome := tailscaleHelpChrome("placeholder", innerWidth)
	chromeLines := lipgloss.Height(boxStyle.Render(chrome))

	vpHeight := max(termHeight-chromeLines-2, 5)

	vp := viewport.New()
	vp.SetWidth(innerWidth)
	vp.SetHeight(vpHeight)
	// Wrap here rather than leaving it to the viewport: its soft wrap
	// breaks mid-word when a line exceeds the width.
	vp.SetContent(lipgloss.NewStyle().Width(innerWidth).Render(renderTailscaleHelpContent()))
	vp.SoftWrap = true
	vp.MouseWheelEnabled = true
	vp.KeyMap = viewport.KeyMap{
		PageDown:     NewKeyBinding("pgdown", "space"),
		PageUp:       NewKeyBinding("pgup"),
		HalfPageUp:   NewKeyBinding("ctrl+u"),
		HalfPageDown: NewKeyBinding("ctrl+d"),
		Down:         NewKeyBinding("down", "j"),
		Up:           NewKeyBinding("up", "k"),
	}

	return TailscaleHelpPanel{viewport: vp}
}

func (m TailscaleHelpPanel) Update(msg tea.Msg) (TailscaleHelpPanel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if key.Matches(msg, tailscaleHelpKeys.Close) {
			return m, func() tea.Msg { return TailscaleHelpCloseMsg{} }
		}
		switch msg.String() {
		case "home", "g":
			m.viewport.GotoTop()
			return m, nil
		case "end", "G":
			m.viewport.GotoBottom()
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m TailscaleHelpPanel) View() string {
	body := m.viewport.View()
	width := m.viewport.Width()
	indicator := tailscaleHelpIndicator(
		m.viewport.YOffset(),
		m.viewport.VisibleLineCount(),
		m.viewport.TotalLineCount(),
	)
	content := lipgloss.JoinVertical(lipgloss.Left,
		tailscaleHelpChrome(body, width),
		indicator,
	)
	return tailscaleHelpBoxStyle().Render(content)
}

// Private

func tailscaleHelpBoxStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Padding(1, 3)
}

func tailscaleHelpChrome(body string, width int) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(Colors.Primary)
	// Body text, not a caption: the intro reads as prose, so it keeps the
	// normal foreground and only wraps to the panel width.
	introStyle := lipgloss.NewStyle().Width(width)

	parts := []string{
		titleStyle.Render("Tailscale Service labels (docktail)"),
		introStyle.Render("ONCE publishes the app as a Tailscale Service by writing `docktail.*` labels on the container. Blank fields mean docktail's default."),
		"",
		body,
	}

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func tailscaleHelpIndicator(yOffset, visibleLines, total int) string {
	muted := lipgloss.NewStyle().Foreground(Colors.Muted)
	if total == 0 || visibleLines == 0 {
		return muted.Render("0-0 of 0")
	}
	visibleStart := yOffset + 1
	visibleEnd := min(visibleStart+visibleLines-1, total)
	if visibleEnd >= total {
		return muted.Render(fmt.Sprintf("%d-%d of %d", visibleStart, total, total))
	}
	return muted.Render(fmt.Sprintf("%d-%d of %d   ↓ more", visibleStart, visibleEnd, total))
}
