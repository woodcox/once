package ui

import (
	"fmt"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestDoctor_RendersPassingChecks(t *testing.T) {
	m := newTestDoctor()

	m, _ = updateDoctor(m, doctorFinishedMsg{result: docker.DoctorResult{
		Reports: []docker.DoctorReport{{
			AppName: "myapp",
			Host:    "myapp.example.com",
			Checks: []docker.DoctorCheck{
				{Label: "container", Status: docker.DoctorPass, Message: "running (once-app-myapp-1234)"},
			},
		}},
	}})

	view := m.View()

	assert.Contains(t, view, "myapp")
	assert.Contains(t, view, "myapp.example.com")
	assert.Contains(t, view, "✓")
	assert.Contains(t, view, "container running")
}

func TestDoctor_RendersFailedCheckWithRemedy(t *testing.T) {
	m := newTestDoctor()

	m, _ = updateDoctor(m, doctorFinishedMsg{result: docker.DoctorResult{
		Reports: []docker.DoctorReport{{
			AppName: "myapp",
			Checks: []docker.DoctorCheck{
				{Label: "app port", Status: docker.DoctorFail, Message: ": did not answer", Remedy: "make sure the app listens on port 3000"},
			},
		}},
	}})

	view := m.View()

	assert.Contains(t, view, "✗")
	assert.Contains(t, view, "app port")
	assert.Contains(t, view, "remedy: make sure the app listens on port 3000")
}

func TestDoctor_RendersNote(t *testing.T) {
	m := newTestDoctor()

	m, _ = updateDoctor(m, doctorFinishedMsg{result: docker.DoctorResult{
		Reports: []docker.DoctorReport{{
			AppName: "myapp",
			Note:    "service approval and client grants live in the Tailscale admin console",
		}},
	}})

	assert.Contains(t, m.View(), "note: service approval and client grants live in the Tailscale admin console")
}

func TestDoctor_InitRunsTheCheckAsynchronously(t *testing.T) {
	m := newTestDoctor()

	cmd := m.Init()
	require.NotNil(t, cmd)

	assert.Contains(t, m.View(), "Checking myapp")
}

func TestDoctor_RefreshRunsTheCheckAgain(t *testing.T) {
	m := newTestDoctor()

	m, _ = updateDoctor(m, doctorFinishedMsg{result: docker.DoctorResult{
		Reports: []docker.DoctorReport{{AppName: "myapp"}},
	}})

	m, cmd := updateDoctor(m, keyPressMsg("r"))
	require.NotNil(t, cmd)

	assert.True(t, m.checking)
	assert.Contains(t, m.View(), "Checking myapp")
}

func TestDoctor_EscReturnsToDashboard(t *testing.T) {
	m := newTestDoctor()

	_, cmd := updateDoctor(m, keyPressMsg("esc"))
	require.NotNil(t, cmd)

	msg := cmd()
	navMsg, ok := msg.(NavigateToDashboardMsg)
	require.True(t, ok, "expected NavigateToDashboardMsg, got %T", msg)
	assert.Equal(t, "myapp", navMsg.AppName)
}

func TestDoctor_ErrorRendersInsteadOfBlankScreen(t *testing.T) {
	m := newTestDoctor()

	m, _ = updateDoctor(m, doctorFinishedMsg{err: assert.AnError})

	view := m.View()

	assert.Contains(t, view, assert.AnError.Error())
	assert.False(t, m.checking)
}

func TestDoctor_HelpShowsRefreshAndBack(t *testing.T) {
	m := newTestDoctor()

	view := m.View()

	assert.Contains(t, view, "re-run")
	assert.Contains(t, view, "back")
}

func TestDoctor_ShowsCheckingStateWhileRunning(t *testing.T) {
	m := newTestDoctor()

	m, _ = updateDoctor(m, doctorFinishedMsg{result: docker.DoctorResult{}})
	m, cmd := updateDoctor(m, keyPressMsg("r"))
	require.NotNil(t, cmd)

	assert.Contains(t, m.View(), "Checking myapp")
}

func TestDoctor_ViewFillsTheTerminalHeight(t *testing.T) {
	m := newTestDoctor()

	m, _ = updateDoctor(m, doctorFinishedMsg{result: docker.DoctorResult{
		Reports: []docker.DoctorReport{{AppName: "myapp"}},
	}})

	assert.Equal(t, m.height, lineCountAt(m.View()))
}

func TestDoctor_ScrollsWhenTheReportIsTallerThanTheScreen(t *testing.T) {
	m := newTestDoctor()

	var checks []docker.DoctorCheck
	for i := range 40 {
		checks = append(checks, docker.DoctorCheck{
			Label:   "container",
			Status:  docker.DoctorPass,
			Message: fmt.Sprintf("running (container-%d)", i),
		})
	}

	m, _ = updateDoctor(m, doctorFinishedMsg{result: docker.DoctorResult{
		Reports: []docker.DoctorReport{{AppName: "myapp", Checks: checks}},
	}})

	assert.Equal(t, 0, m.viewport.YOffset())

	m, _ = updateDoctor(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	assert.Greater(t, m.viewport.YOffset(), 0)

	m, _ = updateDoctor(m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	assert.Equal(t, 0, m.viewport.YOffset())
}

// Helpers

func newTestDoctor() Doctor {
	h := NewHelp()
	h.SetBindings([]key.Binding{doctorKeys.Refresh, doctorKeys.Back})

	m := Doctor{
		app: &docker.Application{
			Settings: docker.ApplicationSettings{Name: "myapp", Host: "myapp.example.com"},
		},
		viewport: newTestDoctorViewport(),
		help:     h,
		probes:   docker.DoctorProbes{},
		checking: true,
	}

	m.width, m.height = 80, 24
	m.help.SetWidth(m.width)
	m.updateViewportSize()
	m.rebuildContent()

	return m
}

func newTestDoctorViewport() viewport.Model {
	vp := viewport.New()
	vp.MouseWheelEnabled = true
	vp.SoftWrap = true
	return vp
}

func updateDoctor(m Doctor, msg tea.Msg) (Doctor, tea.Cmd) {
	comp, cmd := m.Update(msg)
	return comp.(Doctor), cmd
}
