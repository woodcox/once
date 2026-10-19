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

var installTargetDescriptions = map[InstallTarget]string{
	InstallTargetPublic:  "Reachable on the internet through ONCE's proxy, with TLS handled for you.",
	InstallTargetTailnet: "A Tailscale Service, reachable only through your tailnet. No host ports are opened.",
}

type InstallWhereSelectedMsg struct {
	Target InstallTarget
}

type InstallWhereForm struct {
	menu Menu
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
	switch msg := msg.(type) {
	case MenuSelectMsg:
		return m, func() tea.Msg { return InstallWhereSelectedMsg{Target: InstallTarget(msg.Key)} }
	}

	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(msg)
	return m, cmd
}

func (m InstallWhereForm) View() string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Colors.Border).
		Padding(1, 4)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(Colors.Primary).
		MarginBottom(1)

	title := titleStyle.Render("Where should this app run?")
	m.menu.SetWidth(lipgloss.Width(title))

	description := lipgloss.NewStyle().
		Foreground(Colors.Muted).
		MarginTop(1).
		Render(installTargetDescriptions[InstallTarget(m.menu.SelectedKey())])

	content := lipgloss.JoinVertical(lipgloss.Left, title, m.menu.View(), description)

	return boxStyle.Render(content)
}
