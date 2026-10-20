package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestInstallTailscaleForm_Defaults(t *testing.T) {
	m := NewInstallTailscaleForm("ghcr.io/basecamp/once-campfire", 0, docker.TailscaleSettings{})

	assert.Equal(t, "once-campfire", m.form.TextField(installTailscaleServiceField).Value())
	assert.Empty(t, m.form.TextField(installTailscaleDescriptionField).Value())
	assert.Empty(t, m.form.TextField(installTailscalePortField).Value(), "empty means the app port")
	assert.Empty(t, m.form.TextField(installTailscaleBackendProtocolField).Value())
	assert.Equal(t, "443", m.form.TextField(installTailscaleServicePortField).Value())
	assert.Empty(t, m.form.TextField(installTailscaleServiceProtocolField).Value())
	assert.Equal(t, "/", m.form.TextField(installTailscalePathField).Value())
	assert.Empty(t, m.form.TextField(installTailscaleTagsField).Value())
	assert.Empty(t, m.form.TextField(installTailscaleNetworkField).Value())
	assert.Empty(t, m.form.TextField(installTailscaleProxyProtocolField).Value())
}

func TestInstallTailscaleForm_SubmitDefaults(t *testing.T) {
	settings := installTailscaleFormSubmit(t, NewInstallTailscaleForm("ghcr.io/basecamp/once-campfire", 0, docker.TailscaleSettings{}))

	assert.True(t, settings.Enabled)
	assert.Equal(t, docker.DocktailMode, settings.Mode)
	assert.Equal(t, "once-campfire", settings.Service)
	assert.Zero(t, settings.Port)
	assert.Equal(t, 443, settings.ServicePort)
	assert.Equal(t, "/", settings.Path)
	assert.Empty(t, settings.Tags)
	assert.Empty(t, settings.Description)
	assert.Empty(t, settings.Protocol)
	assert.Empty(t, settings.ServiceProtocol)
	assert.Empty(t, settings.Network)
	assert.Empty(t, settings.ProxyProtocol)
	assert.Nil(t, settings.Direct)
}

func TestInstallTailscaleForm_DefaultsReachTheLabels(t *testing.T) {
	tailscale := installTailscaleFormSubmit(t, NewInstallTailscaleForm("ghcr.io/basecamp/once-campfire", 0, docker.TailscaleSettings{}))
	settings := docker.ApplicationSettings{Name: "once-campfire", AppPort: 5006, Tailscale: tailscale}

	assert.Equal(t, map[string]string{
		"docktail.service.enable":           "true",
		"docktail.funnel.enable":            "false",
		"docktail.service.name":             "once-campfire",
		"docktail.service.port":             "5006",
		"docktail.service.service-port":     "443",
		"docktail.service.protocol":         "http",
		"docktail.service.service-protocol": "https",
		"docktail.service.direct":           "true",
		"docktail.tags":                     "tag:container",
	}, settings.DocktailLabels())
}

func TestInstallTailscaleForm_PrefillsExistingSettings(t *testing.T) {
	direct := false
	original := docker.TailscaleSettings{
		Enabled:         true,
		Mode:            docker.DocktailMode,
		Service:         "budget",
		Description:     "Budget tracker",
		Port:            3000,
		Protocol:        "https+insecure",
		ServicePort:     8443,
		ServiceProtocol: "https",
		Path:            "/budget",
		Tags:            "tag:server",
		Network:         "tailnet0",
		Direct:          &direct,
	}

	m := NewInstallTailscaleForm("example/budget", 3000, original)

	assert.Equal(t, "budget", m.form.TextField(installTailscaleServiceField).Value())
	assert.Equal(t, "Budget tracker", m.form.TextField(installTailscaleDescriptionField).Value())
	assert.Equal(t, "3000", m.form.TextField(installTailscalePortField).Value())
	assert.Equal(t, "https+insecure", m.form.TextField(installTailscaleBackendProtocolField).Value())
	assert.Equal(t, "8443", m.form.TextField(installTailscaleServicePortField).Value())
	assert.Equal(t, "https", m.form.TextField(installTailscaleServiceProtocolField).Value())
	assert.Equal(t, "/budget", m.form.TextField(installTailscalePathField).Value())
	assert.Equal(t, "tag:server", m.form.TextField(installTailscaleTagsField).Value())
	assert.Equal(t, "tailnet0", m.form.TextField(installTailscaleNetworkField).Value())

	assert.True(t, original.Equal(installTailscaleFormSubmit(t, m)))
}

func TestInstallTailscaleForm_ProxyProtocolRoundTrip(t *testing.T) {
	original := docker.TailscaleSettings{
		Enabled:         true,
		Mode:            docker.DocktailMode,
		Service:         "postgres",
		Port:            5432,
		Protocol:        "tcp",
		ServicePort:     5432,
		ServiceProtocol: "tcp",
		Path:            "/",
		ProxyProtocol:   "2",
	}

	m := NewInstallTailscaleForm("example/postgres", 5432, original)

	assert.Equal(t, "2", m.form.TextField(installTailscaleProxyProtocolField).Value())
	assert.True(t, original.Equal(installTailscaleFormSubmit(t, m)))
	assert.Equal(t, "2", installTailscaleFormSubmit(t, m).ProxyProtocol)
}

func TestInstallTailscaleForm_DirectRoundTrip(t *testing.T) {
	direct := true

	t.Run("nil Direct is preserved through a settings round-trip", func(t *testing.T) {
		m := NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{
			Enabled: true,
			Mode:    docker.DocktailMode,
			Service: "budget",
		})
		assert.Nil(t, installTailscaleFormSubmit(t, m).Direct)
	})

	t.Run("explicit Direct=true is preserved through a settings round-trip", func(t *testing.T) {
		m := NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{
			Enabled: true,
			Mode:    docker.DocktailMode,
			Service: "budget",
			Direct:  &direct,
		})
		direct := installTailscaleFormSubmit(t, m).Direct
		require.NotNil(t, direct)
		assert.True(t, *direct)
	})
}

func TestInstallTailscaleForm_ShowsTheLabelsItWillSet(t *testing.T) {
	m := NewInstallTailscaleForm("example/budget", 3000, docker.TailscaleSettings{
		Service: "budget",
		Path:    "/budget",
		Tags:    "tag:server",
	})

	m.form.TextField(installTailscaleNetworkField).SetValue("tailnet0")
	m, _ = m.Update(keyPressMsg("x"))

	view := ansi.Strip(m.form.View())

	assert.Contains(t, view, "Labels")
	assert.Contains(t, view, "docktail.service.enable=true")
	assert.Contains(t, view, "docktail.service.name=budgetx")
	assert.Contains(t, view, "docktail.service.port=3000")
	assert.Contains(t, view, "docktail.service.service-port=443")
	assert.Contains(t, view, "docktail.service.path=/budget")
	assert.Contains(t, view, "docktail.tags=tag:server")
	assert.Contains(t, view, "docktail.service.network=tailnet0")
	assert.Contains(t, view, "docktail.service.direct=true")
}

func TestInstallTailscaleForm_LabelPreviewFollowsEdits(t *testing.T) {
	m := NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{Service: "budget"})
	assert.NotContains(t, ansi.Strip(m.form.View()), "docktail.service.description=")

	m.form.TextField(installTailscaleDescriptionField).SetValue("Budget tracker")
	m.form.TextField(installTailscaleServiceProtocolField).SetValue("tcp")
	m.form.TextField(installTailscaleProxyProtocolField).SetValue("2")
	m, _ = m.Update(keyPressMsg("x"))

	view := ansi.Strip(m.form.View())
	assert.Contains(t, view, "docktail.service.description=Budget tracker")
	assert.Contains(t, view, "docktail.service.service-protocol=tcp")
	assert.Contains(t, view, "docktail.service.proxy-protocol=2")
}

func TestInstallTailscaleForm_Cancel(t *testing.T) {
	m := NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{})

	for m.form.Focused() != m.form.ItemCount()+1 {
		m, _ = m.Update(keyPressMsg("tab"))
	}

	_, cmd := m.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	_, ok := cmd().(InstallTailscaleBackMsg)
	assert.True(t, ok)
}

func TestInstallTailscaleForm_View(t *testing.T) {
	m := NewInstallTailscaleForm("example/budget", 0, docker.TailscaleSettings{})
	view := ansi.Strip(m.View())

	assert.Contains(t, view, "Tailscale Service")
	assert.Contains(t, view, "Service name")
	assert.Contains(t, view, "Description")
	assert.Contains(t, view, "Backend port")
	assert.Contains(t, view, "Backend protocol")
	assert.Contains(t, view, "Service port")
	assert.Contains(t, view, "Service protocol")
	assert.Contains(t, view, "Service path")
	assert.Contains(t, view, "Service tags")
	assert.Contains(t, view, "Network")
	assert.Contains(t, view, "Proxy protocol")
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
