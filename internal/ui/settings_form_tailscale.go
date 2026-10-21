package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	tailscaleDirectField
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

	descriptionField := NewTextField("shown in the Tailscale admin console")
	descriptionField.SetValue(settings.Tailscale.Description)

	portField := NewTextField("app port")
	portField.SetDigitsOnly(true)
	if settings.Tailscale.Port != 0 {
		portField.SetValue(strconv.Itoa(settings.Tailscale.Port))
	}

	backendProtocolField := NewTextField("http")
	backendProtocolField.SetValue(settings.Tailscale.Protocol)

	servicePortField := NewTextField("443")
	servicePortField.SetDigitsOnly(true)
	if settings.Tailscale.ServicePort != 0 {
		servicePortField.SetValue(strconv.Itoa(settings.Tailscale.ServicePort))
	}

	serviceProtocolField := NewTextField("auto")
	serviceProtocolField.SetValue(settings.Tailscale.ServiceProtocol)

	pathField := NewTextField("/")
	pathField.SetValue(settings.Tailscale.Path)

	tagsField := NewTextField("tag:container")
	tagsField.SetValue(settings.Tailscale.Tags)

	networkField := NewTextField("docktail default")
	networkField.SetValue(settings.Tailscale.Network)

	proxyProtocolField := NewTextField("off")
	proxyProtocolField.SetValue(settings.Tailscale.ProxyProtocol)

	direct := settings.Tailscale.Direct
	if direct == nil {
		trueDirect := true
		direct = &trueDirect
	}
	directField := NewCheckboxField("Proxy directly to the container IP", *direct)

	labelsField := NewStaticField("", func(s string) string { return s })

	base := settingsFormBase{
		title: "Tailscale",
		form: NewForm("Done",
			FormItem{Label: "Tailscale Service", Field: enabledField},
			// Service name and Backend port are labelled "(required)"
			// because they are the only fields users really must understand,
			// but blank values still resolve to a working default.
			FormItem{Label: "Service name (required)", LabelTag: "docktail.service.name", Field: serviceField, Description: docktailFieldDescriptions[docktailFieldServiceName]},
			FormItem{Label: "Description", LabelTag: "docktail.service.description", Field: descriptionField, Description: docktailFieldDescriptions[docktailFieldDescription]},
			FormItem{Label: "Backend port (required)", LabelTag: "docktail.service.port", Field: portField, Description: docktailFieldDescriptions[docktailFieldBackendPort]},
			FormItem{Label: "Backend protocol", LabelTag: "docktail.service.protocol", Field: backendProtocolField, Description: docktailFieldDescriptions[docktailFieldBackendProtocol]},
			FormItem{Label: "Service port", LabelTag: "docktail.service.service-port", Field: servicePortField, Description: docktailFieldDescriptions[docktailFieldServicePort]},
			FormItem{Label: "Service protocol", LabelTag: "docktail.service.service-protocol", Field: serviceProtocolField, Description: docktailFieldDescriptions[docktailFieldServiceProtocol]},
			FormItem{Label: "Service path", LabelTag: "docktail.service.path", Field: pathField, Description: docktailFieldDescriptions[docktailFieldPath]},
			FormItem{Label: "Service tags", LabelTag: "docktail.tags", Field: tagsField, Description: docktailFieldDescriptions[docktailFieldTags]},
			FormItem{Label: "Network", LabelTag: "docktail.service.network", Field: networkField, Description: docktailFieldDescriptions[docktailFieldNetwork]},
			FormItem{Label: "Proxy protocol", LabelTag: "docktail.service.proxy-protocol", Field: proxyProtocolField, Description: docktailFieldDescriptions[docktailFieldProxyProtocol]},
			FormItem{Label: "Direct", LabelTag: "docktail.service.direct", Field: directField, Description: docktailFieldDescriptions[docktailFieldDirect]},
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

func (m SettingsFormTailscale) View() string {
	heading := Styles.MutedText.Render("Publishes this app as a Tailscale Service by adding docktail.* labels to the container.")
	return lipgloss.JoinVertical(lipgloss.Center, heading, "", m.form.View())
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
	}

	settings.Port, _ = strconv.Atoi(f.TextField(tailscaleBackendPortField).Value())
	settings.ServicePort, _ = strconv.Atoi(f.TextField(tailscaleServicePortField).Value())

	direct := f.CheckboxField(tailscaleDirectField).Checked()
	settings.Direct = &direct

	return settings
}
