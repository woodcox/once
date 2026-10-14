package ui

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/mouse"
)

const (
	advancedHealthCheckField = iota
	advancedAppPortField
	advancedVolumePathsField
	advancedSkipRailsEnvField
	advancedEnvStart
)

type InstallAdvancedForm struct {
	form     Form
	width    int
	height   int
	scroll   int
	settings docker.ApplicationSettings
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
	}

	keys := slices.Sorted(maps.Keys(settings.EnvVars))
	for _, k := range keys {
		items = append(items, newEnvKeyItem(k), newEnvValueItem(settings.EnvVars[k]))
	}
	items = append(items, newEnvKeyItem(""), newEnvValueItem(""))

	m := InstallAdvancedForm{
		form:     NewForm("Install", items...),
		settings: settings,
	}

	m.form.OnRebuild(func(f *Form) {
		lastKeyIdx := f.ItemCount() - 2
		if lastKeyIdx >= advancedEnvStart && f.TextField(lastKeyIdx).Value() != "" {
			f.AppendItems(newEnvKeyItem(""), newEnvValueItem(""))
		}
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

		s.EnvVars = nil
		for i := advancedEnvStart; i < f.ItemCount(); i += 2 {
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

func (m InstallAdvancedForm) Init() tea.Cmd {
	return m.form.Init()
}

func (m InstallAdvancedForm) Update(msg tea.Msg) (InstallAdvancedForm, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		m.height = wsm.Height
	}

	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)
	m.setFieldWidths()
	m.adjustScroll()
	return m, cmd
}

func (m InstallAdvancedForm) View() string {
	title := Styles.Title.Render("Advanced settings")
	return lipgloss.JoinVertical(lipgloss.Center, title, "", m.renderContent())
}

// Private

func (m InstallAdvancedForm) envRowCount() int {
	return (m.form.ItemCount() - advancedEnvStart) / 2
}

func (m InstallAdvancedForm) columnWidths() (int, int) {
	totalWidth := max(min(m.width, 64), 6)
	keyWidth := totalWidth / 3
	valueWidth := totalWidth - keyWidth - 1
	return keyWidth, valueWidth
}

func (m InstallAdvancedForm) setFieldWidths() {
	keyWidth, valueWidth := m.columnWidths()
	fieldWidth := max(keyWidth+valueWidth-3, 1)

	for _, idx := range []int{advancedHealthCheckField, advancedAppPortField, advancedVolumePathsField} {
		m.form.TextField(idx).SetWidth(fieldWidth)
	}

	for i := advancedEnvStart; i < m.form.ItemCount(); i++ {
		if (i-advancedEnvStart)%2 == 0 {
			m.form.TextField(i).SetWidth(max(keyWidth-4, 1))
		} else {
			m.form.TextField(i).SetWidth(max(valueWidth-4, 1))
		}
	}
}

func (m *InstallAdvancedForm) adjustScroll() {
	maxVisible := m.maxVisibleRows()
	if maxVisible <= 0 {
		return
	}

	focusedRow := m.focusedEnvRow()
	if focusedRow < 0 {
		focusedRow = m.envRowCount() - 1
	}

	if focusedRow < m.scroll {
		m.scroll = focusedRow
	}
	if focusedRow >= m.scroll+maxVisible {
		m.scroll = focusedRow - maxVisible + 1
	}
}

func (m InstallAdvancedForm) focusedEnvRow() int {
	focused := m.form.Focused()
	if focused >= advancedEnvStart && focused < m.form.ItemCount() {
		return (focused - advancedEnvStart) / 2
	}
	return -1
}

func (m InstallAdvancedForm) maxVisibleRows() int {
	if m.height <= 0 {
		return m.envRowCount()
	}
	// Title (2) + four labeled fields (12) + env header (2) + buttons (3) + help (1)
	available := m.height - 20
	rowHeight := 4 // bordered input (3) + gap (1)
	return max(available/rowHeight, 1)
}

func (m InstallAdvancedForm) renderContent() string {
	focused := m.form.Focused()

	var parts []string
	parts = append(parts, m.renderField(advancedHealthCheckField, "Health check path")...)
	parts = append(parts, m.renderField(advancedAppPortField, "App port")...)
	parts = append(parts, m.renderField(advancedVolumePathsField, "Volume paths")...)

	checkboxStyle := Styles.Focus(Styles.Input, focused == advancedSkipRailsEnvField)
	checkbox := mouse.Mark(fieldTarget(advancedSkipRailsEnvField), checkboxStyle.Render(m.form.CheckboxField(advancedSkipRailsEnvField).View()))
	parts = append(parts, Styles.Label.Render("Rails environment"), checkbox, "")

	parts = append(parts, m.renderEnvGrid()...)
	parts = append(parts, m.renderButtons())

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m InstallAdvancedForm) renderField(idx int, label string) []string {
	inputStyle := Styles.Focus(Styles.Input, m.form.Focused() == idx)
	field := mouse.Mark(fieldTarget(idx), inputStyle.Render(m.form.Field(idx).View()))
	return []string{Styles.Label.Render(label), field, ""}
}

func (m InstallAdvancedForm) renderEnvGrid() []string {
	keyWidth, valueWidth := m.columnWidths()

	headerStyle := lipgloss.NewStyle().Bold(true)
	header := lipgloss.JoinHorizontal(lipgloss.Top,
		headerStyle.Width(keyWidth).Render("Environment variables"),
		" ",
		headerStyle.Width(valueWidth).Render(""),
	)

	parts := []string{header, ""}

	maxVisible := m.maxVisibleRows()
	rows := m.envRowCount()
	end := min(m.scroll+maxVisible, rows)

	if m.scroll > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(Colors.Border).
			Render(fmt.Sprintf("↑ %d more above", m.scroll)))
	}

	focused := m.form.Focused()
	for i := m.scroll; i < end; i++ {
		keyIdx := advancedEnvStart + i*2
		valIdx := keyIdx + 1

		keyStyle := Styles.Focus(Styles.Input, focused == keyIdx).Width(keyWidth)
		valueStyle := Styles.Focus(Styles.Input, focused == valIdx).Width(valueWidth)

		keyView := mouse.Mark(fieldTarget(keyIdx), keyStyle.Render(m.form.TextField(keyIdx).View()))
		valueView := mouse.Mark(fieldTarget(valIdx), valueStyle.Render(m.form.TextField(valIdx).View()))

		parts = append(parts, lipgloss.JoinHorizontal(lipgloss.Top, keyView, " ", valueView), "")
	}

	if end < rows {
		parts = append(parts, lipgloss.NewStyle().Foreground(Colors.Border).
			Render(fmt.Sprintf("↓ %d more below", rows-end)))
	}

	return parts
}

func (m InstallAdvancedForm) renderButtons() string {
	focused := m.form.Focused()

	submitButton := mouse.Mark("submit", Styles.Focus(Styles.ButtonPrimary, focused == m.form.ItemCount()).
		Render("Install"))
	cancelButton := mouse.Mark("cancel", Styles.Focus(Styles.Button, focused == m.form.ItemCount()+1).
		Render("Cancel"))

	return lipgloss.JoinHorizontal(lipgloss.Center, submitButton, cancelButton)
}
