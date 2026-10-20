package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/internal/docker"
)

const (
	tailscaleEnabledField = iota
	tailscaleServiceNameField
	tailscaleDescriptionField
	tailscaleBackendPortField
	tailscaleBackendProtocolField
	tailscaleServicePortField
	tailscaleServiceProtocolField
	tailscalePathField
	tailscaleTagsField
	tailscaleNetworkField
	tailscaleProxyProtocolField
	tailscaleLabelsField
)

type SettingsFormTailscale struct {
	settingsFormBase
	settings docker.ApplicationSettings
}

func NewSettingsFormTailscale(settings docker.ApplicationSettings) SettingsFormTailscale {
	enabledField := NewCheckboxField("Publish as a Tailscale Service", settings.DocktailEnabled())

	serviceField := NewTextField("defaults to the app name")
	serviceField.SetValue(settings.Tailscale.Service)

	descriptionField := NewTextField("shown in the Tailscale admin console; needs docktail API credentials to sync")
	descriptionField.SetValue(settings.Tailscale.Description)

	portField := NewTextField("app port")
	portField.SetDigitsOnly(true)
	if settings.Tailscale.Port != 0 {
		portField.SetValue(strconv.Itoa(settings.Tailscale.Port))
	}

	backendProtocolField := NewTextField("blank = https on 443, else http; http, https, https+insecure, tcp, tls-terminated-tcp")
	backendProtocolField.SetValue(settings.Tailscale.Protocol)

	servicePortField := NewTextField("443")
	servicePortField.SetDigitsOnly(true)
	if settings.Tailscale.ServicePort != 0 {
		servicePortField.SetValue(strconv.Itoa(settings.Tailscale.ServicePort))
	}

	serviceProtocolField := NewTextField("blank = https on 443, tcp for a TCP backend, else http; http, https, tcp, tls-terminated-tcp")
	serviceProtocolField.SetValue(settings.Tailscale.ServiceProtocol)

	pathField := NewTextField("/")
	pathField.SetValue(settings.Tailscale.Path)

	tagsField := NewTextField("tag:container")
	tagsField.SetValue(settings.Tailscale.Tags)

	networkField := NewTextField("docktail default; the Docker network used to find the container IP")
	networkField.SetValue(settings.Tailscale.Network)

	proxyProtocolField := NewTextField("off; 1 or 2; only for TCP service protocols")
	proxyProtocolField.SetValue(settings.Tailscale.ProxyProtocol)

	labelsField := NewStaticField("", func(s string) string { return s })

	base := settingsFormBase{
		title: "Tailscale",
		form: NewForm("Done",
			FormItem{Label: "Tailscale Service", Field: enabledField},
			FormItem{Label: "Service name", Field: serviceField},
			FormItem{Label: "Description", Field: descriptionField},
			FormItem{Label: "Backend port", Field: portField},
			FormItem{Label: "Backend protocol", Field: backendProtocolField},
			FormItem{Label: "Service port", Field: servicePortField},
			FormItem{Label: "Service protocol", Field: serviceProtocolField},
			FormItem{Label: "Service path", Field: pathField},
			FormItem{Label: "Service tags", Field: tagsField},
			FormItem{Label: "Network", Field: networkField},
			FormItem{Label: "Proxy protocol", Field: proxyProtocolField},
			FormItem{Field: labelsField},
		),
	}

	m := SettingsFormTailscale{
		settingsFormBase: base,
		settings:         settings,
	}

	refreshLabels := func(f *Form) {
		labelsField.SetValue(tailscaleLabelsText(m.applicationSettings(f)))
	}
	m.form.OnRebuild(refreshLabels)
	refreshLabels(&m.form)

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := settings
		s.Tailscale = tailscaleSettings(f, s.Tailscale)
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

// Private

func (m SettingsFormTailscale) applicationSettings(f *Form) docker.ApplicationSettings {
	settings := m.settings
	settings.Tailscale = tailscaleSettings(f, m.settings.Tailscale)
	return settings
}

// Helpers

func tailscaleSettings(f *Form, original docker.TailscaleSettings) docker.TailscaleSettings {
	if !f.CheckboxField(tailscaleEnabledField).Checked() {
		return docker.TailscaleSettings{}
	}

	settings := docker.TailscaleSettings{
		Enabled:         true,
		Mode:            docker.DocktailMode,
		Service:         f.TextField(tailscaleServiceNameField).Value(),
		Description:     f.TextField(tailscaleDescriptionField).Value(),
		Protocol:        f.TextField(tailscaleBackendProtocolField).Value(),
		ServiceProtocol: f.TextField(tailscaleServiceProtocolField).Value(),
		Path:            f.TextField(tailscalePathField).Value(),
		Tags:            f.TextField(tailscaleTagsField).Value(),
		Network:         f.TextField(tailscaleNetworkField).Value(),
		ProxyProtocol:   f.TextField(tailscaleProxyProtocolField).Value(),
		Direct:          original.Direct,
	}

	settings.Port, _ = strconv.Atoi(f.TextField(tailscaleBackendPortField).Value())
	settings.ServicePort, _ = strconv.Atoi(f.TextField(tailscaleServicePortField).Value())

	return settings
}
