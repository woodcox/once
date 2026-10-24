package ui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

var doctorKeys = struct {
	Refresh key.Binding
	Back    key.Binding
}{
	Refresh: WithHelp(NewKeyBinding("r"), "r", "re-run"),
	Back:    WithHelp(NewKeyBinding("esc"), "esc", "back"),
}

type doctorFinishedMsg struct {
	result docker.DoctorResult
	err    error
}

// Doctor inspects an application and renders its checklist. The check runs
// asynchronously, so the screen stays responsive while the HTTP and TCP
// probes run.
type Doctor struct {
	namespace     *docker.Namespace
	app           *docker.Application
	viewport      viewport.Model
	width, height int
	help          Help
	probes        docker.DoctorProbes
	checking      bool
	result        *docker.DoctorResult
	err           error
}

func NewDoctor(ns *docker.Namespace, app *docker.Application) Doctor {
	vp := viewport.New()
	vp.MouseWheelEnabled = true
	vp.SoftWrap = true

	h := NewHelp()
	h.SetBindings([]key.Binding{doctorKeys.Refresh, doctorKeys.Back})

	return Doctor{
		namespace: ns,
		app:       app,
		viewport:  vp,
		help:      h,
		probes:    docker.DoctorProbesFor(ns),
		checking:  true,
	}
}

func (m Doctor) Init() tea.Cmd {
	return m.runDoctor()
}

func (m Doctor) Update(msg tea.Msg) (Component, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(m.width)
		m.updateViewportSize()
		m.rebuildContent()

	case MouseEvent:
		var cmd tea.Cmd
		m.help, cmd = m.help.Update(msg)
		if cmd != nil {
			return m, cmd
		}

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, doctorKeys.Back):
			return m, func() tea.Msg { return NavigateToDashboardMsg{AppName: m.app.Settings.Name} }
		case key.Matches(msg, doctorKeys.Refresh):
			m.checking = true
			m.result = nil
			m.err = nil
			m.rebuildContent()
			return m, m.runDoctor()
		}

		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case doctorFinishedMsg:
		m.checking = false
		m.err = msg.err
		if msg.err == nil {
			result := msg.result
			m.result = &result
		}
		m.rebuildContent()
	}

	return m, nil
}

func (m Doctor) View() string {
	header := m.titleLine() + "\n" + m.headerView()
	helpLine := Styles.CenteredLine(m.width, m.help.View())

	m.updateViewportSize()

	return header + "\n" + m.viewport.View() + "\n" + helpLine
}

// Private

func (m Doctor) runDoctor() tea.Cmd {
	ns := m.namespace
	name := m.app.Settings.Name
	probes := m.probes

	return func() tea.Msg {
		result, err := docker.Doctor(context.Background(), ns, name, probes)
		return doctorFinishedMsg{result: result, err: err}
	}
}

func (m Doctor) headerView() string {
	name := m.app.Settings.Name
	host := m.app.Settings.Host

	heading := name
	if host != "" {
		heading += " (" + host + ")"
	}

	return lipgloss.NewStyle().Bold(true).Foreground(Colors.Primary).Render(heading)
}

func (m *Doctor) rebuildContent() {
	if m.checking {
		m.viewport.SetContent(m.centeredMessage("Checking " + m.app.Settings.Name + "…"))
		return
	}

	if m.err != nil {
		m.viewport.SetContent(m.centeredMessage(lipgloss.NewStyle().Foreground(Colors.Error).Render(docker.ErrorMessage(m.err))))
		return
	}

	if m.result == nil {
		m.viewport.SetContent(m.centeredMessage("Nothing checked yet"))
		return
	}

	m.viewport.SetContent(m.reportContent())
}

func (m Doctor) reportContent() string {
	var lines []string

	for _, report := range m.result.Reports {
		lines = append(lines, m.reportLines(report)...)
	}

	if len(lines) == 0 {
		return m.centeredMessage("No checks were run")
	}

	return strings.Join(lines, "\n")
}

func (m Doctor) reportLines(report docker.DoctorReport) []string {
	var lines []string

	for _, check := range report.Checks {
		lines = append(lines, m.checkLine(check))
		if check.Status == docker.DoctorFail && check.Remedy != "" {
			lines = append(lines, Styles.MutedText.Render("    remedy: "+check.Remedy))
		}
	}

	if report.URL != "" && !doctorHasURLCheck(report) {
		lines = append(lines, "  "+report.URL)
	}

	if report.Note != "" {
		lines = append(lines, Styles.MutedText.Render("  note: "+strings.TrimSpace(report.Note)))
	}

	lines = append(lines, "")

	return lines
}

func (m Doctor) checkLine(check docker.DoctorCheck) string {
	marker := doctorCheckMarker(check.Status)
	message := doctorCheckMessage(check)

	return "  " + marker + " " + message
}

func (m Doctor) centeredMessage(msg string) string {
	return lipgloss.Place(m.viewport.Width(), m.viewport.Height(), lipgloss.Center, lipgloss.Center, msg)
}

func (m *Doctor) updateViewportSize() {
	headerHeight := lipgloss.Height(m.titleLine() + "\n" + m.headerView())
	helpHeight := m.help.Height()
	viewportHeight := m.height - headerHeight - helpHeight

	if viewportHeight > 0 {
		m.viewport.SetHeight(viewportHeight)
	}
	m.viewport.SetWidth(m.width)
}

func (m Doctor) titleLine() string {
	return Styles.TitleRule(m.width, m.app.Settings.Host, "doctor")
}

// Helpers

// doctorCheckMarker returns the marker shown in front of a check.
func doctorCheckMarker(status docker.DoctorStatus) string {
	switch status {
	case docker.DoctorPass:
		return lipgloss.NewStyle().Foreground(Colors.Success).Render("✓")
	case docker.DoctorFail:
		return lipgloss.NewStyle().Foreground(Colors.Error).Render("✗")
	default:
		return lipgloss.NewStyle().Foreground(Colors.Muted).Render("•")
	}
}

// doctorCheckMessage renders a check as its label followed by its message.
func doctorCheckMessage(check docker.DoctorCheck) string {
	if check.Message == "" {
		return check.Label
	}
	if strings.HasPrefix(check.Message, ":") {
		return check.Label + check.Message
	}
	return check.Label + " " + check.Message
}

// doctorHasURLCheck reports whether the checklist already shows the app's
// URL, so it isn't shown twice.
func doctorHasURLCheck(report docker.DoctorReport) bool {
	for _, check := range report.Checks {
		if check.Label == "url" {
			return true
		}
	}
	return false
}
