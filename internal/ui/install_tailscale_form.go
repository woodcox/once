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
	installTailscaleDirectField
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
	servicePortField.SetValue(strconv.Itoa(effectiveServicePort(settings)))

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

	directField := NewCheckboxField("Use the published container port instead of the container IP", settings.Direct != nil && !*settings.Direct)

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
			FormItem{Label: "Container address", Field: directField},
			FormItem{Label: "Proxy protocol", Field: proxyProtocolField},
			FormItem{Field: labelsField},
		),
	}

	refreshLabels := func(f *Form) {
		labelsField.SetValue(tailscaleLabelsText(m.applicationSettings(installTailscaleSettings(f))))
	}
	m.form.OnRebuild(refreshLabels)
	refreshLabels(&m.form)

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		return func() tea.Msg { return InstallTailscaleSubmitMsg{Tailscale: installTailscaleSettings(f)} }
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

// Private

func (m InstallTailscaleForm) applicationSettings(settings docker.TailscaleSettings) docker.ApplicationSettings {
	return docker.ApplicationSettings{
		Name:      docker.NameFromImageRef(m.imageRef),
		AppPort:   m.appPort,
		Tailscale: settings,
	}
}

// Helpers

func defaultServiceName(imageRef string) string {
	return docker.SanitizeDNSLabel(docker.NameFromImageRef(imageRef))
}

// effectiveServicePort mirrors docktail's smart default: 443 when the
// effective service protocol is https, 80 otherwise.
func effectiveServicePort(settings docker.TailscaleSettings) int {
	if settings.ServicePort != 0 {
		return settings.ServicePort
	}
	protocol := effectiveBackendProtocol(settings)
	serviceProtocol := effectiveTailscaleServiceProtocol(settings, protocol)
	if serviceProtocol == "https" {
		return 443
	}
	return 80
}

func effectiveBackendProtocol(settings docker.TailscaleSettings) string {
	if settings.Protocol != "" {
		return settings.Protocol
	}
	if settings.Port != 0 {
		if settings.Port == 443 {
			return "https"
		}
		return "http"
	}
	return "http"
}

func effectiveTailscaleServiceProtocol(settings docker.TailscaleSettings, backend string) string {
	if settings.ServiceProtocol != "" {
		return settings.ServiceProtocol
	}
	if settings.ServicePort != 0 {
		if settings.ServicePort == 443 {
			return "https"
		}
		if isTCPProtocol(backend) {
			return "tcp"
		}
		return "http"
	}
	if isTCPProtocol(backend) {
		return "tcp"
	}
	return "https"
}

func isTCPProtocol(protocol string) bool {
	return protocol == "tcp" || protocol == "tls-terminated-tcp"
}

func installTailscaleSettings(f *Form) docker.TailscaleSettings {
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
	}

	settings.Port, _ = strconv.Atoi(f.TextField(installTailscalePortField).Value())
	settings.ServicePort, _ = strconv.Atoi(f.TextField(installTailscaleServicePortField).Value())

	if f.CheckboxField(installTailscaleDirectField).Checked() {
		direct := false
		settings.Direct = &direct
	}

	return settings
}
