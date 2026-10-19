package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/internal/docker"
)

const (
	tailscaleEnabledField = iota
	tailscaleServiceNameField
	tailscaleBackendPortField
	tailscaleServicePortField
	tailscalePathField
	tailscaleTagsField
)

type SettingsFormTailscale struct {
	settingsFormBase
}

func NewSettingsFormTailscale(settings docker.ApplicationSettings) SettingsFormTailscale {
	enabledField := NewCheckboxField("Publish as a Tailscale Service", settings.DocktailEnabled())

	serviceField := NewTextField("defaults to the app name")
	serviceField.SetValue(settings.Tailscale.Service)

	portField := NewTextField("app port")
	portField.SetDigitsOnly(true)
	if settings.Tailscale.Port != 0 {
		portField.SetValue(strconv.Itoa(settings.Tailscale.Port))
	}

	servicePortField := NewTextField("443")
	servicePortField.SetDigitsOnly(true)
	if settings.Tailscale.ServicePort != 0 {
		servicePortField.SetValue(strconv.Itoa(settings.Tailscale.ServicePort))
	}

	pathField := NewTextField("/")
	pathField.SetValue(settings.Tailscale.Path)

	tagsField := NewTextField("tag:container")
	tagsField.SetValue(settings.Tailscale.Tags)

	m := SettingsFormTailscale{
		title: "Tailscale",
		form: NewForm("Done",
			FormItem{Label: "Tailscale Service", Field: enabledField},
			FormItem{Label: "Service name", Field: serviceField},
			FormItem{Label: "Backend port", Field: portField},
			FormItem{Label: "Service port", Field: servicePortField},
			FormItem{Label: "Service path", Field: pathField},
			FormItem{Label: "Service tags", Field: tagsField},
		),
	}

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := settings
		s.Tailscale.Enabled = f.CheckboxField(tailscaleEnabledField).Checked()
		if !s.Tailscale.Enabled {
			s.Tailscale = docker.TailscaleSettings{}
			return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
		}

		s.Tailscale.Mode = docker.DocktailMode
		s.Tailscale.Service = f.TextField(tailscaleServiceNameField).Value()
		s.Tailscale.Port, _ = strconv.Atoi(f.TextField(tailscaleBackendPortField).Value())
		s.Tailscale.ServicePort, _ = strconv.Atoi(f.TextField(tailscaleServicePortField).Value())
		s.Tailscale.Path = f.TextField(tailscalePathField).Value()
		s.Tailscale.Tags = f.TextField(tailscaleTagsField).Value()

		return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m SettingsFormTailscale) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	var cmd tea.Cmd
	m.settingsFormBase, cmd = m.update(msg)
	return m, cmd
}
