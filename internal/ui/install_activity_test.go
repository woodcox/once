package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestInstallActivity_PromptRaisesAndClears(t *testing.T) {
	m := newTestActivity()

	m.Update(installPromptMsg{})
	require.NotNil(t, m.prompt)
	assert.Contains(t, m.activityView(), "Keep app")
	assert.Equal(t, 1, m.prompt.focused)
}

func TestInstallActivity_DecisionIsDelivered(t *testing.T) {
	m := newTestActivity()
	m.Update(installPromptMsg{})

	cmd := m.Update(keyPressMsg("d"))
	require.NotNil(t, cmd)

	m.Update(cmd())
	assert.Nil(t, m.prompt, "the prompt is dismissed once a decision is made")

	select {
	case decision := <-m.decisionChan:
		assert.Equal(t, docktailRollback, decision)
	default:
		t.Fatal("expected a decision on the decision channel, got none")
	}
}

func TestInstallActivity_KeysIgnoredWithoutPrompt(t *testing.T) {
	m := newTestActivity()

	assert.Nil(t, m.Update(keyPressMsg("d")))
	assert.Nil(t, m.Update(keyPressMsg("esc")))
}

func TestInstallActivity_WarningsAreDrainedBeforeDone(t *testing.T) {
	m := newTestActivity()

	m.warningChan <- installWarningMsg{warnings: []string{"not advertised yet"}}
	m.doneChan <- installDoneMsg{}

	msg := m.waitForProgress()()
	_, ok := msg.(installWarningMsg)
	assert.True(t, ok, "expected installWarningMsg, got %T", msg)
}

func TestInstall_ActivityKeysReachThePrompt(t *testing.T) {
	m := newTestInstall()
	m.state = installStateActivity
	m.activity = newTestActivity()
	m.activity.Update(installPromptMsg{})

	m, cmd := updateInstall(m, keyPressMsg("k"))
	require.NotNil(t, cmd)

	msg, ok := cmd().(docktailDecisionMsg)
	require.True(t, ok, "expected docktailDecisionMsg, got %T", msg)
	assert.Equal(t, docktailKeep, msg.decision)
}

func TestInstall_ActivityKeysAreInertWithoutAPrompt(t *testing.T) {
	m := newTestInstall()
	m.state = installStateActivity
	m.activity = newTestActivity()

	_, cmd := updateInstall(m, keyPressMsg("esc"))
	assert.Nil(t, cmd, "esc on the install activity does nothing unless the prompt is up")
}

// Helpers

func newTestActivity() *InstallActivity {
	return NewInstallActivity(newTestNamespace(), "ghcr.io/basecamp/once-campfire:latest", "app.example.com", docker.RegistrySettings{}, docker.ApplicationSettings{})
}

func (m *InstallActivity) activityView() string {
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	return m.View()
}
