package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

type InstallHostnameBackMsg struct{}

type InstallAdvancedMsg struct {
	ImageRef string
	Hostname string
}

type InstallHostnameForm struct {
	form     Form
	imageRef string
	title    string
	target   InstallTarget
}

func NewInstallHostnameForm(imageRef, title string, target InstallTarget) InstallHostnameForm {
	label := "Hostname"
	placeholder := "app.example.com"
	if appName := docker.NameFromImageRef(imageRef); appName != "" {
		placeholder = appName + ".example.com"
	}
	if target == InstallTargetTailnet {
		label = "Tailnet name"
		placeholder = tailnetNamePlaceholder(imageRef)
	}

	hostnameField := NewTextField(placeholder)

	submitLabel := "Install"
	if target == InstallTargetTailnet {
		submitLabel = "Next"
	}

	m := InstallHostnameForm{
		form: NewForm(submitLabel,
			FormItem{
				Label:    label,
				Field:    hostnameField,
				Required: true,
			},
		),
		imageRef: imageRef,
		title:    title,
		target:   target,
	}

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		return func() tea.Msg {
			return InstallFormSubmitMsg{
				ImageRef: imageRef,
				Hostname: f.TextField(0).Value(),
			}
		}
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return InstallHostnameBackMsg{} }
	})
	m.form.SetActionButton("Advanced settings", func() tea.Msg {
		return InstallAdvancedMsg{
			ImageRef: imageRef,
			Hostname: hostnameField.Value(),
		}
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

// Helpers

// tailnetNamePlaceholder suggests the name a Service for this app would take
// on a generic tailnet. The example suffix keeps the live tailnet's MagicDNS
// value out of the screen: the real net varies by user and the form always
// starts empty.
func tailnetNamePlaceholder(imageRef string) string {
	appName := docker.SanitizeDNSLabel(docker.NameFromImageRef(imageRef))
	if appName == "" {
		return "app.example.ts.net"
	}
	return appName + ".example.ts.net"
}
