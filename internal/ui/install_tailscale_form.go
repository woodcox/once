package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

const (
	installTailscaleServiceField = iota
	installTailscaleDescriptionField
	installTailscalePortField
	installTailscaleBackendProtocolField
	installTailscaleServicePortField
	installTailscaleServiceProtocolField
	installTailscalePathField
	installTailscaleTagsField
	installTailscaleNetworkField
	installTailscaleProxyProtocolField
	installTailscaleLabelsField
)

type InstallTailscaleSubmitMsg struct {
	Tailscale docker.TailscaleSettings
}

type InstallTailscaleBackMsg struct{}

type InstallTailscaleForm struct {
	form     Form
	imageRef string
	appPort  int
}

func NewInstallTailscaleForm(imageRef string, appPort int, settings docker.TailscaleSettings) InstallTailscaleForm {
	serviceField := NewTextField("defaults to the app name")
	serviceField.SetValue(settings.Service)
	if serviceField.Value() == "" {
		serviceField.SetValue(defaultServiceName(imageRef))
	}

	descriptionField := NewTextField("shown in the Tailscale admin console; needs docktail API credentials to sync")
	descriptionField.SetValue(settings.Description)

	portField := NewTextField("app port")
	portField.SetDigitsOnly(true)
	if settings.Port != 0 {
		portField.SetValue(strconv.Itoa(settings.Port))
	}

	backendProtocolField := NewTextField("blank = https on 443, else http; http, https, https+insecure, tcp, tls-terminated-tcp")
	backendProtocolField.SetValue(settings.Protocol)

	servicePortField := NewTextField("443")
	servicePortField.SetDigitsOnly(true)
	servicePortField.SetValue(strconv.Itoa(tailscaleApplicationSettings(imageRef, appPort, settings).EffectiveServicePort()))

	serviceProtocolField := NewTextField("blank = https on 443, tcp for a TCP backend, else http; http, https, tcp, tls-terminated-tcp")
	serviceProtocolField.SetValue(settings.ServiceProtocol)

	pathField := NewTextField("/")
	pathField.SetValue(settings.Path)
	if pathField.Value() == "" {
		pathField.SetValue("/")
	}

	tagsField := NewTextField("tag:container")
	tagsField.SetValue(settings.Tags)

	networkField := NewTextField("docktail default; the Docker network used to find the container IP")
	networkField.SetValue(settings.Network)

	proxyProtocolField := NewTextField("off; 1 or 2; only for TCP service protocols")
	proxyProtocolField.SetValue(settings.ProxyProtocol)

	labelsField := NewStaticField("", func(s string) string { return s })

	m := InstallTailscaleForm{
		imageRef: imageRef,
		appPort:  appPort,
		form: NewForm("Install",
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

	refreshLabels := func(f *Form) {
		labelsField.SetValue(tailscaleLabelsText(tailscaleApplicationSettings(imageRef, appPort, installTailscaleSettings(f, settings))))
	}
	m.form.OnRebuild(refreshLabels)
	refreshLabels(&m.form)

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		return func() tea.Msg { return InstallTailscaleSubmitMsg{Tailscale: installTailscaleSettings(f, settings)} }
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

// tailscaleApplicationSettings builds the minimal ApplicationSettings the
// docker package needs to derive the protocol/port defaults for the form's
// preview and the Service Port field.
func tailscaleApplicationSettings(imageRef string, appPort int, tailscale docker.TailscaleSettings) docker.ApplicationSettings {
	return docker.ApplicationSettings{
		Name:      docker.NameFromImageRef(imageRef),
		AppPort:   appPort,
		Tailscale: tailscale,
	}
}

func installTailscaleSettings(f *Form, original docker.TailscaleSettings) docker.TailscaleSettings {
	settings := docker.TailscaleSettings{
		Enabled:         true,
		Mode:            docker.DocktailMode,
		Service:         f.TextField(installTailscaleServiceField).Value(),
		Description:     f.TextField(installTailscaleDescriptionField).Value(),
		Protocol:        f.TextField(installTailscaleBackendProtocolField).Value(),
		ServiceProtocol: f.TextField(installTailscaleServiceProtocolField).Value(),
		Path:            f.TextField(installTailscalePathField).Value(),
		Tags:            f.TextField(installTailscaleTagsField).Value(),
		Network:         f.TextField(installTailscaleNetworkField).Value(),
		ProxyProtocol:   f.TextField(installTailscaleProxyProtocolField).Value(),
		Direct:          original.Direct,
	}

	settings.Port, _ = strconv.Atoi(f.TextField(installTailscalePortField).Value())
	settings.ServicePort, _ = strconv.Atoi(f.TextField(installTailscaleServicePortField).Value())

	return settings
}
