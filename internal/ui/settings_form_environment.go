package ui

import (
	"maps"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/basecamp/once/internal/docker"
)

type SettingsFormEnvironment struct {
	settingsFormBase
	width    int
	height   int
	settings docker.ApplicationSettings
}

func NewSettingsFormEnvironment(settings docker.ApplicationSettings) SettingsFormEnvironment {
	var items []FormItem

	keys := slices.Sorted(maps.Keys(settings.EnvVars))
	for _, k := range keys {
		items = append(items, newEnvKeyItem(k), newEnvValueItem(settings.EnvVars[k]))
	}
	items = append(items, newEnvKeyItem(""), newEnvValueItem(""))

	m := SettingsFormEnvironment{
		title:    "Environment",
		form:     NewForm("Done", items...),
		settings: settings,
	}

	m.form.assignRowGroupsFrom(0)

	m.form.OnRebuild(func(f *Form) {
		lastKeyIdx := f.ItemCount() - 2
		if lastKeyIdx >= 0 && f.TextField(lastKeyIdx).Value() != "" {
			f.AppendItems(newEnvKeyItem(""), newEnvValueItem(""))
		}
		f.assignRowGroupsFrom(0)
		m.setFieldWidths()
	})

	m.setFieldWidths()

	m.form.OnSubmit(func(f *Form) tea.Cmd {
		s := settings
		s.EnvVars = nil
		for i := 0; i < f.ItemCount(); i += 2 {
			k := f.TextField(i).Value()
			if k == "" {
				continue
			}
			if s.EnvVars == nil {
				s.EnvVars = make(map[string]string)
			}
			s.EnvVars[k] = f.TextField(i + 1).Value()
		}
		return func() tea.Msg { return SettingsSectionSubmitMsg{Settings: s} }
	})
	m.form.OnCancel(func(f *Form) tea.Cmd {
		return func() tea.Msg { return SettingsSectionCancelMsg{} }
	})

	return m
}

func (m SettingsFormEnvironment) Update(msg tea.Msg) (SettingsSection, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		m.height = wsm.Height
	}

	var cmd tea.Cmd
	m.settingsFormBase, cmd = m.update(msg)
	m.setFieldWidths()
	return m, cmd
}

func (m SettingsFormEnvironment) View() string {
	return m.form.View()
}

// Private

func (m SettingsFormEnvironment) columnWidths() (int, int) {
	totalWidth := max(min(m.width, 64), 6)
	keyWidth := totalWidth / 3
	valueWidth := totalWidth - keyWidth - 1
	return keyWidth, valueWidth
}

func (m SettingsFormEnvironment) setFieldWidths() {
	keyWidth, valueWidth := m.columnWidths()
	for i := 0; i+1 < m.form.ItemCount(); i += 2 {
		m.form.TextField(i).SetWidth(max(keyWidth-4, 1))
		m.form.TextField(i + 1).SetWidth(max(valueWidth-4, 1))
	}
}

// Helpers

func newEnvKeyItem(value string) FormItem {
	f := NewTextField("KEY")
	f.SetValue(value)
	f.SetCharLimit(256)
	return FormItem{Field: f}
}

func newEnvValueItem(value string) FormItem {
	f := NewTextField("value")
	f.SetValue(value)
	f.SetCharLimit(1024)
	return FormItem{Field: f}
}
