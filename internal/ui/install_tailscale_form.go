package ui

import (
	"strconv"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

var installTailscaleFormKeys = struct {
	Help key.Binding
}{
	Help: WithHelp(NewKeyBinding("f1"), "F1", "label help"),
}

// The Service screen asks for the fields users must understand first, then the
// optional ones, with the read-only Labels preview last.
const (
	installTailscaleServiceField = iota
	installTailscalePortField
	installTailscaleDescriptionField
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
	form          Form
	panel         *TailscaleHelpPanel
	width         int
	height        int
	imageRef      string
	appPort       int
	actionMessage *func() InstallAdvancedMsg
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
		{Label: "Service name (required)", LabelTag: "docktail.service.name", Field: serviceField},
		{Label: "Backend port (required)", LabelTag: "docktail.service.port", Field: portField},
		{Label: "Description", LabelTag: "docktail.service.description", Field: descriptionField},
		{Label: "Backend protocol", LabelTag: "docktail.service.protocol", Field: backendProtocolField},
		{Label: "Service port", LabelTag: "docktail.service.service-port", Field: servicePortField},
		{Label: "Service protocol", LabelTag: "docktail.service.service-protocol", Field: serviceProtocolField},
		{Label: "Service path", LabelTag: "docktail.service.path", Field: pathField},
		{Label: "Service tags", LabelTag: "docktail.tags", Field: tagsField},
		{Label: "Network", LabelTag: "docktail.service.network", Field: networkField},
		{Label: "Proxy protocol", LabelTag: "docktail.service.proxy-protocol", Field: proxyProtocolField},
		{Label: "Direct", LabelTag: "docktail.service.direct", Field: directField},
		{Field: labelsField},
	}

	m := InstallTailscaleForm{
		imageRef:      imageRef,
		appPort:       appPort,
		form:          NewForm("Install", items...),
		actionMessage: new(func() InstallAdvancedMsg),
	}
	*m.actionMessage = func() InstallAdvancedMsg {
		return InstallAdvancedMsg{ImageRef: imageRef}
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
	m.form.SetActionButton("Advanced settings", func() tea.Msg {
		msg := (*m.actionMessage)()
		msg.FromStage = InstallAdvancedFromTailscale
		return msg
	})

	return m
}

func (m InstallTailscaleForm) Init() tea.Cmd {
	return m.form.Init()
}

func (m InstallTailscaleForm) Update(msg tea.Msg) (InstallTailscaleForm, tea.Cmd) {
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

	if kp, ok := msg.(tea.KeyPressMsg); ok && key.Matches(kp, installTailscaleFormKeys.Help) {
		return m.openHelp(), nil
	}

	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	return m, cmd
}

func (m InstallTailscaleForm) View() string {
	if m.panel != nil {
		return m.panel.View()
	}

	title := Styles.Title.Render("Tailscale Service")
	heading := Styles.MutedText.Render("Publishes this app as a Tailscale Service by adding docktail.* labels to the container.")
	return lipgloss.JoinVertical(lipgloss.Center, title, "", heading, "", m.form.View())
}

// Settings reads the Service options currently on the form, so the install
// screen can carry them across a rebuild.
func (m InstallTailscaleForm) Settings() docker.TailscaleSettings {
	return installTailscaleSettings(&m.form, docker.TailscaleSettings{})
}

// SetActionMessage is the message the form's Advanced settings button emits.
// The install screen fills in the hostname, which only it knows.
func (m *InstallTailscaleForm) SetActionMessage(fn func() InstallAdvancedMsg) {
	*m.actionMessage = fn
}

// Private

func (m InstallTailscaleForm) openHelp() InstallTailscaleForm {
	panel := NewTailscaleHelpPanel(m.width, m.height)
	m.panel = &panel
	return m
}

func (m InstallTailscaleForm) updateWithPanel(msg tea.Msg) (InstallTailscaleForm, tea.Cmd) {
	switch msg := msg.(type) {
	case TailscaleHelpCloseMsg:
		m.panel = nil
		return m, nil

	case tea.KeyPressMsg:
		if key.Matches(msg, installTailscaleFormKeys.Help) {
			m.panel = nil
			return m, nil
		}
		var cmd tea.Cmd
		panel := *m.panel
		panel, cmd = panel.Update(msg)
		m.panel = &panel
		return m, cmd
	}

	return m, nil
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
