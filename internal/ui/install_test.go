package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestInstall_KnownAppFlow(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Equal(t, installStateAppList, m.state)

	// Select Campfire (first item, already selected)
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	assert.Equal(t, installStateWhere, m.state)

	// Choose where it runs (Public is the default selection)
	installSelectTarget(t, &m, InstallTargetPublic)
	assert.Equal(t, installStateHostname, m.state)

	// Enter hostname and submit
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})
	assert.Equal(t, installStateActivity, m.state)
}

func TestInstall_CustomImageFlow(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Select "Other image..."
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	assert.Equal(t, installStateImageForm, m.state)

	// Submit image
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest"})
	assert.Equal(t, installStateWhere, m.state)

	installSelectTarget(t, &m, InstallTargetPublic)

	// Submit hostname
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)
}

func TestInstall_CustomImageWithRegistryCredentials(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{
		ImageRef:         "registry.example.com/app:latest",
		RegistryUsername: "user",
		RegistryPassword: "pass",
	})
	installSelectTarget(t, &m, InstallTargetPublic)

	expected := docker.RegistrySettings{Host: "registry.example.com", Username: "user", Password: "pass"}
	assert.Equal(t, expected, m.registry)

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "registry.example.com/app:latest", Hostname: "app.example.com"})
	assert.Equal(t, expected, m.activity.registry)

	// Choosing a built-in app afterwards clears the credentials
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	assert.True(t, m.registry.Empty())
}

func TestInstall_CustomImageWithoutCredentialsLeavesRegistryEmpty(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest"})
	assert.True(t, m.registry.Empty())
}

func TestInstall_PartialRegistryCredentialsStayOnImageForm(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})

	m, _ = updateInstall(m, InstallImageSubmitMsg{
		ImageRef:         "registry.example.com/app:latest",
		RegistryUsername: "user",
	})
	assert.Equal(t, installStateImageForm, m.state)
	assert.ErrorIs(t, m.err, docker.ErrRegistryUsernameWithoutPassword)

	m.err = nil
	m, _ = updateInstall(m, InstallImageSubmitMsg{
		ImageRef:         "registry.example.com/app:latest",
		RegistryPassword: "pass",
	})
	assert.Equal(t, installStateImageForm, m.state)
	assert.ErrorIs(t, m.err, docker.ErrRegistryPasswordWithoutUsername)
}

func TestInstall_CLIModeSkipsToHostname(t *testing.T) {
	m := NewInstall(newTestNamespace(), "campfire")
	assert.Equal(t, installStateHostname, m.state)
}

func TestInstall_CLIModeExpandsAlias(t *testing.T) {
	m := NewInstall(newTestNamespace(), "campfire")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	view := ansi.Strip(m.View())
	assert.Contains(t, view, "once-campfire.example.com")
	assert.Contains(t, view, "Installing campfire")
}

func TestInstall_InteractiveModeHasNoTitle(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	view := ansi.Strip(m.View())
	assert.NotContains(t, view, "Installing")
}

func TestInstall_SubmitTriggersActivity(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)
}

func TestInstall_SuccessNavigatesToApp(t *testing.T) {
	m := newTestInstall()
	app := &docker.Application{}

	_, cmd := updateInstall(m, InstallActivityDoneMsg{App: app})
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(NavigateToAppMsg)
	require.True(t, ok, "expected NavigateToAppMsg, got %T", msg)
	assert.Equal(t, app, navMsg.App)
}

func TestInstall_FailureReturnsToHostname(t *testing.T) {
	ns := newTestNamespace()
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Go through known app flow to activity
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})
	assert.Equal(t, installStateActivity, m.state)

	// Simulate failure
	installErr := errors.New("connection refused")
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: installErr})

	assert.Equal(t, installStateHostname, m.state)
	assert.Equal(t, installErr, m.err)
	assert.Contains(t, m.View(), "connection refused")
}

func TestInstall_ErrorClearsOnKeypress(t *testing.T) {
	m := newTestInstall()
	m.state = installStateHostname
	m.hostnameForm = NewInstallHostnameForm("ghcr.io/basecamp/once-campfire:latest", "", InstallTargetPublic)
	m.err = errors.New("some error")

	m, _ = updateInstall(m, keyPressMsg("a"))
	assert.Nil(t, m.err)
}

func TestInstall_BackNavigation_AppListEscNavigatesToDashboard(t *testing.T) {
	m := newTestInstall()

	_, cmd := updateInstall(m, keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok, "expected NavigateToDashboardMsg, got %T", msg)
}

func TestInstall_BackNavigation_ImageFormEscGoesToAppList(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	assert.Equal(t, installStateImageForm, m.state)

	m, _ = updateInstall(m, keyPressMsg("esc"))
	assert.Equal(t, installStateAppList, m.state)
}

func TestInstall_BackNavigation_WhereEscGoesToAppList(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	assert.Equal(t, installStateWhere, m.state)

	m, _ = updateInstall(m, keyPressMsg("esc"))
	assert.Equal(t, installStateAppList, m.state)
}

func TestInstall_BackNavigation_WhereEscGoesToImageForm(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest"})
	assert.Equal(t, installStateWhere, m.state)

	m, _ = updateInstall(m, keyPressMsg("esc"))
	assert.Equal(t, installStateImageForm, m.state)
}

func TestInstall_BackNavigation_HostnameEscGoesToWhere(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	assert.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, keyPressMsg("esc"))
	assert.Equal(t, installStateWhere, m.state)
}

func TestInstall_BackNavigation_HostnameBackMsgGoesToWhere(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)

	m, _ = updateInstall(m, InstallHostnameBackMsg{})
	assert.Equal(t, installStateWhere, m.state)
}

func TestInstall_BackNavigation_HostnameBackMsgCustomImageGoesToWhere(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest"})
	installSelectTarget(t, &m, InstallTargetPublic)

	m, _ = updateInstall(m, InstallHostnameBackMsg{})
	assert.Equal(t, installStateWhere, m.state)
}

func TestInstall_BackNavigation_TailscaleEscGoesToHostname(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.com"})
	require.Equal(t, installStateTailscale, m.state)

	m, _ = updateInstall(m, keyPressMsg("esc"))
	assert.Equal(t, installStateHostname, m.state, "back from the Service options returns to the hostname")
}

func TestInstall_BackNavigation_ImageFormBackMsg(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})

	m, _ = updateInstall(m, InstallImageBackMsg{})
	assert.Equal(t, installStateAppList, m.state)
}

func TestInstall_EscQuitsInCLIMode(t *testing.T) {
	m := NewInstall(newTestNamespace(), "ghcr.io/basecamp/once-campfire:latest")

	_, cmd := updateInstall(m, keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(QuitMsg)
	assert.True(t, ok, "expected QuitMsg, got %T", msg)
}

func TestInstall_HostnameBackQuitsInCLIMode(t *testing.T) {
	m := NewInstall(newTestNamespace(), "ghcr.io/basecamp/once-campfire:latest")

	_, cmd := updateInstall(m, InstallHostnameBackMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(QuitMsg)
	assert.True(t, ok, "expected QuitMsg, got %T", msg)
}

func TestInstall_ShowsLogoAndHidesTitleWhenNoApps(t *testing.T) {
	m := NewInstall(newTestNamespace(), "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})

	view := m.View()
	assert.Contains(t, view, "██████╗")
	assert.NotContains(t, view, "ONCE · install")
}

func TestInstall_ShowsTitleAndHidesLogoWhenAppsExist(t *testing.T) {
	ns := newTestNamespace(docker.ApplicationSettings{Name: "myapp"})
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})

	view := m.View()
	assert.NotContains(t, view, "██████╗")
	assert.Contains(t, view, "ONCE · install")
}

func TestInstall_ShowLogo(t *testing.T) {
	noApps := newTestNamespace()
	withApps := newTestNamespace(docker.ApplicationSettings{Name: "myapp"})

	assert.True(t, NewInstall(noApps, "").showLogo())
	assert.False(t, NewInstall(withApps, "").showLogo())

	t.Setenv("ONCE_REDUCED_MOTION", "true")
	assert.False(t, NewInstall(noApps, "").showLogo())
	assert.False(t, NewInstall(withApps, "").showLogo())
}

func TestInstall_PullFailureReturnsToImageForm(t *testing.T) {
	ns := newTestNamespace()
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "bad:image"})
	installSelectTarget(t, &m, InstallTargetPublic)
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "bad:image", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)

	pullErr := fmt.Errorf("%w: %w", docker.ErrDeployFailed, docker.ErrPullFailed)
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: pullErr})
	assert.Equal(t, installStateImageForm, m.state)
	assert.Equal(t, pullErr, m.err)
	assert.False(t, m.imageForm.ShowsCredentials(), "a plain pull failure keeps the credential fields hidden")
}

func TestInstall_AuthLikePullFailureRevealsCredentialFields(t *testing.T) {
	ns := newTestNamespace()
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "registry.example.com/app:latest"})
	installSelectTarget(t, &m, InstallTargetPublic)
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "registry.example.com/app:latest", Hostname: "app.example.com"})
	assert.Equal(t, installStateActivity, m.state)
	assert.False(t, m.imageForm.ShowsCredentials())

	pullErr := fmt.Errorf("%w: %w: %w", docker.ErrDeployFailed, docker.ErrPullMaybeUnauthorized, docker.ErrPullFailed)
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: pullErr})
	assert.Equal(t, installStateImageForm, m.state)
	assert.True(t, m.imageForm.ShowsCredentials())
	assert.Equal(t, "registry.example.com/app:latest", m.imageForm.ImageRef(), "typed image ref is preserved")
	assert.Equal(t, pullErr, m.err)
}

func TestInstall_PullFailureReturnsToAppList(t *testing.T) {
	ns := newTestNamespace()
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})
	assert.Equal(t, installStateActivity, m.state)

	pullErr := fmt.Errorf("%w: %w", docker.ErrDeployFailed, docker.ErrPullFailed)
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: pullErr})
	assert.Equal(t, installStateAppList, m.state)
	assert.Equal(t, pullErr, m.err)
}

func TestInstall_NonPullDeployFailureReturnsToHostname(t *testing.T) {
	ns := newTestNamespace()
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})

	deployErr := fmt.Errorf("%w: %w", docker.ErrDeployFailed, errors.New("container crashed"))
	m, _ = updateInstall(m, InstallActivityFailedMsg{Err: deployErr})
	assert.Equal(t, installStateHostname, m.state)
}

func TestInstall_HostnameInUseBlocksInstall(t *testing.T) {
	ns := newTestNamespace(docker.ApplicationSettings{Name: "myapp", Host: "taken.example.com"})
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	assert.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "taken.example.com"})
	assert.Equal(t, installStateHostname, m.state)
	assert.ErrorIs(t, m.err, docker.ErrHostnameInUse)
}

func TestInstall_UniqueHostnameAllowsInstall(t *testing.T) {
	ns := newTestNamespace(docker.ApplicationSettings{Name: "myapp", Host: "taken.example.com"})
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "unique.example.com"})
	assert.Equal(t, installStateActivity, m.state)
	assert.Nil(t, m.err)
}

func TestInstall_FailureDoesNotRestartLogo(t *testing.T) {
	ns := newTestNamespace()
	noApps := NewInstall(ns, "")
	noApps, _ = updateInstall(noApps, tea.WindowSizeMsg{Width: 80, Height: 40})
	noApps, _ = updateInstall(noApps, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &noApps, InstallTargetPublic)
	noApps, _ = updateInstall(noApps, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "app.example.com"})
	_, cmd := updateInstall(noApps, InstallActivityFailedMsg{Err: errors.New("fail")})
	assert.Nil(t, cmd)
}

func TestInstall_HelpKeyShownOnHostnameScreen(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// App list: no F1
	view := ansi.Strip(m.View())
	assert.NotContains(t, view, "F1")

	// Image form: no F1
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	view = ansi.Strip(m.View())
	assert.NotContains(t, view, "F1")

	// Where: no F1
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest"})
	view = ansi.Strip(m.View())
	assert.NotContains(t, view, "F1")

	// Hostname: F1 shown
	installSelectTarget(t, &m, InstallTargetPublic)
	view = ansi.Strip(m.View())
	assert.Contains(t, view, "F1")
	assert.Contains(t, view, "help")
}

func TestInstall_HelpKeyShownOnHostnameScreenNonFirstRun(t *testing.T) {
	ns := newTestNamespace(docker.ApplicationSettings{Name: "campfire"})
	m := NewInstall(ns, "")
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Navigate to hostname screen
	m, _ = updateInstall(m, InstallCustomSelectedMsg{})
	m, _ = updateInstall(m, InstallImageSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire:latest"})
	installSelectTarget(t, &m, InstallTargetPublic)

	view := ansi.Strip(m.View())
	assert.Contains(t, view, "F1")
	assert.Contains(t, view, "help")
}

func TestInstall_AdvancedFromTailscaleReturnsToTheServiceScreen(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)
	require.Equal(t, installStateHostname, m.state)
	require.Nil(t, m.hostnameForm.form.actionButton, "the tailnet name screen has no Advanced settings button")

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.ts.net"})
	require.Equal(t, installStateTailscale, m.state)

	view := ansi.Strip(m.View())
	assert.Contains(t, view, "Advanced settings", "the Service screen offers the advanced settings action")

	m.tailscaleForm.form.TextField(installTailscaleServiceField).SetValue("budget")
	m.tailscaleForm.form.TextField(installTailscalePathField).SetValue("/budget")

	m, _ = updateInstall(m, InstallAdvancedMsg{
		ImageRef:  "ghcr.io/basecamp/once-campfire",
		Hostname:  "campfire.example.ts.net",
		FromStage: InstallAdvancedFromTailscale,
	})
	require.Equal(t, installStateAdvanced, m.state)

	m, _ = updateInstall(m, SettingsSectionSubmitMsg{Settings: docker.ApplicationSettings{AppPort: 5006}})

	require.Equal(t, installStateTailscale, m.state, "submitting returns to the Service screen")
	assert.Nil(t, m.activity, "and does not start the install")
	assert.Equal(t, "budget", m.tailscaleForm.form.TextField(installTailscaleServiceField).Value(),
		"the Service values typed before opening advanced settings are preserved")
	assert.Equal(t, "/budget", m.tailscaleForm.form.TextField(installTailscalePathField).Value())

	m, _ = updateInstall(m, InstallTailscaleSubmitMsg{Tailscale: docker.TailscaleSettings{
		Enabled: true,
		Mode:    docker.DocktailMode,
		Service: "budget",
	}})

	require.Equal(t, installStateActivity, m.state, "the Service screen's Install starts the install")
	require.NotNil(t, m.activity)
	assert.Equal(t, 5006, m.activity.settings.AppPort, "the advanced settings are carried through")
	assert.Equal(t, "budget", m.activity.settings.EffectiveServiceName())
}

func TestInstall_AdvancedCancelFromTailscaleReturnsToTheServiceScreen(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.ts.net"})

	m, _ = updateInstall(m, InstallAdvancedMsg{
		ImageRef:  "ghcr.io/basecamp/once-campfire",
		Hostname:  "campfire.example.ts.net",
		FromStage: InstallAdvancedFromTailscale,
	})
	require.Equal(t, installStateAdvanced, m.state)

	m, _ = updateInstall(m, SettingsSectionCancelMsg{})

	assert.Equal(t, installStateTailscale, m.state)
	assert.Nil(t, m.activity)
}

func TestInstall_EscFromAdvancedReturnsToTheOpeningScreen(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.ts.net"})
	m, _ = updateInstall(m, InstallAdvancedMsg{
		ImageRef:  "ghcr.io/basecamp/once-campfire",
		Hostname:  "campfire.example.ts.net",
		FromStage: InstallAdvancedFromTailscale,
	})

	m, _ = updateInstall(m, keyPressMsg("esc"))

	assert.Equal(t, installStateTailscale, m.state)
}

func TestInstall_AdvancedFromHostnameStillInstallsAPublicApp(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)

	m, _ = updateInstall(m, InstallAdvancedMsg{
		ImageRef:  "ghcr.io/basecamp/once-campfire",
		Hostname:  "app.example.com",
		FromStage: InstallAdvancedFromHostname,
	})
	require.Equal(t, installStateAdvanced, m.state)

	m, _ = updateInstall(m, SettingsSectionSubmitMsg{Settings: docker.ApplicationSettings{AppPort: 5006}})

	require.Equal(t, installStateActivity, m.state, "a public app still installs straight from advanced settings")
}

func TestInstall_HelpKeyShownOnTailscaleScreen(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.com"})

	require.Equal(t, installStateTailscale, m.state)
	view := ansi.Strip(m.View())
	assert.Contains(t, view, "F1", "the Tailscale screen advertises F1")
	assert.Contains(t, view, "label help", "the help description names what F1 opens")

	m, _ = updateInstall(m, keyPressMsg("f1"))
	assert.NotNil(t, m.tailscaleForm.panel, "F1 opens the panel inside the Tailscale form")
	overlay := ansi.Strip(m.View())
	assert.Contains(t, overlay, "Tailscale Service labels (docktail)")
	assert.NotContains(t, overlay, "Install", "the form's submit button is hidden behind the panel")

	m, _ = updateInstall(m, keyPressMsg("f1"))
	assert.Nil(t, m.tailscaleForm.panel, "F1 toggles the panel closed again")

	m, _ = updateInstall(m, keyPressMsg("f1"))
	require.NotNil(t, m.tailscaleForm.panel, "F1 reopens the panel")

	m, cmd := updateInstall(m, keyPressMsg("esc"))
	require.NotNil(t, cmd, "Esc on the panel returns a close command")
	closeMsg, ok := cmd().(TailscaleHelpCloseMsg)
	require.True(t, ok)
	m, _ = updateInstall(m, closeMsg)
	assert.Nil(t, m.tailscaleForm.panel, "Esc dismisses the panel")
	assert.Equal(t, installStateTailscale, m.state, "Esc did not navigate back")
}

func TestOverlayBlockContainsRow(t *testing.T) {
	block := newOverlayBlock("hello\nworld", 5, 10)
	assert.Equal(t, 5, block.width) // "hello" and "world" are both 5 chars

	// Row before block
	_, ok := block.containsRow(4)
	assert.False(t, ok)

	// First row
	line, ok := block.containsRow(5)
	assert.True(t, ok)
	assert.Equal(t, "hello", line)

	// Second row
	line, ok = block.containsRow(6)
	assert.True(t, ok)
	assert.Equal(t, "world", line)

	// Row after block
	_, ok = block.containsRow(7)
	assert.False(t, ok)
}

func TestOverlayBlockPadsShortLines(t *testing.T) {
	block := newOverlayBlock("hi\nworld", 0, 0)
	assert.Equal(t, 5, block.width) // max("hi", "world") = 5

	line, ok := block.containsRow(0)
	assert.True(t, ok)
	assert.Equal(t, "hi   ", line) // "hi" padded to width 5
}

func TestBlockWidth(t *testing.T) {
	lines := []string{"short", "a longer line", "mid"}
	assert.Equal(t, 13, blockWidth(lines))
}

func TestBlockWidthEmpty(t *testing.T) {
	assert.Equal(t, 0, blockWidth(nil))
}

func TestInstall_AdvancedSettingsFlow(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	assert.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, InstallAdvancedMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "app.example.com"})
	assert.Equal(t, installStateAdvanced, m.state)

	m, _ = updateInstall(m, SettingsSectionCancelMsg{})
	assert.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, InstallAdvancedMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "app.example.com"})
	m, _ = updateInstall(m, SettingsSectionSubmitMsg{Settings: docker.ApplicationSettings{AppPort: 5006, HealthCheckPath: "/healthz"}})

	require.Equal(t, installStateActivity, m.state, "a public app installs straight from advanced settings")
	require.NotNil(t, m.activity)
	assert.Equal(t, 5006, m.activity.settings.AppPort)
	assert.Equal(t, "/healthz", m.activity.settings.HealthCheckPath)
	assert.False(t, m.activity.settings.DocktailEnabled())
}

func TestInstall_TailnetFlow(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.com"})
	require.Equal(t, installStateTailscale, m.state, "a tailnet app asks for its Service options")
	assert.Equal(t, "once-campfire", m.tailscaleForm.form.TextField(installTailscaleServiceField).Value())

	m, _ = updateInstall(m, InstallTailscaleSubmitMsg{Tailscale: docker.TailscaleSettings{
		Enabled:     true,
		Mode:        docker.DocktailMode,
		Service:     "campfire",
		ServicePort: 443,
		Path:        "/",
	}})

	require.Equal(t, installStateActivity, m.state)
	require.NotNil(t, m.activity)
	assert.Equal(t, "ghcr.io/basecamp/once-campfire", m.activity.imageRef)
	assert.Equal(t, "campfire.example.com", m.activity.hostname)
	assert.True(t, m.activity.settings.DocktailEnabled())
	assert.Equal(t, "campfire", m.activity.settings.EffectiveServiceName())
	assert.Equal(t, 443, m.activity.settings.EffectiveServicePort())
}

func TestInstall_TailnetAdvancedSettingsSurviveTheServiceScreen(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)

	m, _ = updateInstall(m, InstallAdvancedMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.com"})
	m, _ = updateInstall(m, SettingsSectionSubmitMsg{Settings: docker.ApplicationSettings{
		AppPort:         5006,
		HealthCheckPath: "/healthz",
		EnvVars:         map[string]string{"TZ": "UTC"},
	}})
	require.Equal(t, installStateTailscale, m.state)

	m, _ = updateInstall(m, InstallTailscaleSubmitMsg{Tailscale: docker.TailscaleSettings{
		Enabled: true,
		Mode:    docker.DocktailMode,
		Service: "campfire",
	}})

	require.Equal(t, installStateActivity, m.state)
	require.NotNil(t, m.activity)
	assert.Equal(t, 5006, m.activity.settings.AppPort)
	assert.Equal(t, "/healthz", m.activity.settings.HealthCheckPath)
	assert.Equal(t, map[string]string{"TZ": "UTC"}, m.activity.settings.EnvVars)
	assert.Equal(t, "campfire", m.activity.settings.EffectiveServiceName())
}

func TestInstall_SwitchingBackToPublicClearsTailscaleSettings(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)
	m, _ = updateInstall(m, InstallAdvancedMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.com"})
	m, _ = updateInstall(m, SettingsSectionSubmitMsg{Settings: docker.ApplicationSettings{AppPort: 5006}})
	m, _ = updateInstall(m, InstallTailscaleSubmitMsg{Tailscale: docker.TailscaleSettings{
		Enabled: true,
		Mode:    docker.DocktailMode,
		Service: "campfire",
	}})
	require.Equal(t, installStateActivity, m.state)

	// Start again and pick Public: the app must not carry the Service settings.
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.com"})

	require.Equal(t, installStateActivity, m.state)
	assert.Equal(t, docker.TailscaleSettings{}, m.activity.settings.Tailscale)
}

func TestInstall_AdvancedMsgRequiresHostname(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)

	m, _ = updateInstall(m, InstallAdvancedMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: ""})

	assert.Equal(t, installStateHostname, m.state)
	assert.ErrorIs(t, m.err, docker.ErrHostRequired)
}

func TestInstall_AdvancedSubmitValidatesSettings(t *testing.T) {
	newAdvanced := func(t *testing.T) Install {
		t.Helper()

		m := newTestInstall()
		m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
		m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
		installSelectTarget(t, &m, InstallTargetPublic)
		m, _ = updateInstall(m, InstallAdvancedMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "app.example.com"})
		require.Equal(t, installStateAdvanced, m.state)

		return m
	}

	t.Run("app port out of range is rejected before installing", func(t *testing.T) {
		m := newAdvanced(t)

		m, _ = updateInstall(m, SettingsSectionSubmitMsg{Settings: docker.ApplicationSettings{AppPort: 65536}})

		assert.Equal(t, installStateAdvanced, m.state)
		assert.ErrorIs(t, m.err, docker.ErrInvalidAppPort)
	})

	t.Run("relative volume path is rejected before installing", func(t *testing.T) {
		m := newAdvanced(t)

		m, _ = updateInstall(m, SettingsSectionSubmitMsg{Settings: docker.ApplicationSettings{VolumePaths: []string{"data"}}})

		assert.Equal(t, installStateAdvanced, m.state)
		assert.ErrorIs(t, m.err, docker.ErrInvalidVolumePath)
	})

	t.Run("invalid Service settings are rejected before installing", func(t *testing.T) {
		m := newAdvanced(t)
		m.target = InstallTargetTailnet
		m.tailscaleConfirmed = true
		m.pendingTailscale = docker.TailscaleSettings{Enabled: true, Mode: docker.DocktailMode, Service: "campfire", Port: 70000}

		m, _ = updateInstall(m, SettingsSectionSubmitMsg{Settings: docker.ApplicationSettings{}})

		assert.Equal(t, installStateAdvanced, m.state)
		assert.ErrorIs(t, m.err, docker.ErrInvalidDocktailPort)
	})
}

func TestInstall_TailscaleSubmitSurfacesDockerValidation(t *testing.T) {
	m := newTestInstall()
	m.target = InstallTargetTailnet
	m.pendingImage = "example/postgres:latest"
	m.pendingHost = "postgres.example.ts.net"
	m.state = installStateTailscale

	m, _ = updateInstall(m, InstallTailscaleSubmitMsg{Tailscale: docker.TailscaleSettings{
		Enabled:         true,
		Mode:            docker.DocktailMode,
		Service:         "postgres",
		Protocol:        "tcp",
		ServiceProtocol: "tcp",
		Path:            "/db",
	}})

	assert.Equal(t, installStateTailscale, m.state)
	assert.ErrorIs(t, m.err, docker.ErrDocktailPathOnTCP)
}

func TestInstall_TailscaleHelpDescribesTheTailnet(t *testing.T) {
	m := newTestInstall()
	m.target = InstallTargetTailnet
	m.state = installStateHostname

	title, content := m.helpForState()

	assert.Equal(t, "Naming the Service on your tailnet", title)
	assert.Contains(t, content, "tailnet")
	assert.Contains(t, content, "MagicDNS")
	assert.NotContains(t, content, "localhost")
}

// Helpers

func newTestInstall() Install {
	return NewInstall(newTestNamespace(), "")
}

// installSelectTarget picks an option on the where screen.
func installSelectTarget(t *testing.T, m *Install, target InstallTarget) {
	t.Helper()

	for m.whereForm.menu.SelectedKey() != int(target) {
		*m, _ = updateInstall(*m, keyPressMsg("down"))
	}

	var cmd tea.Cmd
	*m, cmd = updateInstall(*m, keyPressMsg("enter"))
	require.NotNil(t, cmd)

	// The menu reports the selection, and the where form turns that into its
	// own message.
	*m, cmd = updateInstall(*m, cmd().(MenuSelectMsg))
	require.NotNil(t, cmd)

	msg, ok := cmd().(InstallWhereSelectedMsg)
	require.True(t, ok, "expected InstallWhereSelectedMsg, got %T", msg)
	*m, _ = updateInstall(*m, msg)
}

func newTestNamespace(apps ...docker.ApplicationSettings) *docker.Namespace {
	ns, err := docker.NewNamespace("test", docker.WithApplications(apps...))
	if err != nil {
		panic(err)
	}
	return ns
}

func TestInstall_TailnetWarningsShowOnTheHostnameScreen(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)
	require.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, installTailnetWarningsMsg{warnings: []string{
		"the tailscale CLI isn't installed on this host",
	}})

	view := ansi.Strip(m.View())
	assert.Contains(t, view, "⚠ the tailscale CLI isn't installed on this host")
	assert.Less(t, strings.Index(view, "⚠"), strings.Index(view, "Tailnet name"), "the banner sits above the form")
}

func TestInstall_TailnetWarningsDoNotShowForPublicInstalls(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	require.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, installTailnetWarningsMsg{warnings: []string{
		"the tailscale CLI isn't installed on this host",
	}})

	view := ansi.Strip(m.View())
	assert.NotContains(t, view, "⚠")
}

func TestInstall_TailnetWarningsDoNotBlockTheInstall(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)

	m, _ = updateInstall(m, installTailnetWarningsMsg{warnings: []string{
		"the tailscale CLI isn't installed on this host",
	}})

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.ts.net"})

	require.Equal(t, installStateTailscale, m.state, "the install carries on despite the warnings")
}

func TestInstall_TailnetReadinessRunsInTheBackground(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})

	_, cmd := updateInstall(m, InstallWhereSelectedMsg{Target: InstallTargetTailnet})
	require.NotNil(t, cmd, "picking Tailnet only starts the readiness check")

	msg := installTailnetWarningsFrom(t, cmd)
	assert.Empty(t, msg.warnings, "a healthy host has nothing to warn about")
}

func TestInstall_TailnetReadinessIsNotRunForPublicInstalls(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})

	_, cmd := updateInstall(m, InstallWhereSelectedMsg{Target: InstallTargetPublic})
	require.NotNil(t, cmd)

	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, batched := range batch {
			_, isReadiness := batched().(installTailnetWarningsMsg)
			assert.False(t, isReadiness, "a public install needs no tailnet check")
		}
		return
	}

	_, isReadiness := msg.(installTailnetWarningsMsg)
	assert.False(t, isReadiness, "a public install needs no tailnet check")
}

func TestInstall_TailnetWarningsAreClearedWhenTheTargetChanges(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)

	m, _ = updateInstall(m, installTailnetWarningsMsg{warnings: []string{"a warning"}})

	m, _ = updateInstall(m, InstallWhereSelectedMsg{Target: InstallTargetPublic})

	assert.Empty(t, m.tailnetWarnings)
}

func TestInstall_BackNavigationPreservesHostnameInput(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	m, _ = updateInstall(m, InstallWhereSelectedMsg{Target: InstallTargetPublic})
	require.Equal(t, installStateHostname, m.state)

	hostnameFormTypeTextOnInstall(&m, "chat.example.com")
	m, _ = updateInstall(m, InstallHostnameBackMsg{})
	require.Equal(t, installStateWhere, m.state)

	m, _ = updateInstall(m, InstallWhereSelectedMsg{Target: InstallTargetPublic})
	assert.Equal(t, "chat.example.com", m.hostnameForm.Hostname(), "the same target returns to the typed hostname")
}

func TestInstall_BackNavigationPreservesAdvancedFormInput(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	require.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, InstallAdvancedMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "app.example.com"})
	require.Equal(t, installStateAdvanced, m.state)

	m.advancedForm.form.TextField(advancedAppPortField).SetValue("5006")
	m.advancedForm.form.TextField(advancedHealthCheckField).SetValue("/healthz")

	m, _ = updateInstall(m, SettingsSectionCancelMsg{})
	require.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, InstallAdvancedMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "app.example.com"})
	require.Equal(t, installStateAdvanced, m.state)
	assert.Equal(t, "5006", m.advancedForm.form.TextField(advancedAppPortField).Value(), "advanced settings typed values are preserved")
	assert.Equal(t, "/healthz", m.advancedForm.form.TextField(advancedHealthCheckField).Value())
}

func TestInstall_HostnameResubmitCarriesTailscaleFormValues(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)
	require.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.com"})
	require.Equal(t, installStateTailscale, m.state)

	m.tailscaleForm.form.TextField(installTailscaleServiceField).SetValue("budget")
	m.tailscaleForm.form.TextField(installTailscalePathField).SetValue("/budget")

	m, _ = updateInstall(m, InstallTailscaleBackMsg{})
	require.Equal(t, installStateHostname, m.state)

	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.com"})
	require.Equal(t, installStateTailscale, m.state)
	assert.Equal(t, "budget", m.tailscaleForm.form.TextField(installTailscaleServiceField).Value(), "unsubmitted Tailscale values are preserved")
	assert.Equal(t, "/budget", m.tailscaleForm.form.TextField(installTailscalePathField).Value())
}

func TestInstall_SwitchingBackToPublicClearsTailscaleSettingsAfterResubmit(t *testing.T) {
	m := newTestInstall()
	m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetTailnet)
	m, _ = updateInstall(m, InstallAdvancedMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.com"})
	m, _ = updateInstall(m, SettingsSectionSubmitMsg{Settings: docker.ApplicationSettings{AppPort: 5006}})
	m, _ = updateInstall(m, InstallTailscaleSubmitMsg{Tailscale: docker.TailscaleSettings{
		Enabled: true,
		Mode:    docker.DocktailMode,
		Service: "campfire",
	}})
	require.Equal(t, installStateActivity, m.state)

	m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
	installSelectTarget(t, &m, InstallTargetPublic)
	m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "campfire.example.com"})

	require.Equal(t, installStateActivity, m.state)
	assert.Equal(t, docker.TailscaleSettings{}, m.activity.settings.Tailscale, "switching back to Public still drops the Service settings")
}

func TestInstall_StaleSubmitIsIgnored(t *testing.T) {
	t.Run("InstallFormSubmitMsg after leaving hostname is ignored", func(t *testing.T) {
		m := newTestInstall()
		m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
		m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
		installSelectTarget(t, &m, InstallTargetPublic)
		require.Equal(t, installStateHostname, m.state)

		m, _ = updateInstall(m, InstallHostnameBackMsg{})
		require.Equal(t, installStateWhere, m.state)

		m, _ = updateInstall(m, InstallFormSubmitMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})
		assert.Equal(t, installStateWhere, m.state, "a stale submit is dropped")
		assert.Nil(t, m.activity)
	})

	t.Run("InstallAdvancedMsg after leaving hostname is ignored", func(t *testing.T) {
		m := newTestInstall()
		m, _ = updateInstall(m, tea.WindowSizeMsg{Width: 80, Height: 24})
		m, _ = updateInstall(m, InstallAppSelectedMsg{ImageRef: "ghcr.io/basecamp/once-campfire"})
		installSelectTarget(t, &m, InstallTargetPublic)
		require.Equal(t, installStateHostname, m.state)

		m, _ = updateInstall(m, InstallHostnameBackMsg{})
		require.Equal(t, installStateWhere, m.state)

		m, _ = updateInstall(m, InstallAdvancedMsg{ImageRef: "ghcr.io/basecamp/once-campfire", Hostname: "chat.example.com"})
		assert.Equal(t, installStateWhere, m.state, "a stale advanced message is dropped")
	})
}

func updateInstall(m Install, msg tea.Msg) (Install, tea.Cmd) {
	comp, cmd := m.Update(msg)
	return comp.(Install), cmd
}

// installTailnetWarningsFrom pulls the readiness result out of a batched
// command, so a test can check what the background check reported.
func installTailnetWarningsFrom(t *testing.T, cmd tea.Cmd) installTailnetWarningsMsg {
	t.Helper()

	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, batched := range batch {
			result, ok := batched().(installTailnetWarningsMsg)
			if ok {
				return result
			}
		}
		t.Fatalf("no readiness result in the batched commands")
	}

	result, ok := msg.(installTailnetWarningsMsg)
	require.True(t, ok, "expected installTailnetWarningsMsg, got %T", msg)
	return result
}

func hostnameFormTypeTextOnInstall(m *Install, text string) {
	for _, r := range text {
		*m, _ = updateInstall(*m, keyPressMsg(string(r)))
	}
}
