package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

type InstallTarget int

const (
	InstallTargetPublic InstallTarget = iota
	InstallTargetTailnet
)

const installWhereInnerWidth = 52

var installTargetDescriptions = map[InstallTarget]string{
	InstallTargetPublic:  "Reachable on the internet through ONCE's proxy, with TLS handled for you.",
	InstallTargetTailnet: "A Tailscale Service, reachable only through your tailnet. No ports are reachable off the host except through the Service.",
}

type InstallWhereSelectedMsg struct {
	Target InstallTarget
}

type InstallWhereForm struct {
	menu  Menu
	width int
}

func NewInstallWhereForm() InstallWhereForm {
	return InstallWhereForm{menu: NewMenu(
		MenuItem{Label: "Public", Key: int(InstallTargetPublic)},
		MenuItem{Label: "Tailnet only", Key: int(InstallTargetTailnet)},
	)}
}

// Apply sets the settings that describe where the app runs. A public app is
// reached through ONCE's proxy, so any Tailscale settings are cleared; a
// tailnet app is advertised as a Service.
func (t InstallTarget) Apply(settings docker.ApplicationSettings) docker.ApplicationSettings {
	if t == InstallTargetTailnet {
		settings.Tailscale.Enabled = true
		settings.Tailscale.Mode = docker.DocktailMode
		return settings
	}

	settings.Tailscale = docker.TailscaleSettings{}
	return settings
}

func (m InstallWhereForm) Init() tea.Cmd {
	return nil
}

func (m InstallWhereForm) Update(msg tea.Msg) (InstallWhereForm, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
	}

	switch msg := msg.(type) {
	case MenuSelectMsg:
		return m, func() tea.Msg { return InstallWhereSelectedMsg{Target: InstallTarget(msg.Key)} }
	}

	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(msg)
	return m, cmd
}

func (m InstallWhereForm) View() string {
	inner := lipgloss.NewStyle().Width(installWhereInnerWidth)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(Colors.Primary).
		Width(installWhereInnerWidth).
		Align(lipgloss.Center).
		Render("Where should this app run?")

	menuWidth := max(installWhereInnerWidth, m.menu.minWidth)
	m.menu.SetWidth(menuWidth)
	menuView := lipgloss.NewStyle().Width(installWhereInnerWidth).Align(lipgloss.Center).Render(m.menu.View())

	description := inner.
		Foreground(Colors.Muted).
		Align(lipgloss.Center).
		Render(installTargetDescriptions[InstallTarget(m.menu.SelectedKey())])

	gap := lipgloss.NewStyle().Width(installWhereInnerWidth).Render("")
	content := lipgloss.JoinVertical(lipgloss.Center,
		title,
		gap,
		menuView,
		gap,
		description,
	)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Padding(1, 4)

	box := boxStyle.Render(content)
	if m.width <= 0 {
		return box
	}
	return lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(box)
}
