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
	installTailscaleDirectField
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

	descriptionField := NewTextField("shown in the Tailscale admin console")
	descriptionField.SetValue(settings.Description)

	portField := NewTextField("app port")
	portField.SetDigitsOnly(true)
	if settings.Port != 0 {
		portField.SetValue(strconv.Itoa(settings.Port))
	}

	backendProtocolField := NewTextField("http")
	backendProtocolField.SetValue(settings.Protocol)

	servicePortField := NewTextField("443")
	servicePortField.SetDigitsOnly(true)
	servicePortField.SetValue(strconv.Itoa(tailscaleApplicationSettings(imageRef, appPort, settings).EffectiveServicePort()))

	serviceProtocolField := NewTextField("auto")
	serviceProtocolField.SetValue(settings.ServiceProtocol)

	pathField := NewTextField("/")
	pathField.SetValue(settings.Path)
	if pathField.Value() == "" {
		pathField.SetValue("/")
	}

	tagsField := NewTextField("tag:container")
	tagsField.SetValue(settings.Tags)

	networkField := NewTextField("docktail default")
	networkField.SetValue(settings.Network)

	proxyProtocolField := NewTextField("off")
	proxyProtocolField.SetValue(settings.ProxyProtocol)

	direct := settings.Direct
	if direct == nil {
		trueDirect := true
		direct = &trueDirect
	}
	directField := NewCheckboxField("Proxy directly to the container IP", *direct)

	labelsField := NewStaticField("", func(s string) string { return s })

	items := []FormItem{
		// Service name and Backend port are labelled "(required)" because
		// they are the only fields users really must understand, but blank
		// values still resolve to a working default, so the form lets the
		// user submit without filling them in.
		{Label: "Service name (required)", LabelTag: "docktail.service.name", Field: serviceField, Description: docktailFieldDescriptions[docktailFieldServiceName]},
		{Label: "Description", LabelTag: "docktail.service.description", Field: descriptionField, Description: docktailFieldDescriptions[docktailFieldDescription]},
		{Label: "Backend port (required)", LabelTag: "docktail.service.port", Field: portField, Description: docktailFieldDescriptions[docktailFieldBackendPort]},
		{Label: "Backend protocol", LabelTag: "docktail.service.protocol", Field: backendProtocolField, Description: docktailFieldDescriptions[docktailFieldBackendProtocol]},
		{Label: "Service port", LabelTag: "docktail.service.service-port", Field: servicePortField, Description: docktailFieldDescriptions[docktailFieldServicePort]},
		{Label: "Service protocol", LabelTag: "docktail.service.service-protocol", Field: serviceProtocolField, Description: docktailFieldDescriptions[docktailFieldServiceProtocol]},
		{Label: "Service path", LabelTag: "docktail.service.path", Field: pathField, Description: docktailFieldDescriptions[docktailFieldPath]},
		{Label: "Service tags", LabelTag: "docktail.tags", Field: tagsField, Description: docktailFieldDescriptions[docktailFieldTags]},
		{Label: "Network", LabelTag: "docktail.service.network", Field: networkField, Description: docktailFieldDescriptions[docktailFieldNetwork]},
		{Label: "Proxy protocol", LabelTag: "docktail.service.proxy-protocol", Field: proxyProtocolField, Description: docktailFieldDescriptions[docktailFieldProxyProtocol]},
		{Label: "Direct", LabelTag: "docktail.service.direct", Field: directField, Description: docktailFieldDescriptions[docktailFieldDirect]},
		{Field: labelsField},
	}

	m := InstallTailscaleForm{
		imageRef: imageRef,
		appPort:  appPort,
		form:     NewForm("Install", items...),
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
	heading := Styles.MutedText.Render("Publishes this app as a Tailscale Service by adding docktail.* labels to the container.")
	return lipgloss.JoinVertical(lipgloss.Center, title, "", heading, "", m.form.View())
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
	}

	settings.Port, _ = strconv.Atoi(f.TextField(installTailscalePortField).Value())
	settings.ServicePort, _ = strconv.Atoi(f.TextField(installTailscaleServicePortField).Value())

	direct := f.CheckboxField(installTailscaleDirectField).Checked()
	settings.Direct = &direct

	return settings
}
