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

func TestTailscaleHelpPanel_OpensAndListsLabels(t *testing.T) {
	panel := NewTailscaleHelpPanel(80, 24)

	for _, tag := range []string{
		"docktail.service.name",
		"docktail.service.description",
		"docktail.service.port",
		"docktail.service.protocol",
		"docktail.service.service-port",
		"docktail.service.service-protocol",
		"docktail.service.path",
		"docktail.tags",
		"docktail.service.network",
		"docktail.service.proxy-protocol",
		"docktail.service.direct",
	} {
		assert.Contains(t, panel.viewport.GetContent(), tag)
	}

	assert.Contains(t, ansi.Strip(panel.View()), "Tailscale Service labels (docktail)")
	assert.NotEmpty(t, ansi.Strip(panel.View()), "the panel renders an overlay")
}

func TestTailscaleHelpPanel_F1OpensAndToggles(t *testing.T) {
	m := NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{})

	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, cmd := m.Update(keyPressMsg("f1"))
	require.Nil(t, cmd, "F1 must not produce a command on the form")
	require.NotNil(t, m.panel, "F1 opens the panel")

	view := ansi.Strip(m.View())
	assert.Contains(t, view, "Tailscale Service labels (docktail)", "the panel title is visible")
	assert.NotContains(t, view, "docktail.service.name=budget", "the live Labels preview is hidden behind the panel")
	assert.NotContains(t, view, "Install", "the form's submit button is hidden behind the panel")

	m, _ = m.Update(keyPressMsg("f1"))
	assert.Nil(t, m.panel, "F1 closes the panel again")
}

func TestTailscaleHelpPanel_ScrollMovesContentAndIndicator(t *testing.T) {
	panel := NewTailscaleHelpPanel(80, 24)
	assert.Greater(t, panel.viewport.TotalLineCount(), panel.viewport.Height(), "test precondition: content must overflow the panel")

	initial := ansi.Strip(panel.View())
	assert.Contains(t, initial, "1-", "the indicator shows a starting position")
	assert.Contains(t, initial, "more", "the indicator signals there is more below")

	panel, _ = panel.Update(keyPressMsg("down"))
	panel, _ = panel.Update(keyPressMsg("down"))
	panel, _ = panel.Update(keyPressMsg("down"))
	after := ansi.Strip(panel.View())

	assert.Greater(t, panel.viewport.YOffset(), 0, "scrolling down moves the y-offset")
	assert.NotEqual(t, initial, after, "the panel re-renders with new content")
	assert.True(t, strings.Contains(after, "of ") || strings.Contains(initial, "of "), "the position indicator remains visible after scrolling")

	panel, _ = panel.Update(keyPressMsg("home"))
	assert.Zero(t, panel.viewport.YOffset(), "Home returns to the top")
	assert.Equal(t, initial, ansi.Strip(panel.View()))
}

func TestTailscaleHelpPanel_EscClosesWithoutCancel(t *testing.T) {
	m := NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(keyPressMsg("f1"))
	require.NotNil(t, m.panel)

	m, cmd := m.Update(keyPressMsg("esc"))
	require.NotNil(t, cmd, "the panel returns a command that signals it should close")
	msg := cmd()
	closeMsg, ok := msg.(TailscaleHelpCloseMsg)
	require.True(t, ok, "expected TailscaleHelpCloseMsg, got %T", msg)

	m, followup := m.Update(closeMsg)
	assert.Nil(t, m.panel, "TailscaleHelpCloseMsg dismisses the panel")
	assert.Nil(t, followup, "closing the panel must not produce any cancel/back command")
}

func TestTailscaleHelpPanel_PrintableKeyDoesNotModifyField(t *testing.T) {
	m := NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{})
	m.form.TextField(installTailscaleServiceField).SetValue("budget")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = m.Update(keyPressMsg("f1"))
	require.NotNil(t, m.panel)

	for _, key := range []string{"x", "y", "z", "1", "0", "tab"} {
		_, cmd := m.Update(keyPressMsg(key))
		assert.Nil(t, cmd, "a printable key typed into the panel must not produce a command: %s", key)
	}

	assert.Equal(t, "budget", m.form.TextField(installTailscaleServiceField).Value(), "the Service name is untouched")
	assert.NotNil(t, m.panel, "the panel stays open")
}

func TestTailscaleHelpPanel_InstallForm_OmitsInlineDescriptions(t *testing.T) {
	m := NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 34})

	view := ansi.Strip(m.View())

	for _, fragment := range []string{
		"Defaults to the application name",
		"Leave blank to use the application port",
		"loopback only",
	} {
		assert.NotContains(t, view, fragment, "the help panel content must not leak into the form view")
	}

	assert.Contains(t, view, "Service name (required)", "labels and tags stay on the form")
	assert.Contains(t, view, "docktail.service.name", "the docktail label tag stays on the form")
}

func TestTailscaleHelpPanel_SettingsForm_OmitsInlineDescriptions(t *testing.T) {
	form := NewSettingsFormTailscale(docker.ApplicationSettings{
		Name: "budget",
		Tailscale: docker.TailscaleSettings{
			Enabled: true,
			Mode:    docker.DocktailMode,
			Service: "budget",
		},
	})
	updateSettingsForm(&form, tea.WindowSizeMsg{Width: 100, Height: 34})

	view := ansi.Strip(form.View())

	for _, fragment := range []string{
		"Defaults to the application name",
		"loopback only",
	} {
		assert.NotContains(t, view, fragment, "the help panel content must not leak into the form view")
	}

	assert.Contains(t, view, "Service name (required)", "labels and tags stay on the form")
}

func TestTailscaleHelpPanel_ColoursItsEntries(t *testing.T) {
	content := renderTailscaleHelpContent()

	name := docktailHelpHeading(docktailFields[0])
	assert.NotEqual(t, "Service name (required)", name, "the field name is styled, not plain")
	assert.Contains(t, ansi.Strip(name), "Service name (required)")
	assert.Contains(t, name, "\x1b[", "the styled heading carries colour codes")

	optional := docktailHelpHeading(docktailFields[1])
	assert.NotEqual(t, "Description (optional)", optional)
	assert.Contains(t, ansi.Strip(optional), "Description (optional)")

	key := docktailHelpKey("docktail.service.name")
	assert.NotEqual(t, "docktail.service.name", key, "the docktail key is styled in the primary colour")
	assert.Contains(t, ansi.Strip(key), "docktail.service.name")

	note := docktailHelpNote("Default: the application's port.")
	assert.NotEqual(t, "Default: the application's port.", note)
	assert.Contains(t, ansi.Strip(note), "Default: the application's port.")
	assert.Contains(t, strings.SplitN(note, "\x1b[m", 2)[0], "Default", "the label prefix carries the muted style")

	assert.Contains(t, content, "\x1b[", "the panel content is styled throughout")
}

func TestTailscaleHelpPanel_FormLabelsMatchThePanelColours(t *testing.T) {
	form := NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{})
	form, _ = form.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	view := form.View()

	assert.Contains(t, view, "\x1b[", "the form view is styled")
	assert.Contains(t, ansi.Strip(view), "Service name (required)", "the required marker stays with the name")
	assert.Contains(t, ansi.Strip(view), "docktail.service.name", "the docktail key still names the field")

	styledName := docktailHelpHeading(docktailFields[0])
	assert.Contains(t, view, ansi.Strip(styledName), "the form uses the same name text as the panel")
}

func TestTailscaleHelpPanel_FormIsMateriallyShorter(t *testing.T) {
	before := lineCountAt(ansi.Strip(NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{}).View()))

	m := NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	after := lineCountAt(ansi.Strip(m.View()))

	t.Logf("install tailscale view line count: before=%d after=%d (at 100x34)", before, after)

	assert.Less(t, after, before, "dropping the descriptions should make the view shorter")
	assert.LessOrEqual(t, after, 34, "the form must fit a 34-row terminal height")
}
