package ui

import (
	"testing"

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
	view := ansi.Strip(NewInstallWhereForm().View())

	assert.Contains(t, view, "Where should this app run?")
	assert.Contains(t, view, "Public")
	assert.Contains(t, view, "Tailnet only")
	assert.Contains(t, view, "ONCE's proxy", "the selected option is described")
	assert.NotContains(t, view, "No host ports are opened")
}

func TestInstallWhereForm_ViewDescribesTailnet(t *testing.T) {
	form, _ := NewInstallWhereForm().Update(keyPressMsg("down"))

	view := ansi.Strip(form.View())

	assert.Contains(t, view, "No host ports are opened")
	assert.NotContains(t, view, "ONCE's proxy")
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
