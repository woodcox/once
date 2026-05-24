package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/woodcox/once/internal/docker"
)

type InstallHostnameBackMsg struct{}

type InstallAdvancedMsg struct {
	ImageRef string
	Hostname string
	TailscaleEnabled bool
}

type InstallHostnameForm struct {
	form     Form
	imageRef string
	title    string
}

// NewInstallHostnameForm creates an InstallHostnameForm preconfigured for the install hostname screen.
// 
// The form contains a required hostname text field (default "app.example.com") whose placeholder is
// derived from the imageRef when available, and a "Create a tailscale service" checkbox.
// It wires up form actions: submit produces an InstallFormSubmitMsg containing ImageRef, the entered
// hostname, and Tailscale settings; the "Advanced settings" action returns InstallAdvancedMsg with
// ImageRef, the current hostname, and whether tailscale is enabled; cancel returns InstallHostnameBackMsg.
func NewInstallHostnameForm(imageRef, title string) InstallHostnameForm {
	hostnameField := NewTextField("app.example.com")
	appName := docker.NameFromImageRef(imageRef)
	if appName != "" {
		hostnameField.SetPlaceholder(appName + ".example.com")
	}

	tailscaleField := NewCheckboxField("Create a tailscale service", false)

	m := InstallHostnameForm{
		form: NewForm("Install",
			FormItem{
				Label:    "Hostname",
				Field:    hostnameField,
				Required: true,
			},
			FormItem{Label: "Tailscale", Field: tailscaleField},
		),
		imageRef: imageRef,
		title:    title,
	}

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		return func() tea.Msg {
			return InstallFormSubmitMsg{
				ImageRef: imageRef,
				Hostname: f.TextField(0).Value(),
				Settings: docker.ApplicationSettings{Tailscale: docker.TailscaleSettings{Enabled: f.CheckboxField(1).Checked()}},
			}
		}
	})
	m.form.SetActionButton("Advanced settings", func() tea.Msg {
		return InstallAdvancedMsg{
			ImageRef:         imageRef,
			Hostname:         hostnameField.Value(),
			TailscaleEnabled: tailscaleField.Checked(),
		}
	})

	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return InstallHostnameBackMsg{} }
	})

	return m
}

func (m InstallHostnameForm) Init() tea.Cmd {
	return m.form.Init()
}

func (m InstallHostnameForm) Update(msg tea.Msg) (InstallHostnameForm, tea.Cmd) {
	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	return m, cmd
}

func (m InstallHostnameForm) View() string {
	if m.title != "" {
		titleLine := Styles.Title.Render("Installing " + m.title)
		return lipgloss.JoinVertical(lipgloss.Center, titleLine, "", m.form.View())
	}
	return m.form.View()
}

func (m InstallHostnameForm) Hostname() string {
	return m.form.TextField(0).Value()
}
