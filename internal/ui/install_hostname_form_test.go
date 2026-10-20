package ui

import (
	"sync"
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
	stubTailnetMagicDNSSuffix(t, "tail1234.ts.net")

	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "", InstallTargetPublic)
	hostnameFormSendWindowSize(&form)
	view := ansi.Strip(form.View())

	assert.Contains(t, view, "Hostname")
	assert.Contains(t, view, "once-campfire.example.com")
}

func TestInstallHostnameForm_TailnetTargetAsksForATailnetName(t *testing.T) {
	stubTailnetMagicDNSSuffix(t, "tail1234.ts.net")

	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "", InstallTargetTailnet)
	hostnameFormSendWindowSize(&form)
	view := ansi.Strip(form.View())

	assert.Contains(t, view, "Tailnet name")
	assert.Contains(t, view, "once-campfire.tail1234.ts.net")
	assert.NotContains(t, view, "Hostname")
}

func TestInstallHostnameForm_TailnetTargetFallsBackWithoutSuffix(t *testing.T) {
	stubTailnetMagicDNSSuffix(t, "")

	form := NewInstallHostnameForm("ghcr.io/basecamp/once-campfire", "", InstallTargetTailnet)
	hostnameFormSendWindowSize(&form)
	view := ansi.Strip(form.View())

	assert.Contains(t, view, "once-campfire.example.com")
}

func TestTailnetNamePlaceholder(t *testing.T) {
	stubTailnetMagicDNSSuffix(t, "tail1234.ts.net")

	assert.Equal(t, "once-campfire.tail1234.ts.net", tailnetNamePlaceholder("ghcr.io/basecamp/once-campfire:latest"))
	assert.Equal(t, "app.example.com", tailnetNamePlaceholder(""), "no image ref leaves the generic placeholder")
}

func TestTailnetMagicDNSSuffixIsCached(t *testing.T) {
	stubTailnetMagicDNSSuffix(t, "tail1234.ts.net")

	first := tailnetMagicDNSSuffix()
	second := tailnetMagicDNSSuffix()
	third := tailnetMagicDNSSuffix()

	assert.Equal(t, "tail1234.ts.net", first)
	assert.Equal(t, first, second)
	assert.Equal(t, first, third)
}

func TestTailnetMagicDNSSuffixCachesAfterFirstRead(t *testing.T) {
	calls := 0
	suffix := sync.OnceValue(func() string {
		calls++
		return "tail1234.ts.net"
	})
	original := tailnetMagicDNSSuffix
	tailnetMagicDNSSuffix = suffix
	t.Cleanup(func() { tailnetMagicDNSSuffix = original })

	for range 3 {
		_ = tailnetMagicDNSSuffix()
	}
	assert.Equal(t, 1, calls, "the lookup should happen exactly once when cached")
}

// Helpers

func stubTailnetMagicDNSSuffix(t *testing.T, suffix string) {
	t.Helper()

	original := tailnetMagicDNSSuffix
	tailnetMagicDNSSuffix = func() string { return suffix }
	t.Cleanup(func() { tailnetMagicDNSSuffix = original })
}

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
