package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

	assert.Equal(t, 7, m.form.ItemCount(), "four fixed fields, the env heading, and one empty env row")
	assert.Equal(t, 0, advancedHealthCheckField)
	assert.Equal(t, 1, advancedAppPortField)
	assert.Equal(t, 2, advancedVolumePathsField)
	assert.Equal(t, 3, advancedSkipRailsEnvField)
	assert.Equal(t, 5, advancedEnvStart)
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

	for m.form.Focused() != advancedEnvStart {
		m, _ = m.Update(keyPressMsg("tab"))
	}
	require.Equal(t, advancedEnvStart, m.form.Focused())

	m.form.TextField(advancedEnvStart).SetValue("TZ")
	m, _ = m.Update(keyPressMsg("Z"))

	assert.Equal(t, initial+2, m.form.ItemCount(), "typing a new key appends an empty row")
}

func TestInstallAdvancedForm_FixedFieldFocusKeepsTheWholeFormOnScreen(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})

	view := ansi.Strip(m.View())
	assert.Contains(t, view, "Health check path")
	assert.Contains(t, view, "Environment variables")
	assert.NotContains(t, view, "more fields", "everything fits, so nothing is hidden")

	for m.form.Focused() != advancedEnvStart {
		m, _ = m.Update(keyPressMsg("tab"))
	}

	view = ansi.Strip(m.View())
	assert.Contains(t, view, "Health check path", "entering the env grid hides nothing")
	assert.Contains(t, view, "Environment variables")
	assert.NotContains(t, view, "more fields", "no hidden-fields indicator before it overflows")

	for m.form.Focused() != advancedHealthCheckField {
		m, _ = m.Update(keyPressMsg("shift+tab"))
	}
	require.Equal(t, advancedHealthCheckField, m.form.Focused(), "back at the first fixed field")

	view = ansi.Strip(m.View())
	assert.Contains(t, view, "Health check path")
	assert.Contains(t, view, "Environment variables")
}

func TestInstallAdvancedForm_ShortSettingsFitWithoutWindowing(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})

	view := ansi.Strip(m.View())

	assert.Contains(t, view, "Health check path")
	assert.Contains(t, view, "App port")
	assert.Contains(t, view, "Volume paths")
	assert.Contains(t, view, "Rails environment")
	assert.Contains(t, view, "Environment variables")
	assert.Contains(t, view, "Install")
	assert.Contains(t, view, "Cancel")
	assert.NotContains(t, view, "more fields", "everything fits, so no hidden-fields indicator")
}

func TestInstallAdvancedForm_TallSettingsWindowAndKeepFocusVisible(t *testing.T) {
	m := tallAdvancedForm(t)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := ansi.Strip(m.View())
	assert.Contains(t, view, "more fields", "the form windows itself")
	assert.Contains(t, view, "Install", "the buttons stay reachable")
	assert.Contains(t, view, "Cancel")

	for m.form.Focused() != advancedEnvStart+6 {
		m, _ = m.Update(keyPressMsg("tab"))
	}
	require.Equal(t, advancedEnvStart+6, m.form.Focused())

	view = ansi.Strip(m.View())
	assert.Contains(t, view, "2", "the focused env row's value stays on screen")
	assert.Contains(t, view, "Cancel")
	assert.LessOrEqual(t, lipgloss.Height(view), 24, "the window still fits the viewport")
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

func TestInstallAdvancedForm_PrunesEmptiedEnvRows(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{
		EnvVars: map[string]string{
			"AAA": "1",
			"BBB": "2",
		},
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	require.Equal(t, 3, envRowCount(m), "two rows plus the blank one")

	for m.form.Focused() != advancedEnvStart {
		m, _ = m.Update(keyPressMsg("tab"))
	}
	// The row prunes itself the moment both cells are empty.
	clearEnvRow(&m, advancedEnvStart, len("AAA"), len("1"))

	assert.Equal(t, 2, envRowCount(m), "the emptied row disappears as soon as it is edited")
	assert.Equal(t, "BBB", m.form.TextField(advancedEnvStart).Value(), "the next row slides up")
	assert.Equal(t, "2", m.form.TextField(advancedEnvStart+1).Value())
	assert.Equal(t, advancedEnvStart, m.form.Focused(), "focus follows the row into the emptied slot")
}

func TestInstallAdvancedForm_KeepsABlankEnvRowAfterPruning(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{
		EnvVars: map[string]string{"AAA": "1", "BBB": "2"},
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})

	for m.form.Focused() != advancedEnvStart {
		m, _ = m.Update(keyPressMsg("tab"))
	}
	clearEnvRow(&m, advancedEnvStart, len("AAA"), len("1"))
	for m.form.Focused() != advancedEnvStart {
		m, _ = m.Update(keyPressMsg("tab"))
	}
	clearEnvRow(&m, advancedEnvStart, len("BBB"), len("2"))

	rows := envRowCount(m)
	require.Equal(t, 1, rows, "every real row pruned, leaving only the blank add row")

	lastKey := m.form.TextField(m.form.ItemCount() - 2)
	assert.Empty(t, lastKey.Value(), "the trailing row stays blank and ready for a new variable")
	assert.Empty(t, m.form.TextField(m.form.ItemCount()-1).Value())
}

func TestInstallAdvancedForm_PrunesEveryEnvRowButTheBlankOne(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{
		EnvVars: map[string]string{"AAA": "1", "BBB": "2"},
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})

	for m.form.Focused() != advancedEnvStart {
		m, _ = m.Update(keyPressMsg("tab"))
	}
	clearEnvRow(&m, advancedEnvStart, len("AAA"), len("1"))
	for m.form.Focused() != advancedEnvStart {
		m, _ = m.Update(keyPressMsg("tab"))
	}
	clearEnvRow(&m, advancedEnvStart, len("BBB"), len("2"))

	assert.Equal(t, 1, envRowCount(m), "only the blank add row survives")
	assert.Nil(t, advancedFormSubmit(t, m).EnvVars)
}

func TestInstallAdvancedForm_KeepsARowWhoseValueIsStillSet(t *testing.T) {
	m := NewInstallAdvancedForm(docker.ApplicationSettings{
		EnvVars: map[string]string{"AAA": "1", "BBB": "2"},
	})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})

	for m.form.Focused() != advancedEnvStart {
		m, _ = m.Update(keyPressMsg("tab"))
	}
	for range len("AAA") {
		m, _ = m.Update(keyPressMsg("backspace"))
	}

	assert.Equal(t, 3, envRowCount(m), "a row with a value but no key is still a variable")
	assert.Equal(t, "1", m.form.TextField(advancedEnvStart+1).Value())
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

// clearEnvRow backspaces over a row's key and value so the form prunes it.
func clearEnvRow(m *InstallAdvancedForm, keyIdx, keyLen, valueLen int) {
	for range keyLen {
		*m, _ = m.Update(keyPressMsg("backspace"))
	}
	*m, _ = m.Update(keyPressMsg("tab"))
	for range valueLen {
		*m, _ = m.Update(keyPressMsg("backspace"))
	}
}

func envRowCount(m InstallAdvancedForm) int {
	return (m.form.ItemCount() - advancedEnvStart) / 2
}

func tallAdvancedForm(t *testing.T) InstallAdvancedForm {
	t.Helper()

	envVars := make(map[string]string, 12)
	for i := range 12 {
		envVars[fmt.Sprintf("VAR_%02d", i)] = fmt.Sprintf("%d", i)
	}

	m := NewInstallAdvancedForm(docker.ApplicationSettings{EnvVars: envVars})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	return m
}

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
