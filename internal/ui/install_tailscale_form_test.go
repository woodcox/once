package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestInstallTailscaleForm_Defaults(t *testing.T) {
	m := NewInstallTailscaleForm("ghcr.io/basecamp/once-campfire", docker.TailscaleSettings{})

	assert.Equal(t, "once-campfire", m.form.TextField(installTailscaleServiceField).Value())
	assert.Empty(t, m.form.TextField(installTailscalePortField).Value(), "empty means the app port")
	assert.Equal(t, "443", m.form.TextField(installTailscaleServicePortField).Value())
	assert.Equal(t, "/", m.form.TextField(installTailscalePathField).Value())
	assert.Empty(t, m.form.TextField(installTailscaleTagsField).Value())
}

func TestInstallTailscaleForm_SubmitDefaults(t *testing.T) {
	settings := installTailscaleFormSubmit(t, NewInstallTailscaleForm("ghcr.io/basecamp/once-campfire", docker.TailscaleSettings{}))

	assert.True(t, settings.Enabled)
	assert.Equal(t, docker.DocktailMode, settings.Mode)
	assert.Equal(t, "once-campfire", settings.Service)
	assert.Zero(t, settings.Port)
	assert.Equal(t, 443, settings.ServicePort)
	assert.Equal(t, "/", settings.Path)
	assert.Empty(t, settings.Tags)
}

func TestInstallTailscaleForm_DefaultsReachTheLabels(t *testing.T) {
	tailscale := installTailscaleFormSubmit(t, NewInstallTailscaleForm("ghcr.io/basecamp/once-campfire", docker.TailscaleSettings{}))
	settings := docker.ApplicationSettings{Name: "once-campfire", AppPort: 5006, Tailscale: tailscale}

	assert.Equal(t, map[string]string{
		"docktail.service.enable":       "true",
		"docktail.service.name":         "once-campfire",
		"docktail.service.port":         "5006",
		"docktail.service.service-port": "443",
	}, settings.DocktailLabels())
}

func TestInstallTailscaleForm_PrefillsExistingSettings(t *testing.T) {
	original := docker.TailscaleSettings{
		Enabled:     true,
		Mode:        docker.DocktailMode,
		Service:     "budget",
		Port:        3000,
		ServicePort: 8443,
		Path:        "/budget",
		Tags:        "tag:server",
	}

	m := NewInstallTailscaleForm("example/budget", original)

	assert.Equal(t, "budget", m.form.TextField(installTailscaleServiceField).Value())
	assert.Equal(t, "3000", m.form.TextField(installTailscalePortField).Value())
	assert.Equal(t, "8443", m.form.TextField(installTailscaleServicePortField).Value())
	assert.Equal(t, "/budget", m.form.TextField(installTailscalePathField).Value())
	assert.Equal(t, "tag:server", m.form.TextField(installTailscaleTagsField).Value())

	assert.Equal(t, original, installTailscaleFormSubmit(t, m))
}

func TestInstallTailscaleForm_Cancel(t *testing.T) {
	m := NewInstallTailscaleForm("example/budget", docker.TailscaleSettings{})

	for m.form.Focused() != m.form.ItemCount()+1 {
		m, _ = m.Update(keyPressMsg("tab"))
	}

	_, cmd := m.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	_, ok := cmd().(InstallTailscaleBackMsg)
	assert.True(t, ok)
}

func TestInstallTailscaleForm_View(t *testing.T) {
	m := NewInstallTailscaleForm("example/budget", docker.TailscaleSettings{})
	view := ansi.Strip(m.View())

	assert.Contains(t, view, "Tailscale Service")
	assert.Contains(t, view, "Service name")
	assert.Contains(t, view, "Backend port")
	assert.Contains(t, view, "Install")
}

// Helpers

func installTailscaleFormSubmit(t *testing.T, m InstallTailscaleForm) docker.TailscaleSettings {
	t.Helper()

	for m.form.Focused() != m.form.ItemCount() {
		m, _ = m.Update(keyPressMsg("tab"))
	}

	_, cmd := m.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg, ok := cmd().(InstallTailscaleSubmitMsg)
	require.True(t, ok, "expected InstallTailscaleSubmitMsg, got %T", msg)
	return msg.Tailscale
}
