package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocktailPrompt_DefaultFocusKeepsApp(t *testing.T) {
	p := NewDocktailPrompt("Service not advertised")
	assert.Equal(t, 1, p.focused, "deleting the app must never be the default")
}

func TestDocktailPrompt_KeysChooseDecisions(t *testing.T) {
	t.Run("r retries", func(t *testing.T) {
		decision, ok := promptDecision(t, "r")
		require.True(t, ok)
		assert.Equal(t, docktailRetry, decision)
	})

	t.Run("k keeps the app", func(t *testing.T) {
		decision, ok := promptDecision(t, "k")
		require.True(t, ok)
		assert.Equal(t, docktailKeep, decision)
	})

	t.Run("d deletes the app", func(t *testing.T) {
		decision, ok := promptDecision(t, "d")
		require.True(t, ok)
		assert.Equal(t, docktailRollback, decision)
	})

	t.Run("esc keeps the app", func(t *testing.T) {
		decision, ok := promptDecision(t, "esc")
		require.True(t, ok)
		assert.Equal(t, docktailKeep, decision)
	})

	t.Run("enter activates the default focus", func(t *testing.T) {
		decision, ok := promptDecision(t, "enter")
		require.True(t, ok)
		assert.Equal(t, docktailKeep, decision)
	})
}

func TestDocktailPrompt_TabCyclesFocus(t *testing.T) {
	t.Run("tab", func(t *testing.T) {
		p := NewDocktailPrompt("Service not advertised")

		p, _ = p.Update(keyPressMsg("tab"))
		assert.Equal(t, 2, p.focused)

		p, _ = p.Update(keyPressMsg("tab"))
		assert.Equal(t, 0, p.focused)
	})

	t.Run("shift+tab", func(t *testing.T) {
		p := NewDocktailPrompt("Service not advertised")

		p, _ = p.Update(keyPressMsg("shift+tab"))
		assert.Equal(t, 0, p.focused)

		p, _ = p.Update(keyPressMsg("shift+tab"))
		assert.Equal(t, 2, p.focused)
	})

	t.Run("enter activates the focused button", func(t *testing.T) {
		p := NewDocktailPrompt("Service not advertised")
		p, _ = p.Update(keyPressMsg("tab")) // focus delete

		_, cmd := p.Update(keyPressMsg("enter"))
		require.NotNil(t, cmd)

		msg, ok := cmd().(docktailDecisionMsg)
		require.True(t, ok)
		assert.Equal(t, docktailRollback, msg.decision)
	})
}

func TestDocktailPrompt_ClickChoosesDecision(t *testing.T) {
	t.Run("keep", func(t *testing.T) {
		p := NewDocktailPrompt("Service not advertised")

		_, cmd := p.Update(MouseEvent{IsClick: true, Target: "docktail-keep"})
		require.NotNil(t, cmd)

		msg, ok := cmd().(docktailDecisionMsg)
		require.True(t, ok)
		assert.Equal(t, docktailKeep, msg.decision)
	})

	t.Run("delete", func(t *testing.T) {
		p := NewDocktailPrompt("Service not advertised")

		_, cmd := p.Update(MouseEvent{IsClick: true, Target: "docktail-delete"})
		require.NotNil(t, cmd)

		msg, ok := cmd().(docktailDecisionMsg)
		require.True(t, ok)
		assert.Equal(t, docktailRollback, msg.decision)
	})
}

func TestDocktailPrompt_ViewShowsMessageAndButtons(t *testing.T) {
	p := NewDocktailPrompt("Service not advertised")
	view := p.View()

	assert.Contains(t, view, "Service not advertised")
	assert.Contains(t, view, "Retry")
	assert.Contains(t, view, "Keep app")
	assert.Contains(t, view, "Delete app")
}

// Helpers

func promptDecision(t *testing.T, key string) (docktailDecision, bool) {
	t.Helper()

	p := NewDocktailPrompt("Service not advertised")
	_, cmd := p.Update(keyPressMsg(key))
	if cmd == nil {
		return 0, false
	}

	msg, ok := cmd().(docktailDecisionMsg)
	if !ok {
		return 0, false
	}
	return msg.decision, true
}
