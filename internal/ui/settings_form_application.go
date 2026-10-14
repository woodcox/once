package ui

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/internal/docker"
)

const (
	appImageField = iota
	appHostnameField
	appTLSField
	appHealthCheckPathField
	appAppPortField
	appVolumePathsField
	appSkipRailsEnvField
	appDocktailEnabledField
	appDocktailModeField
	appDocktailServiceField
	appDocktailPortField
	appDocktailServicePortField
	appDocktailPathField
	appDocktailTagsField
)

type SettingsFormApplication struct {
	settingsFormBase
}

func NewSettingsFormApplication(settings docker.ApplicationSettings) SettingsFormApplication {
	imageField := NewTextField("user/repo:tag")
	imageField.SetValue(settings.Image)

	hostnameField := NewTextField("app.example.com")
	hostnameField.SetValue(settings.Host)

	tlsField := NewCheckboxField("Enabled", !settings.DisableTLS)
	tlsField.SetDisabledWhen(func() (bool, string) {
		if docker.IsLocalhost(hostnameField.Value()) {
			return true, "Not available for localhost"
		}
		return false, ""
	})

	healthCheckField := NewTextField(docker.HealthCheckPath)
	healthCheckField.SetValue(settings.HealthCheckPath)

	appPortField := NewTextField(strconv.Itoa(docker.DefaultAppPort))
	appPortField.SetDigitsOnly(true)
	if settings.AppPort != 0 {
		appPortField.SetValue(strconv.Itoa(settings.AppPort))
	}

	volumePathsField := NewTextField(strings.Join(docker.AppVolumeMountTargets, ", "))
	if len(settings.VolumePaths) > 0 {
		volumePathsField.SetValue(settings.VolumePathsString())
	}

	skipRailsEnvField := NewCheckboxField("Skip Rails environment variables", settings.SkipRailsEnv)
	docktailEnabledField := NewCheckboxField("Publish as a Tailscale Service", settings.DocktailEnabled())

	docktailModeField := NewTextField(docker.DocktailMode)
	docktailModeField.SetValue(settings.Tailscale.Mode)

	docktailServiceField := NewTextField("defaults to the app name")
	docktailServiceField.SetValue(settings.Tailscale.Service)

	docktailPortField := NewTextField("app port")
	docktailPortField.SetDigitsOnly(true)
	if settings.Tailscale.Port != 0 {
		docktailPortField.SetValue(strconv.Itoa(settings.Tailscale.Port))
	}

	docktailServicePortField := NewTextField("443")
	docktailServicePortField.SetDigitsOnly(true)
	if settings.Tailscale.ServicePort != 0 {
		docktailServicePortField.SetValue(strconv.Itoa(settings.Tailscale.ServicePort))
	}

	docktailPathField := NewTextField("/")
	docktailPathField.SetValue(settings.Tailscale.Path)

	docktailTagsField := NewTextField("tag:container")
	docktailTagsField.SetValue(settings.Tailscale.Tags)

	m := SettingsFormApplication{
		title: "Application",
		form: NewForm("Done",
			FormItem{Label: "Image", Field: imageField, Required: true},
			FormItem{Label: "Hostname", Field: hostnameField, Required: true},
			FormItem{Label: "TLS", Field: tlsField},
			FormItem{Label: "Health check path", Field: healthCheckField},
			FormItem{Label: "App port", Field: appPortField},
			FormItem{Label: "Volume paths", Field: volumePathsField},
			FormItem{Label: "Rails environment", Field: skipRailsEnvField},
			FormItem{Label: "Tailscale Service", Field: docktailEnabledField},
			FormItem{Label: "Docktail mode", Field: docktailModeField},
			FormItem{Label: "Service name", Field: docktailServiceField},
			FormItem{Label: "Backend port", Field: docktailPortField},
			FormItem{Label: "Service port", Field: docktailServicePortField},
			FormItem{Label: "Service path", Field: docktailPathField},
			FormItem{Label: "Service tags", Field: docktailTagsField},
		),
	}

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := settings
		s.Image = f.TextField(appImageField).Value()
		s.Host = f.TextField(appHostnameField).Value()
		s.DisableTLS = !f.CheckboxField(appTLSField).Checked()

		s.HealthCheckPath = f.TextField(appHealthCheckPathField).Value()
		if s.HealthCheckPath == docker.HealthCheckPath {
			s.HealthCheckPath = ""
		}

		s.AppPort, _ = strconv.Atoi(f.TextField(appAppPortField).Value())

		s.VolumePaths = docker.ParseVolumePaths(f.TextField(appVolumePathsField).Value())
		if slices.Equal(s.VolumePaths, docker.AppVolumeMountTargets) {
			s.VolumePaths = nil
		}

		s.SkipRailsEnv = f.CheckboxField(appSkipRailsEnvField).Checked()

		s.Tailscale.Enabled = f.CheckboxField(appDocktailEnabledField).Checked()
		s.Tailscale.Mode = f.TextField(appDocktailModeField).Value()
		if s.Tailscale.Enabled && s.Tailscale.Mode == "" {
			s.Tailscale.Mode = docker.DocktailMode
		}
		s.Tailscale.Service = f.TextField(appDocktailServiceField).Value()
		s.Tailscale.Port, _ = strconv.Atoi(f.TextField(appDocktailPortField).Value())
		s.Tailscale.ServicePort, _ = strconv.Atoi(f.TextField(appDocktailServicePortField).Value())
		s.Tailscale.Path = f.TextField(appDocktailPathField).Value()
		s.Tailscale.Tags = f.TextField(appDocktailTagsField).Value()

		return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m SettingsFormApplication) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	var cmd tea.Cmd
	m.settingsFormBase, cmd = m.update(msg)
	return m, cmd
}
