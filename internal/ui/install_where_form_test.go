package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestInstallWhereForm_DefaultsToPublic(t *testing.T) {
	msg, ok := installWhereFormSelect(NewInstallWhereForm())
	require.True(t, ok, "expected InstallWhereSelectedMsg, got %T", msg)
	assert.Equal(t, InstallTargetPublic, msg.Target)
}

func TestInstallWhereForm_SelectTailnet(t *testing.T) {
	form := NewInstallWhereForm()
	form, _ = form.Update(keyPressMsg("down"))

	msg, ok := installWhereFormSelect(form)
	require.True(t, ok, "expected InstallWhereSelectedMsg, got %T", msg)
	assert.Equal(t, InstallTargetTailnet, msg.Target)
}

func TestInstallWhereForm_View(t *testing.T) {
	form, _ := NewInstallWhereForm().Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	view := ansi.Strip(form.View())

	assert.Contains(t, view, "Where should this app run?")
	assert.Contains(t, view, "Public")
	assert.Contains(t, view, "Tailnet only")
	assert.Contains(t, view, "ONCE's proxy", "the selected option is described")
	assert.NotContains(t, view, "Tailscale Service", "the unselected option's description is hidden")
}

func TestInstallWhereForm_ViewDescribesTailnet(t *testing.T) {
	form, _ := NewInstallWhereForm().Update(keyPressMsg("down"))
	form, _ = form.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	view := ansi.Strip(form.View())

	assert.Contains(t, view, "Tailscale Service")
	assert.Contains(t, view, "through the Service", "the new wording explains no off-host ports are reachable")
	assert.NotContains(t, view, "ONCE's proxy")
}

func TestInstallWhereForm_DescriptionWraps(t *testing.T) {
	plain := NewInstallWhereForm()
	plain, _ = plain.Update(tea.WindowSizeMsg{Width: 80, Height: 40})

	tailnet := NewInstallWhereForm()
	tailnet, _ = tailnet.Update(keyPressMsg("down"))
	tailnet, _ = tailnet.Update(tea.WindowSizeMsg{Width: 80, Height: 40})

	boxWidth := borderedBoxWidth(t, plain.View())
	assert.LessOrEqual(t, boxWidth, installWhereInnerWidth+10, "the box stays narrow regardless of source description length")
	boxWidth = borderedBoxWidth(t, tailnet.View())
	assert.LessOrEqual(t, boxWidth, installWhereInnerWidth+10)

	stripped := ansi.Strip(tailnet.View())
	lines := strings.Split(stripped, "\n")

	var descLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "Tailscale Service") ||
			strings.Contains(trimmed, "reachable only through") ||
			strings.Contains(trimmed, "through the Service") {
			descLines = append(descLines, trimmed)
		}
	}
	require.NotEmpty(t, descLines, "found the tailnet description in the view")

	maxLine := installWhereInnerWidth + 2 + 8
	for _, line := range descLines {
		assert.LessOrEqual(t, lipgloss.Width(line), maxLine, "each description line fits within the wrap column plus borders and padding")
	}
}

func TestInstallWhereForm_BoxIsCentredHorizontally(t *testing.T) {
	form, _ := NewInstallWhereForm().Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	view := ansi.Strip(form.View())
	first, _, _ := strings.Cut(view, "\n")

	boxStart := lipgloss.Width(first[:strings.Index(first, "╭")])
	boxEnd := lipgloss.Width(first[:strings.LastIndex(first, "╮")+len("╮")])
	boxWidth := boxEnd - boxStart
	assert.Equal(t, boxStart, (100-boxWidth)/2, "the box's left edge sits centred within the 100-column terminal")
}

func TestInstallTarget_Apply(t *testing.T) {
	settings := docker.ApplicationSettings{Name: "budget", AppPort: 5006}

	tailnet := InstallTargetTailnet.Apply(settings)
	assert.True(t, tailnet.DocktailEnabled())
	assert.Equal(t, docker.DocktailMode, tailnet.Tailscale.Mode)
	assert.Equal(t, docker.TailscaleSettings{Enabled: true, Mode: docker.DocktailMode}, tailnet.Tailscale)

	public := InstallTargetPublic.Apply(tailnet)
	assert.False(t, public.DocktailEnabled())
	assert.Equal(t, docker.TailscaleSettings{}, public.Tailscale, "a public app carries no Service settings")
}

// Helpers

// installWhereFormSelect presses enter and feeds the menu's selection back to
// the form, which is what turns it into an InstallWhereSelectedMsg.
func installWhereFormSelect(form InstallWhereForm) (InstallWhereSelectedMsg, bool) {
	_, cmd := form.Update(keyPressMsg("enter"))
	if cmd == nil {
		return InstallWhereSelectedMsg{}, false
	}

	_, cmd = form.Update(cmd().(MenuSelectMsg))
	if cmd == nil {
		return InstallWhereSelectedMsg{}, false
	}

	return cmd().(InstallWhereSelectedMsg), true
}

// borderedBoxWidth measures a rendered where-form view from the first `╭` to
// the matching `╮` on the box's top border, in display columns rather than
// raw bytes (the dash characters are three UTF-8 bytes each).
func borderedBoxWidth(t *testing.T, view string) int {
	t.Helper()

	for line := range strings.SplitSeq(ansi.Strip(view), "\n") {
		boxStart := strings.Index(line, "╭")
		boxEnd := strings.LastIndex(line, "╮")
		if boxStart < 0 || boxEnd <= boxStart {
			continue
		}
		left := lipgloss.Width(line[:boxStart])
		right := lipgloss.Width(line[:boxEnd+len("╮")])
		return right - left
	}
	t.Fatalf("no box border found in view:\n%s", view)
	return 0
}
