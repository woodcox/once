package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestInstallAdvancedForm_SubmitDefaults(t *testing.T) {
	settings := advancedFormSubmit(t, NewInstallAdvancedForm(docker.ApplicationSettings{}))

	assert.Empty(t, settings.HealthCheckPath)
	assert.Zero(t, settings.AppPort)
	assert.Nil(t, settings.VolumePaths)
	assert.False(t, settings.SkipRailsEnv)
	assert.Nil(t, settings.EnvVars)
}

func TestInstallAdvancedForm_SubmitCustomValues(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{})
	m.form.TextField(advancedHealthCheckField).SetValue("/healthz")
	m.form.TextField(advancedAppPortField).SetValue("5006")
	m.form.TextField(advancedVolumePathsField).SetValue("/data")
	m.form.CheckboxField(advancedSkipRailsEnvField).Toggle()
	m.form.TextField(advancedEnvStart).SetValue("TZ")
	m.form.TextField(advancedEnvStart + 1).SetValue("UTC")

	settings := advancedFormSubmit(t, m)

	assert.Equal(t, "/healthz", settings.HealthCheckPath)
	assert.Equal(t, 5006, settings.AppPort)
	assert.Equal(t, []string{"/data"}, settings.VolumePaths)
	assert.True(t, settings.SkipRailsEnv)
	assert.Equal(t, map[string]string{"TZ": "UTC"}, settings.EnvVars)
}

func TestInstallAdvancedForm_PrefillsExistingSettings(t *testing.T) {
	original := docker.ApplicationSettings{
		HealthCheckPath: "/healthz",
		AppPort:         5006,
		VolumePaths:     []string{"/data"},
		SkipRailsEnv:    true,
		EnvVars:         map[string]string{"TZ": "UTC"},
	}

	m := NewInstallAdvancedForm(original)

	assert.Equal(t, "/healthz", m.form.TextField(advancedHealthCheckField).Value())
	assert.Equal(t, "5006", m.form.TextField(advancedAppPortField).Value())
	assert.Equal(t, "/data", m.form.TextField(advancedVolumePathsField).Value())
	assert.True(t, m.form.CheckboxField(advancedSkipRailsEnvField).Checked())
	assert.Equal(t, "TZ", m.form.TextField(advancedEnvStart).Value())
	assert.Equal(t, "UTC", m.form.TextField(advancedEnvStart+1).Value())

	assert.True(t, original.Equal(advancedFormSubmit(t, m)))
}

func TestInstallAdvancedForm_DefaultVolumePathsSubmitAsUnset(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{})
	m.form.TextField(advancedVolumePathsField).SetValue("/storage, /rails/storage")

	settings := advancedFormSubmit(t, m)

	assert.Nil(t, settings.VolumePaths)
}

func TestInstallAdvancedForm_GrowsEnvRows(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	initial := m.form.ItemCount()

	for range advancedEnvStart {
		m, _ = m.Update(keyPressMsg("tab"))
	}
	require.Equal(t, advancedEnvStart, m.form.Focused())

	m.form.TextField(advancedEnvStart).SetValue("TZ")
	m, _ = m.Update(keyPressMsg("Z"))

	assert.Equal(t, initial+2, m.form.ItemCount(), "typing a new key appends an empty row")
}

func TestInstallAdvancedForm_SubmitDocktailValues(t *testing.T) {
	original := docker.ApplicationSettings{Name: "budget", AppPort: 5006}

	m := NewInstallAdvancedForm(original)
	m.form.CheckboxField(advancedDocktailEnabledField).Toggle()
	m.form.TextField(advancedDocktailServiceField).SetValue("Budget")
	m.form.TextField(advancedDocktailServicePortField).SetValue("8443")
	m.form.TextField(advancedDocktailPathField).SetValue("/budget")
	m.form.TextField(advancedDocktailTagsField).SetValue("tag:server")

	settings := advancedFormSubmit(t, m)

	assert.True(t, settings.DocktailEnabled())
	assert.Equal(t, docker.DocktailMode, settings.Tailscale.Mode, "the mode defaults to docktail")
	assert.Equal(t, "Budget", settings.Tailscale.Service, "the typed name is kept as entered")
	assert.Equal(t, "budget", settings.EffectiveServiceName(), "the service name is sanitized for the tailnet")
	assert.Equal(t, 8443, settings.Tailscale.ServicePort)
	assert.Equal(t, "/budget", settings.Tailscale.Path)
	assert.Equal(t, "tag:server", settings.Tailscale.Tags)
	assert.Equal(t, map[string]string{
		"docktail.service.enable":       "true",
		"docktail.service.name":         "budget",
		"docktail.service.port":         "5006",
		"docktail.service.service-port": "8443",
		"docktail.service.path":         "/budget",
		"docktail.tags":                 "tag:server",
	}, settings.DocktailLabels())
}

func TestInstallAdvancedForm_DocktailPrefillsAndRoundTrips(t *testing.T) {
	original := docker.ApplicationSettings{
		Name:    "budget",
		AppPort: 5006,
		Tailscale: docker.TailscaleSettings{
			Enabled:     true,
			Mode:        docker.DocktailMode,
			Service:     "budget",
			Port:        3000,
			ServicePort: 443,
			Path:        "/",
			Tags:        "tag:server",
		},
	}

	m := NewInstallAdvancedForm(original)

	assert.True(t, m.form.CheckboxField(advancedDocktailEnabledField).Checked())
	assert.Equal(t, docker.DocktailMode, m.form.TextField(advancedDocktailModeField).Value())
	assert.Equal(t, "budget", m.form.TextField(advancedDocktailServiceField).Value())
	assert.Equal(t, "3000", m.form.TextField(advancedDocktailPortField).Value())
	assert.Equal(t, "443", m.form.TextField(advancedDocktailServicePortField).Value())
	assert.Equal(t, "/", m.form.TextField(advancedDocktailPathField).Value())
	assert.Equal(t, "tag:server", m.form.TextField(advancedDocktailTagsField).Value())

	assert.True(t, original.Equal(advancedFormSubmit(t, m)))
}

func TestInstallAdvancedForm_Cancel(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{})

	for m.form.Focused() != m.form.ItemCount()+1 {
		m, _ = m.Update(keyPressMsg("tab"))
	}

	_, cmd := m.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	_, ok := cmd().(SettingsSectionCancelMsg)
	assert.True(t, ok)
}

func TestInstallAdvancedForm_View(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})

	view := ansi.Strip(m.View())

	assert.Contains(t, view, "Advanced settings")
	assert.Contains(t, view, "Health check path")
	assert.Contains(t, view, "App port")
	assert.Contains(t, view, "Volume paths")
	assert.Contains(t, view, "Environment variables")
	assert.Contains(t, view, "Install")
}

// Helpers

func advancedFormSubmit(t *testing.T, m InstallAdvancedForm) docker.ApplicationSettings {
	t.Helper()

	for m.form.Focused() != m.form.ItemCount() {
		m, _ = m.Update(keyPressMsg("tab"))
	}

	_, cmd := m.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg, ok := cmd().(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	return msg.Settings
}
