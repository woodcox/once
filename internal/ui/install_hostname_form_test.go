package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallHostnameForm_Submit(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "")

	hostnameFormTypeText(&form, "chat.example.com")
	hostnameFormPressTab(&form)
	hostnameFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(InstallFormSubmitMsg)
	require.True(t, ok, "expected InstallFormSubmitMsg, got %T", msg)
	assert.Equal(t, "ghcr.io/basecamp/once-campfire", submit.ImageRef)
	assert.Equal(t, "chat.example.com", submit.Hostname)
	assert.False(t, submit.Settings.Tailscale.Enabled)
}

func TestInstallHostnameForm_Cancel(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire:latest", "")

	// Tab to tailscale, submit, action, cancel
	hostnameFormPressTab(&form)
	hostnameFormPressTab(&form)
	hostnameFormPressTab(&form)
	hostnameFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(InstallHostnameBackMsg)
	assert.True(t, ok, "expected InstallHostnameBackMsg, got %T", msg)
}

func TestInstallHostnameForm_AdvancedSettings(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "")

	hostnameFormTypeText(&form, "chat.example.com")

	// Tab to tailscale, submit, action button
	hostnameFormPressTab(&form)
	hostnameFormPressTab(&form)
	hostnameFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg := cmd()
	advMsg, ok := msg.(InstallAdvancedMsg)
	require.True(t, ok, "expected InstallAdvancedMsg, got %T", msg)
	assert.Equal(t, "ghcr.io/basecamp/once-campfire", advMsg.ImageRef)
	assert.Equal(t, "chat.example.com", advMsg.Hostname)
	assert.False(t, advMsg.TailscaleEnabled)
}

func TestInstallHostnameForm_RequiresHostname(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire:latest", "")

	// Tab to tailscale then submit button, then press enter with empty hostname
	hostnameFormPressTab(&form)
	hostnameFormPressTab(&form)
	form, _ = form.Update(keyPressMsg("enter"))
	assert.True(t, form.form.HasError())
}

func TestInstallHostnameForm_Hostname(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire:latest", "")
	hostnameFormTypeText(&form, "app.example.com")
	assert.Equal(t, "app.example.com", form.Hostname())
}

func TestInstallHostnameForm_ShowsTitleWhenSet(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "campfire")
	view := ansi.Strip(form.View())
	assert.Contains(t, view, "Installing campfire")
}

func TestInstallHostnameForm_NoTitleWhenEmpty(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "")
	view := ansi.Strip(form.View())
	assert.NotContains(t, view, "Installing")
}

// Helpers

func hostnameFormTypeText(form *InstallHostnameForm, text string) {
	for _, r := range text {
		*form, _ = form.Update(keyPressMsg(string(r)))
	}
}

func hostnameFormPressTab(form *InstallHostnameForm) {
	*form, _ = form.Update(keyPressMsg("tab"))
}

func TestInstallHostnameForm_SubmitWithTailscaleEnabled(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "")
	hostnameFormTypeText(&form, "chat.example.com")
	hostnameFormPressTab(&form)
	form, _ = form.Update(keyPressMsg(" "))
	hostnameFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)
	msg := cmd()
	submit, ok := msg.(InstallFormSubmitMsg)
	require.True(t, ok)
	assert.True(t, submit.Settings.Tailscale.Enabled)
}
