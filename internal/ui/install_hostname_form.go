package ui

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

const tailnetLookupTimeout = 2 * time.Second

type InstallHostnameBackMsg struct{}

type InstallAdvancedMsg struct {
	ImageRef string
	Hostname string
}

type InstallHostnameForm struct {
	form     Form
	imageRef string
	title    string
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

	m := InstallHostnameForm{
		form: NewForm("Install",
			FormItem{
				Label:    label,
				Field:    hostnameField,
				Required: true,
			},
		),
		imageRef: imageRef,
		title:    title,
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

// tailnetMagicDNSSuffix returns the MagicDNS suffix of the tailnet this host
// belongs to, or "" when the tailscale CLI isn't available. It is a variable
// so tests can substitute a stub.
var tailnetMagicDNSSuffix = readTailnetMagicDNSSuffix

// tailnetNamePlaceholder suggests the name a Service for this app would take
// on the tailnet, falling back to the usual hostname when the suffix can't be
// read.
func tailnetNamePlaceholder(imageRef string) string {
	appName := docker.SanitizeDNSLabel(docker.NameFromImageRef(imageRef))
	if appName == "" {
		return "app.example.com"
	}
	if suffix := tailnetMagicDNSSuffix(); suffix != "" {
		return appName + "." + suffix
	}
	return appName + ".example.com"
}

func readTailnetMagicDNSSuffix() string {
	ctx, cancel := context.WithTimeout(context.Background(), tailnetLookupTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
	if err != nil {
		return ""
	}

	var status struct {
		MagicDNSSuffix string `json:"MagicDNSSuffix"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return ""
	}

	return strings.Trim(status.MagicDNSSuffix, ".")
}
