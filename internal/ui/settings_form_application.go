package ui

import (
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/woodcox/once/internal/docker"
)

const (
	appImageField = iota
	appHostnameField
	appTLSField
	appHealthCheckPathField
	appTLSCertPathField
	appTLSKeyPathField
	appAppPortField
	appVolumePathsField
	appSkipRailsEnvField
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

	healthCheckField := NewTextField(docker.DefaultHealthCheckPath)
	healthCheckField.SetValue(settings.HealthCheckPath)

	tlsCertPathField := NewTextField("/path/to/tailscale.crt")
	tlsCertPathField.SetValue(settings.TLSCertPath)

	tlsKeyPathField := NewTextField("/path/to/tailscale.key")
	tlsKeyPathField.SetValue(settings.TLSKeyPath)

	appPortField := NewTextField("3000")
	appPortField.SetDigitsOnly(true)
	if settings.AppPort != 0 {
		appPortField.SetValue(strconv.Itoa(settings.AppPort))
	}

	volumePathsField := NewTextField(docker.DefaultVolumePaths[0] + ", " + docker.DefaultVolumePaths[1])
	if len(settings.VolumePaths) > 0 {
		volumePathsField.SetValue(settings.VolumePathsString())
	}

	skipRailsEnvField := NewCheckboxField("Skip SECRET_KEY_BASE, VAPID keys, DISABLE_SSL", settings.SkipRailsEnv)

	m := SettingsFormApplication{
		settingsFormBase: settingsFormBase{
			title: "Application",
			form: NewForm("Done",
				FormItem{Label: "Image", Field: imageField, Required: true},
				FormItem{Label: "Hostname", Field: hostnameField, Required: true},
				FormItem{Label: "TLS", Field: tlsField},
				FormItem{Label: "Health check path", Field: healthCheckField},
				FormItem{Label: "TLS cert path", Field: tlsCertPathField},
				FormItem{Label: "TLS key path", Field: tlsKeyPathField},
				FormItem{Label: "App port", Field: appPortField},
				FormItem{Label: "Volume paths", Field: volumePathsField},
				FormItem{Label: "Rails environment", Field: skipRailsEnvField},
			),
		},
	}

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := settings
		s.Image = f.TextField(appImageField).Value()
		s.Host = f.TextField(appHostnameField).Value()
		s.DisableTLS = !f.CheckboxField(appTLSField).Checked()

		s.HealthCheckPath = f.TextField(appHealthCheckPathField).Value()
		if s.HealthCheckPath == docker.DefaultHealthCheckPath {
			s.HealthCheckPath = ""
		}
		s.AppPort, _ = strconv.Atoi(f.TextField(appAppPortField).Value())

		volumeStr := f.TextField(appVolumePathsField).Value()
		s.VolumePaths = docker.ParseVolumePaths(volumeStr)
		if slices.Equal(s.VolumePaths, docker.DefaultVolumePaths) {
			s.VolumePaths = nil
		}

		s.SkipRailsEnv = f.CheckboxField(appSkipRailsEnvField).Checked()

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
