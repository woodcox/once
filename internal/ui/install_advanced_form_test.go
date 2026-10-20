package ui

import (
	"strings"
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

func TestInstallAdvancedForm_CoversAppShapeOnly(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{})

	assert.Equal(t, 6, m.form.ItemCount(), "four fixed fields plus one empty env row")
	assert.Equal(t, 0, advancedHealthCheckField)
	assert.Equal(t, 1, advancedAppPortField)
	assert.Equal(t, 2, advancedVolumePathsField)
	assert.Equal(t, 3, advancedSkipRailsEnvField)
	assert.Equal(t, 4, advancedEnvStart)
}

func TestInstallAdvancedForm_HasNoTailscaleFields(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{})
	view := ansi.Strip(m.View())

	assert.NotContains(t, view, "Tailscale")
	assert.NotContains(t, view, "Docktail")
}

func TestInstallAdvancedForm_LeavesTailscaleSettingsAlone(t *testing.T) {
	original := docker.ApplicationSettings{
		Name: "budget",
		Tailscale: docker.TailscaleSettings{
			Enabled:     true,
			Mode:        docker.DocktailMode,
			Service:     "budget",
			ServicePort: 443,
			Path:        "/",
		},
	}

	assert.True(t, original.Equal(advancedFormSubmit(t, NewInstallAdvancedForm(original))))
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

func TestInstallAdvancedForm_FixedFieldFocusLeavesEnvScrollAlone(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{
		EnvVars: map[string]string{
			"AAA": "1",
			"BBB": "2",
			"CCC": "3",
			"DDD": "4",
		},
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})

	for range 10 {
		m, _ = m.Update(keyPressMsg("tab"))
	}
	require.Equal(t, advancedEnvStart+6, m.form.Focused(), "scrolled deep into the env grid")

	for m.form.Focused() != advancedHealthCheckField {
		m, _ = m.Update(keyPressMsg("shift+tab"))
	}
	require.Equal(t, advancedHealthCheckField, m.form.Focused(), "back at the first fixed field")
	assert.Zero(t, m.scroll, "fixed-field focus must not jump the env grid to its last row")
}

func TestInstallAdvancedForm_FitsFieldsAndButtonsAtSmallHeight(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})

	view := ansi.Strip(m.View())

	assert.LessOrEqual(t, strings.Count(view, "\n")+1, 40)
	assert.Contains(t, view, "Environment variables")
	assert.Contains(t, view, "Install")
	assert.Contains(t, view, "Cancel")
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
