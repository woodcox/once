package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallHostnameForm_Submit(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "", InstallTargetPublic)

	hostnameFormTypeText(&form, "chat.example.com")
	hostnameFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg := cmd()
	submit, ok := msg.(InstallFormSubmitMsg)
	require.True(t, ok, "expected InstallFormSubmitMsg, got %T", msg)
	assert.Equal(t, "ghcr.io/basecamp/once-campfire", submit.ImageRef)
	assert.Equal(t, "chat.example.com", submit.Hostname)
}

func TestInstallHostnameForm_Cancel(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire:latest", "", InstallTargetPublic)

	// Tab past submit and the advanced settings action to reach cancel
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
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire:latest", "", InstallTargetPublic)
	hostnameFormTypeText(&form, "app.example.com")

	hostnameFormPressTab(&form)
	hostnameFormPressTab(&form)
	form, cmd := form.Update(keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg := cmd()
	advanced, ok := msg.(InstallAdvancedMsg)
	require.True(t, ok, "expected InstallAdvancedMsg, got %T", msg)
	assert.Equal(t, "ghcr.io/basecamp/once-campfire:latest", advanced.ImageRef)
	assert.Equal(t, "app.example.com", advanced.Hostname)
}

func TestInstallHostnameForm_RequiresHostname(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire:latest", "", InstallTargetPublic)

	// Tab to submit button, then press enter with empty hostname
	hostnameFormPressTab(&form)
	form, _ = form.Update(keyPressMsg("enter"))
	assert.True(t, form.form.HasError())
}

func TestInstallHostnameForm_Hostname(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire:latest", "", InstallTargetPublic)
	hostnameFormTypeText(&form, "app.example.com")
	assert.Equal(t, "app.example.com", form.Hostname())
}

func TestInstallHostnameForm_ShowsTitleWhenSet(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "campfire", InstallTargetPublic)
	view := ansi.Strip(form.View())
	assert.Contains(t, view, "Installing campfire")
}

func TestInstallHostnameForm_NoTitleWhenEmpty(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "", InstallTargetPublic)
	view := ansi.Strip(form.View())
	assert.NotContains(t, view, "Installing")
}

func TestInstallHostnameForm_PublicTargetAsksForAHostname(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "", InstallTargetPublic)
	hostnameFormSendWindowSize(&form)
	view := ansi.Strip(form.View())

	assert.Contains(t, view, "Hostname")
	assert.Contains(t, view, "once-campfire.example.com")
	assert.Contains(t, view, "Install")
}

func TestInstallHostnameForm_TailnetTargetAsksForATailnetName(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "", InstallTargetTailnet)
	hostnameFormSendWindowSize(&form)
	view := ansi.Strip(form.View())

	assert.Contains(t, view, "Tailnet name")
	assert.Contains(t, view, "once-campfire.example.ts.net")
	assert.Contains(t, view, "Tailnet settings", "submit label names the next screen")
	assert.NotContains(t, view, "Install")
	assert.NotContains(t, view, "Next")
	assert.NotContains(t, view, "Hostname")
	assert.NotContains(t, view, "Advanced settings", "the tailnet flow reaches advanced settings from the Service screen")
}

func TestInstallHostnameForm_TailnetFocusOrderSkipsTheActionButton(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "", InstallTargetTailnet)
	hostnameFormTypeText(&form, "app.example.ts.net")

	hostnameFormPressTab(&form)
	require.Equal(t, form.form.ItemCount(), form.form.Focused(), "tab lands on the submit button")

	hostnameFormPressTab(&form)
	require.Equal(t, form.form.ItemCount()+1, form.form.Focused(), "then straight to cancel")
}

func TestInstallHostnameForm_PublicKeepsInstallAndAdvancedSettings(t *testing.T) {
	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "", InstallTargetPublic)
	hostnameFormSendWindowSize(&form)
	view := ansi.Strip(form.View())

	assert.Contains(t, view, "Install")
	assert.Contains(t, view, "Advanced settings")
}

func TestInstallHostnameForm_TailnetPlaceholderIsGeneric(t *testing.T) {
	assert.Equal(t, "once-campfire.example.ts.net", tailnetNamePlaceholder("ghcr.io/basecamp/once-campfire:latest"))
	assert.Equal(t, "app.example.ts.net", tailnetNamePlaceholder(""), "no image ref leaves the generic placeholder")

	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "", InstallTargetTailnet)
	view := ansi.Strip(form.View())
	assert.NotContains(t, view, "lemur-carat", "the placeholder must never leak the live tailnet name")
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

func hostnameFormSendWindowSize(form *InstallHostnameForm) {
	*form, _ = form.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
}
