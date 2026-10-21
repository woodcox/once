package ui

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

// The Service screen asks for the fields users must understand first, then the
// optional ones, with the read-only Labels preview last.
const (
	tailscaleEnabledField = iota
	tailscaleServiceNameField
	tailscaleBackendPortField
	tailscaleDescriptionField
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
	panel    *TailscaleHelpPanel
	width    int
	height   int
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
			FormItem{Label: "Service name (required)", LabelTag: "docktail.service.name", Field: serviceField},
			FormItem{Label: "Backend port (required)", LabelTag: "docktail.service.port", Field: portField},
			FormItem{Label: "Description", LabelTag: "docktail.service.description", Field: descriptionField},
			FormItem{Label: "Backend protocol", LabelTag: "docktail.service.protocol", Field: backendProtocolField},
			FormItem{Label: "Service port", LabelTag: "docktail.service.service-port", Field: servicePortField},
			FormItem{Label: "Service protocol", LabelTag: "docktail.service.service-protocol", Field: serviceProtocolField},
			FormItem{Label: "Service path", LabelTag: "docktail.service.path", Field: pathField},
			FormItem{Label: "Service tags", LabelTag: "docktail.tags", Field: tagsField},
			FormItem{Label: "Network", LabelTag: "docktail.service.network", Field: networkField},
			FormItem{Label: "Proxy protocol", LabelTag: "docktail.service.proxy-protocol", Field: proxyProtocolField},
			FormItem{Label: "Direct", LabelTag: "docktail.service.direct", Field: directField},
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
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		m.height = wsm.Height
		m.panel = nil
		wsm.Height = formViewportHeight(wsm.Height)
		msg = wsm
	}

	if m.panel != nil {
		return m.updateWithPanel(msg)
	}

	if kp, ok := msg.(tea.KeyPressMsg); ok && key.Matches(kp, settingsKeys.Help) {
		return m.openHelp(), nil
	}

	var cmd tea.Cmd
	m.settingsFormBase, cmd = m.update(msg)
	return m, cmd
}

func (m SettingsFormTailscale) View() string {
	if m.panel != nil {
		return m.panel.View()
	}
	heading := Styles.MutedText.Render("Publishes this app as a Tailscale Service by adding docktail.* labels to the container.")
	return lipgloss.JoinVertical(lipgloss.Center, heading, "", m.form.View())
}

// Private

func (m SettingsFormTailscale) applicationSettings(f *Form) docker.ApplicationSettings {
	settings := m.settings
	settings.Tailscale = tailscaleSettings(f, m.settings.Tailscale)
	return settings
}

func (m SettingsFormTailscale) openHelp() SettingsFormTailscale {
	panel := NewTailscaleHelpPanel(m.width, m.height)
	m.panel = &panel
	return m
}

func (m SettingsFormTailscale) updateWithPanel(msg tea.Msg) (SettingsSection, tea.Cmd) {
	switch msg := msg.(type) {
	case TailscaleHelpCloseMsg:
		m.panel = nil
		return m, nil

	case tea.KeyPressMsg:
		if key.Matches(msg, settingsKeys.Help) {
			m.panel = nil
			return m, nil
		}
		var cmd tea.Cmd
		panel := *m.panel
		panel, cmd = panel.Update(msg)
		m.panel = &panel
		return m, cmd

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		panel := *m.panel
		panel, cmd = panel.Update(msg)
		m.panel = &panel
		return m, cmd
	}

	return m, nil
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
