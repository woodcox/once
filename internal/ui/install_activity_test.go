package ui

import (
	"testing"
	"time"

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

func TestInstallActivity_SecondDecisionIsIgnored(t *testing.T) {
	m := newTestActivity()
	m.Update(installPromptMsg{})

	cmd := m.Update(keyPressMsg("d"))
	require.NotNil(t, cmd)
	m.Update(cmd())
	require.Nil(t, m.prompt, "the prompt is dismissed once a decision is made")

	select {
	case decision := <-m.decisionChan:
		assert.Equal(t, docktailRollback, decision)
	default:
		t.Fatal("expected a decision on the decision channel, got none")
	}

	cmd = m.Update(keyPressMsg("d"))
	require.Nil(t, cmd, "a second decision after the prompt is cleared produces no command")

	select {
	case decision := <-m.decisionChan:
		t.Fatalf("second decision should be ignored, got %v", decision)
	default:
	}
}

func TestInstallActivity_DecisionSendNeverBlocks(t *testing.T) {
	m := newTestActivity()
	m.Update(installPromptMsg{})

	cmd := m.Update(keyPressMsg("k"))
	require.NotNil(t, cmd)
	m.Update(cmd())

	select {
	case decision := <-m.decisionChan:
		assert.Equal(t, docktailKeep, decision)
	default:
		t.Fatal("expected the first decision on the channel")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		cmd := m.Update(keyPressMsg("k"))
		assert.Nil(t, cmd, "no prompt means the key produces no command")

		cmd = m.Update(docktailDecisionMsg{decision: docktailKeep})
		assert.Nil(t, cmd, "a stray decision after the prompt is cleared produces no command")
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Update blocked when decision channel had no consumer")
	}

	select {
	case decision := <-m.decisionChan:
		t.Fatalf("extra decisions should be dropped, got %v", decision)
	default:
	}
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
