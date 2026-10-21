package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestSettings_InitialStateIsForm(t *testing.T) {
	s := testSettings()
	assert.Equal(t, settingsStateForm, s.state)
}

func TestSettings_EscNavigatesToDashboard(t *testing.T) {
	s := testSettings()
	s, _ = updateSettings(s, tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := updateSettings(s, keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(NavigateToDashboardMsg)
	require.True(t, ok, "expected NavigateToDashboardMsg, got %T", msg)
	assert.Equal(t, "test-app", navMsg.AppName)
}

func TestSettings_CancelNavigatesToDashboard(t *testing.T) {
	s := testSettings()
	_, cmd := updateSettings(s, SettingsSectionCancelMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok)
}

func TestSettings_SubmitUnchangedNavigatesToDashboard(t *testing.T) {
	s := testSettings()
	_, cmd := updateSettings(s, SettingsSectionSubmitMsg{Settings: s.app.Settings})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok)
}

func TestSettings_SubmitChangedStartsDeploy(t *testing.T) {
	s := testSettings()
	s, _ = updateSettings(s, tea.WindowSizeMsg{Width: 80, Height: 24})

	changed := s.app.Settings
	changed.Host = "new.example.com"

	s, _ = updateSettings(s, SettingsSectionSubmitMsg{Settings: changed})
	assert.Equal(t, settingsStateDeploying, s.state)
}

func TestSettings_SubmitInvalidSettingsStaysOnForm(t *testing.T) {
	s := testSettings()

	changed := s.app.Settings
	changed.Registry = docker.RegistrySettings{Host: "ghcr.io", Username: "user"}

	s, _ = updateSettings(s, SettingsSectionSubmitMsg{Settings: changed})
	assert.Equal(t, settingsStateForm, s.state)
	assert.ErrorIs(t, s.err, docker.ErrRegistryUsernameWithoutPassword)
}

func TestSettings_DeployFinishedNavigatesToApp(t *testing.T) {
	s := testSettings()
	s.state = settingsStateDeploying

	_, cmd := updateSettings(s, settingsDeployFinishedMsg{})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToAppMsg)
	assert.True(t, ok)
}

func TestSettings_DeployFailureReturnsToFormAndRestoresSettings(t *testing.T) {
	s := testSettings()
	original := s.app.Settings

	changed := original
	changed.Image = "registry.example.com/missing:latest"
	s, _ = updateSettings(s, SettingsSectionSubmitMsg{Settings: changed})
	assert.Equal(t, settingsStateDeploying, s.state)
	assert.Equal(t, changed, s.app.Settings)

	s, cmd := updateSettings(s, settingsDeployFinishedMsg{err: assert.AnError})
	assert.Nil(t, cmd)
	assert.Equal(t, settingsStateForm, s.state)
	assert.ErrorIs(t, s.err, assert.AnError)
	assert.Equal(t, original, s.app.Settings, "failed deploy restores the saved settings")
}

func TestSettings_ActionFinishedWithError(t *testing.T) {
	s := testSettings()
	s.state = settingsStateRunningAction

	s, _ = updateSettings(s, settingsActionFinishedMsg{err: assert.AnError})
	assert.Equal(t, settingsStateForm, s.state)
	assert.Error(t, s.err)
}

func TestSettings_ActionFinishedWithMessage(t *testing.T) {
	s := testSettings()
	s.state = settingsStateRunningAction

	s, _ = updateSettings(s, settingsActionFinishedMsg{message: "Success!"})
	assert.Equal(t, settingsStateActionComplete, s.state)
	assert.Equal(t, "Success!", s.actionSuccessMessage)
}

func TestSettings_ActionCompleteEnterNavigates(t *testing.T) {
	s := testSettings()
	s.state = settingsStateActionComplete

	_, cmd := updateSettings(s, keyPressMsg("enter"))
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok)
}

func TestSettings_ActionCompleteDoneClick(t *testing.T) {
	s := testSettings()
	s.state = settingsStateActionComplete

	_, cmd := updateSettings(s, MouseEvent{IsClick: true, Target: "done"})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(NavigateToDashboardMsg)
	assert.True(t, ok)
}

func TestSettings_ErrorClearsOnKeypress(t *testing.T) {
	s := testSettings()
	s.state = settingsStateForm
	s.err = assert.AnError

	s, _ = updateSettings(s, keyPressMsg("a"))
	assert.Nil(t, s.err)
}

func TestSettings_ViewShowsError(t *testing.T) {
	s := testSettings()
	s.state = settingsStateForm
	s.err = assert.AnError
	s.width = 80
	s.height = 24

	view := s.View()
	assert.Contains(t, view, assert.AnError.Error())
}

func TestSettings_ViewShowsProgressWhileDeploying(t *testing.T) {
	s := testSettings()
	s.state = settingsStateDeploying
	s.width = 80
	s.height = 24

	// Should not panic
	view := s.View()
	assert.NotEmpty(t, view)
}

func TestSettings_TailscaleSectionIsWired(t *testing.T) {
	ns, _ := docker.NewNamespace("test")
	app := &docker.Application{
		Settings: docker.ApplicationSettings{
			Name:  "budget",
			Host:  "budget.example.com",
			Image: "example/app:latest",
		},
	}

	s := NewSettings(ns, app, SettingsSectionTailscale)

	assert.Equal(t, "Tailscale", s.section.Title())
}

func TestSettings_TailscaleSectionAdvertisesF1(t *testing.T) {
	ns, _ := docker.NewNamespace("test")
	app := &docker.Application{
		Settings: docker.ApplicationSettings{
			Name:  "budget",
			Host:  "budget.example.com",
			Image: "example/app:latest",
		},
	}

	tailscale := NewSettings(ns, app, SettingsSectionTailscale)
	other := NewSettings(ns, app, SettingsSectionApplication)

	assert.Equal(t, "f1", tailscale.help.bindings[0].Keys()[0])
	assert.Equal(t, "label help", tailscale.help.bindings[0].Help().Desc)

	var sawLabelsHelp bool
	for _, b := range other.help.bindings {
		if b.Help().Desc == "label help" {
			sawLabelsHelp = true
			break
		}
	}
	assert.False(t, sawLabelsHelp, "non-Tailscale sections must not advertise the label help panel")
}

func TestSettings_TailscalePanelEscDoesNotNavigate(t *testing.T) {
	s, _ := updateSettings(testSettingsWithTailscale(t), tea.WindowSizeMsg{Width: 80, Height: 24})
	s, _ = updateSettings(s, keyPressMsg("f1"))

	require.NotNil(t, tailscalePanel(t, s).panel, "F1 opens the Tailscale panel")

	s, cmd := updateSettings(s, keyPressMsg("esc"))
	require.NotNil(t, cmd)
	closeMsg, ok := cmd().(TailscaleHelpCloseMsg)
	require.True(t, ok, "Esc on the panel produces a close command, not a dashboard navigation")

	s, navCmd := updateSettings(s, closeMsg)
	assert.Nil(t, tailscalePanel(t, s).panel, "Esc dismisses the panel")
	assert.Nil(t, navCmd, "the close command must not navigate away")
}

// Helpers

func testSettings() Settings {
	ns, _ := docker.NewNamespace("test")
	app := &docker.Application{
		Running: true,
		Settings: docker.ApplicationSettings{
			Name:  "test-app",
			Host:  "app.example.com",
			Image: "ghcr.io/basecamp/once-campfire:latest",
		},
	}

	return NewSettings(ns, app, SettingsSectionApplication)
}

func testSettingsWithTailscale(t *testing.T) Settings {
	t.Helper()
	ns, _ := docker.NewNamespace("test")
	app := &docker.Application{
		Running: true,
		Settings: docker.ApplicationSettings{
			Name:  "test-app",
			Host:  "app.example.com",
			Image: "ghcr.io/basecamp/once-campfire:latest",
		},
	}
	return NewSettings(ns, app, SettingsSectionTailscale)
}

func tailscalePanel(t *testing.T, s Settings) SettingsFormTailscale {
	t.Helper()
	section, ok := s.section.(SettingsFormTailscale)
	require.True(t, ok, "expected the Tailscale section")
	return section
}

func updateSettings(s Settings, msg tea.Msg) (Settings, tea.Cmd) {
	comp, cmd := s.Update(msg)
	return comp.(Settings), cmd
}
