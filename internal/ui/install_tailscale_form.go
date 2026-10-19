package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

const (
	installTailscaleServiceField = iota
	installTailscalePortField
	installTailscaleServicePortField
	installTailscalePathField
	installTailscaleTagsField
)

type InstallTailscaleSubmitMsg struct {
	Tailscale docker.TailscaleSettings
}

type InstallTailscaleBackMsg struct{}

type InstallTailscaleForm struct {
	form Form
}

func NewInstallTailscaleForm(imageRef string, settings docker.TailscaleSettings) InstallTailscaleForm {
	serviceField := NewTextField("defaults to the app name")
	serviceField.SetValue(settings.Service)
	if serviceField.Value() == "" {
		serviceField.SetValue(defaultServiceName(imageRef))
	}

	portField := NewTextField("app port")
	portField.SetDigitsOnly(true)
	if settings.Port != 0 {
		portField.SetValue(strconv.Itoa(settings.Port))
	}

	servicePortField := NewTextField("443")
	servicePortField.SetDigitsOnly(true)
	servicePortField.SetValue(strconv.Itoa(effectiveServicePort(settings)))

	pathField := NewTextField("/")
	pathField.SetValue(settings.Path)
	if pathField.Value() == "" {
		pathField.SetValue("/")
	}

	tagsField := NewTextField("tag:container")
	tagsField.SetValue(settings.Tags)

	m := InstallTailscaleForm{
		form: NewForm("Install",
			FormItem{Label: "Service name", Field: serviceField},
			FormItem{Label: "Backend port", Field: portField},
			FormItem{Label: "Service port", Field: servicePortField},
			FormItem{Label: "Service path", Field: pathField},
			FormItem{Label: "Service tags", Field: tagsField},
		),
	}

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := settings
		s.Enabled = true
		s.Mode = docker.DocktailMode
		s.Service = f.TextField(installTailscaleServiceField).Value()
		s.Port, _ = strconv.Atoi(f.TextField(installTailscalePortField).Value())
		s.ServicePort, _ = strconv.Atoi(f.TextField(installTailscaleServicePortField).Value())
		s.Path = f.TextField(installTailscalePathField).Value()
		s.Tags = f.TextField(installTailscaleTagsField).Value()
		return func() tea.Msg { return InstallTailscaleSubmitMsg{Tailscale: s} }
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return InstallTailscaleBackMsg{} }
	})

	return m
}

func (m InstallTailscaleForm) Init() tea.Cmd {
	return m.form.Init()
}

func (m InstallTailscaleForm) Update(msg tea.Msg) (InstallTailscaleForm, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		wsm.Height = formViewportHeight(wsm.Height)
		msg = wsm
	}

	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	return m, cmd
}

func (m InstallTailscaleForm) View() string {
	title := Styles.Title.Render("Tailscale Service")
	return lipgloss.JoinVertical(lipgloss.Center, title, "", m.form.View())
}

// Helpers

func defaultServiceName(imageRef string) string {
	return docker.SanitizeDNSLabel(docker.NameFromImageRef(imageRef))
}

func effectiveServicePort(settings docker.TailscaleSettings) int {
	if settings.ServicePort != 0 {
		return settings.ServicePort
	}
	return 443
}
