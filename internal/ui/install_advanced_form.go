package ui

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/woodcox/once/internal/docker"
	"github.com/woodcox/once/internal/mouse"
)

const (
	advancedAppPortField      = 0
	advancedHealthCheckField  = 1
	advancedVolumePathsField  = 2
	advancedTLSCertPathField  = 3
	advancedTLSKeyPathField   = 4
	advancedSkipRailsEnvField = 5
	advancedEnvStart          = 6
)

type InstallAdvancedForm struct {
	form     Form
	width    int
	height   int
	scroll   int
	settings docker.ApplicationSettings
}

func NewInstallAdvancedForm(settings docker.ApplicationSettings) InstallAdvancedForm {
	appPortField := NewTextField("3000")
	appPortField.SetDigitsOnly(true)
	if settings.AppPort != 0 {
		appPortField.SetValue(strconv.Itoa(settings.AppPort))
	}

	healthCheckField := NewTextField(docker.DefaultHealthCheckPath)
	healthCheckField.SetValue(settings.HealthCheckPath)

	volumePathsField := NewTextField(docker.DefaultVolumePaths[0] + ", " + docker.DefaultVolumePaths[1])
	if len(settings.VolumePaths) > 0 {
		volumePathsField.SetValue(settings.VolumePathsString())
	}

	tlsCertPathField := NewTextField("/path/to/tailscale.crt")
	tlsCertPathField.SetValue(settings.TLSCertPath)

	tlsKeyPathField := NewTextField("/path/to/tailscale.key")
	tlsKeyPathField.SetValue(settings.TLSKeyPath)

	skipRailsEnvField := NewCheckboxField("Skip SECRET_KEY_BASE, VAPID keys, DISABLE_SSL", settings.SkipRailsEnv)

	items := []FormItem{
		{Label: "App port", Field: appPortField},
		{Label: "Health check path", Field: healthCheckField},
		{Label: "Volume paths", Field: volumePathsField},
		{Label: "TLS cert path", Field: tlsCertPathField},
		{Label: "TLS key path", Field: tlsKeyPathField},
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
		s.AppPort, _ = strconv.Atoi(f.TextField(advancedAppPortField).Value())
		s.HealthCheckPath = f.TextField(advancedHealthCheckField).Value()
		if s.HealthCheckPath == docker.DefaultHealthCheckPath {
			s.HealthCheckPath = ""
		}
		volumeStr := f.TextField(advancedVolumePathsField).Value()
		s.VolumePaths = docker.ParseVolumePaths(volumeStr)
		if slices.Equal(s.VolumePaths, docker.DefaultVolumePaths) {
			s.VolumePaths = nil
		}
		s.TLSCertPath = strings.TrimSpace(f.TextField(advancedTLSCertPathField).Value())
		s.TLSKeyPath = strings.TrimSpace(f.TextField(advancedTLSKeyPathField).Value())
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
	totalWidth := keyWidth + valueWidth + 1
	fieldWidth := max(totalWidth-4, 1)
	halfWidth := max((totalWidth-1)/2-4, 1)

	// App port + health check share the env var column widths
	m.form.TextField(advancedAppPortField).SetWidth(max(keyWidth-4, 1))
	m.form.TextField(advancedHealthCheckField).SetWidth(max(valueWidth-4, 1))

	// Volume paths full width
	m.form.TextField(advancedVolumePathsField).SetWidth(fieldWidth)

	// TLS cert + key 50/50
	m.form.TextField(advancedTLSCertPathField).SetWidth(halfWidth)
	m.form.TextField(advancedTLSKeyPathField).SetWidth(halfWidth)

	for i := advancedEnvStart; i < m.form.ItemCount(); i++ {
		envIdx := i - advancedEnvStart
		if envIdx%2 == 0 {
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
	// Title (2) + app port/health check row (3) + volume paths (3) + TLS row (3) +
	// skip rails env (2) + gap (1) + env headers (2) + buttons (3) + button gap (1) + help (1)
	available := m.height - 21
	rowHeight := 4
	visible := available / rowHeight
	return max(visible, 1)
}

func (m InstallAdvancedForm) renderContent() string {
	focused := m.form.Focused()
	keyWidth, valueWidth := m.columnWidths()
	totalWidth := keyWidth + valueWidth + 1
	halfWidth := (totalWidth - 1) / 2

	var parts []string

	// Row 1: App port + Health check path (env var column widths)
	appPortLabel := lipgloss.NewStyle().Width(keyWidth).Render(Styles.Label.Render("App port"))
	healthCheckLabel := lipgloss.NewStyle().Width(valueWidth).Render(Styles.Label.Render("Health check path"))
	parts = append(parts, lipgloss.JoinHorizontal(lipgloss.Top, appPortLabel, " ", healthCheckLabel))

	appPortStyle := Styles.Focus(Styles.Input, focused == advancedAppPortField)
	healthCheckStyle := Styles.Focus(Styles.Input, focused == advancedHealthCheckField)
	appPortView := mouse.Mark(fieldTarget(advancedAppPortField), appPortStyle.Render(m.form.TextField(advancedAppPortField).View()))
	healthCheckView := mouse.Mark(fieldTarget(advancedHealthCheckField), healthCheckStyle.Render(m.form.TextField(advancedHealthCheckField).View()))
	parts = append(parts, lipgloss.JoinHorizontal(lipgloss.Top, appPortView, " ", healthCheckView), "")

	// Row 2: Volume paths (full width)
	parts = append(parts, Styles.Label.Render("Volume paths"))
	volumeStyle := Styles.Focus(Styles.Input, focused == advancedVolumePathsField)
	parts = append(parts, mouse.Mark(fieldTarget(advancedVolumePathsField), volumeStyle.Render(m.form.TextField(advancedVolumePathsField).View())), "")

	// Row 3: TLS cert path + TLS key path (50/50)
	tlsCertLabel := lipgloss.NewStyle().Width(halfWidth).Render(Styles.Label.Render("TLS cert path"))
	tlsKeyLabel := lipgloss.NewStyle().Width(halfWidth).Render(Styles.Label.Render("TLS key path"))
	parts = append(parts, lipgloss.JoinHorizontal(lipgloss.Top, tlsCertLabel, " ", tlsKeyLabel))

	tlsCertStyle := Styles.Focus(Styles.Input, focused == advancedTLSCertPathField)
	tlsKeyStyle := Styles.Focus(Styles.Input, focused == advancedTLSKeyPathField)
	tlsCertView := mouse.Mark(fieldTarget(advancedTLSCertPathField), tlsCertStyle.Render(m.form.TextField(advancedTLSCertPathField).View()))
	tlsKeyView := mouse.Mark(fieldTarget(advancedTLSKeyPathField), tlsKeyStyle.Render(m.form.TextField(advancedTLSKeyPathField).View()))
	parts = append(parts, lipgloss.JoinHorizontal(lipgloss.Top, tlsCertView, " ", tlsKeyView), "")

	// Skip Rails env checkbox
	checkboxStyle := Styles.Focus(Styles.Input, focused == advancedSkipRailsEnvField)
	checkbox := mouse.Mark(fieldTarget(advancedSkipRailsEnvField), checkboxStyle.Render(m.form.CheckboxField(advancedSkipRailsEnvField).View()))
	parts = append(parts, checkbox, "")

	// Env vars grid
	headerStyle := lipgloss.NewStyle().Bold(true)
	keyHeader := headerStyle.Width(keyWidth).Render("Environment variables")
	valueHeader := headerStyle.Width(valueWidth).Render("")
	header := lipgloss.JoinHorizontal(lipgloss.Top, keyHeader, " ", valueHeader)
	parts = append(parts, header, "")

	maxVisible := m.maxVisibleRows()
	rows := m.envRowCount()
	end := min(m.scroll+maxVisible, rows)

	if m.scroll > 0 {
		indicator := lipgloss.NewStyle().Foreground(Colors.Border).
			Render(fmt.Sprintf("↑ %d more above", m.scroll))
		parts = append(parts, indicator)
	}

	for i := m.scroll; i < end; i++ {
		keyIdx := advancedEnvStart + i*2
		valIdx := advancedEnvStart + i*2 + 1

		keyStyle := Styles.Focus(Styles.Input, focused == keyIdx).Width(keyWidth)
		valueStyle := Styles.Focus(Styles.Input, focused == valIdx).Width(valueWidth)

		keyView := mouse.Mark(fieldTarget(keyIdx), keyStyle.Render(m.form.TextField(keyIdx).View()))
		valueView := mouse.Mark(fieldTarget(valIdx), valueStyle.Render(m.form.TextField(valIdx).View()))

		rowView := lipgloss.JoinHorizontal(lipgloss.Top, keyView, " ", valueView)
		parts = append(parts, rowView, "")
	}

	if end < rows {
		remaining := rows - end
		indicator := lipgloss.NewStyle().Foreground(Colors.Border).
			Render(fmt.Sprintf("↓ %d more below", remaining))
		parts = append(parts, indicator)
	}

	submitIdx := m.form.ItemCount()
	cancelIdx := m.form.ItemCount() + 1
	submitButton := mouse.Mark("submit", Styles.Focus(Styles.ButtonPrimary, focused == submitIdx).
		Render("Install"))
	cancelButton := mouse.Mark("cancel", Styles.Focus(Styles.Button, focused == cancelIdx).
		Render("Cancel"))
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, submitButton, cancelButton)
	parts = append(parts, buttons)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}