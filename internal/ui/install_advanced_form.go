package ui

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

const (
	advancedHealthCheckField = iota
	advancedAppPortField
	advancedVolumePathsField
	advancedSkipRailsEnvField
	advancedEnvHeading
	advancedEnvStart
)

type InstallAdvancedForm struct {
	form     Form
	width    int
	settings docker.ApplicationSettings
	envKeys  []string
}

func NewInstallAdvancedForm(settings docker.ApplicationSettings) InstallAdvancedForm {
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

	items := []FormItem{
		{Label: "Health check path", Field: healthCheckField},
		{Label: "App port", Field: appPortField},
		{Label: "Volume paths", Field: volumePathsField},
		{Label: "Rails environment", Field: skipRailsEnvField},
		{Field: NewStaticField("Environment variables", func(s string) string {
			return lipgloss.NewStyle().Bold(true).Render(s)
		})},
	}

	keys := slices.Sorted(maps.Keys(settings.EnvVars))
	for _, k := range keys {
		items = append(items, newEnvKeyItem(k), newEnvValueItem(settings.EnvVars[k]))
	}
	items = append(items, newEnvKeyItem(""), newEnvValueItem(""))

	m := InstallAdvancedForm{
		form:     NewForm("Install", items...),
		settings: settings,
		envKeys:  keys,
	}

	m.form.assignRowGroupsFrom(advancedEnvStart)

	m.form.OnRebuild(func(f *Form) {
		pruneEnvRows(f)
		lastKeyIdx := f.ItemCount() - 2
		if lastKeyIdx >= advancedEnvStart && f.TextField(lastKeyIdx).Value() != "" {
			f.AppendItems(newEnvKeyItem(""), newEnvValueItem(""))
		}
		f.assignRowGroupsFrom(advancedEnvStart)
	})

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := settings

		s.HealthCheckPath = f.TextField(advancedHealthCheckField).Value()
		if s.HealthCheckPath == docker.HealthCheckPath {
			s.HealthCheckPath = ""
		}

		s.AppPort, _ = strconv.Atoi(f.TextField(advancedAppPortField).Value())

		s.VolumePaths = docker.ParseVolumePaths(f.TextField(advancedVolumePathsField).Value())
		if slices.Equal(s.VolumePaths, docker.AppVolumeMountTargets) {
			s.VolumePaths = nil
		}

		s.SkipRailsEnv = f.CheckboxField(advancedSkipRailsEnvField).Checked()

		s.EnvVars = envVarsFromForm(f)

		return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
	})

	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m InstallAdvancedForm) Init() tea.Cmd {
	return m.form.Init()
}

func (m InstallAdvancedForm) Update(msg tea.Msg) (InstallAdvancedForm, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		wsm.Height = formViewportHeight(wsm.Height)
		msg = wsm
	}

	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	m.setFieldWidths()
	return m, cmd
}

func (m InstallAdvancedForm) View() string {
	title := Styles.Title.Render("Advanced settings")
	return lipgloss.JoinVertical(lipgloss.Center, title, "", m.renderContent())
}

// matches reports whether the existing form's pre-filled values are still in
// step with the settings it was built from, so the caller can decide whether
// rebuilding from pendingSettings would discard typed input.
func (m InstallAdvancedForm) matches(settings docker.ApplicationSettings) bool {
	if m.form.ItemCount() == 0 {
		return false
	}
	if m.settings.AppPort != settings.AppPort ||
		m.settings.HealthCheckPath != settings.HealthCheckPath ||
		m.settings.SkipRailsEnv != settings.SkipRailsEnv ||
		!slices.Equal(m.settings.VolumePaths, settings.VolumePaths) {
		return false
	}
	if len(m.envKeys) != len(settings.EnvVars) {
		return false
	}
	for _, k := range m.envKeys {
		if _, ok := settings.EnvVars[k]; !ok {
			return false
		}
	}
	return true
}

// Private

func (m InstallAdvancedForm) columnWidths() (int, int) {
	totalWidth := max(min(m.width, 64), 6)
	keyWidth := totalWidth / 3
	valueWidth := totalWidth - keyWidth - 1
	return keyWidth, valueWidth
}

func (m InstallAdvancedForm) setFieldWidths() {
	keyWidth, valueWidth := m.columnWidths()
	fieldWidth := max(keyWidth+valueWidth-3, 1)

	for i := range advancedEnvHeading {
		if i == advancedSkipRailsEnvField {
			continue
		}
		m.form.TextField(i).SetWidth(fieldWidth)
	}

	for i := advancedEnvStart; i+1 < m.form.ItemCount(); i += 2 {
		m.form.TextField(i).SetWidth(max(keyWidth-4, 1))
		m.form.TextField(i + 1).SetWidth(max(valueWidth-4, 1))
	}
}

// renderContent puts the whole form through Form's own windowing: the four
// fixed fields and the env grid share the screen, and nothing scrolls out of
// reach until the content genuinely doesn't fit.
func (m InstallAdvancedForm) renderContent() string {
	return m.form.View()
}

// Helpers

// pruneEnvRows drops env rows whose key and value are both empty, always
// leaving exactly one blank row at the end to start a new variable in.
func pruneEnvRows(f *Form) {
	rows := (f.ItemCount() - advancedEnvStart) / 2
	empty := make([]bool, f.ItemCount())

	for row := range rows {
		if row == rows-1 {
			break // the trailing blank row stays
		}
		key := advancedEnvStart + row*2
		value := key + 1
		if f.TextField(key).Value() == "" && f.TextField(value).Value() == "" {
			empty[key] = true
			empty[value] = true
		}
	}

	f.PruneItems(func(i int, item FormItem) bool { return empty[i] })
}

func envVarsFromForm(f *Form) map[string]string {
	var vars map[string]string
	for i := advancedEnvStart; i < f.ItemCount(); i += 2 {
		k := f.TextField(i).Value()
		if k == "" {
			continue
		}
		if vars == nil {
			vars = make(map[string]string)
		}
		vars[k] = f.TextField(i + 1).Value()
	}
	return vars
}
