package ui

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"github.com/basecamp/once/internal/docker"
	"github.com/basecamp/once/internal/metrics"
)

func TestDashboardPanelViewRunningApp(t *testing.T) {
	panel := testPanel(true)
	view := panel.View(false, false, true, 120)
	stripped := ansi.Strip(view)

	assert.Contains(t, stripped, "app.example.com")
	assert.Contains(t, stripped, "running")
}

func TestDashboardPanelViewStoppedApp(t *testing.T) {
	panel := testPanel(false)
	view := panel.View(false, false, true, 120)
	stripped := ansi.Strip(view)

	assert.Contains(t, stripped, "app.example.com")
	assert.Contains(t, stripped, "stopped")
}

func TestDashboardPanelHeightRunning(t *testing.T) {
	panel := testPanel(true)

	fullHeight := panel.Height(true)
	compactHeight := panel.Height(false)

	assert.Equal(t, PanelHeight+2, fullHeight)
	assert.Equal(t, StoppedPanelHeight+2, compactHeight)
}

func TestDashboardPanelHeightStopped(t *testing.T) {
	panel := testPanel(false)

	// Stopped always uses compact height regardless of showDetails
	assert.Equal(t, StoppedPanelHeight+2, panel.Height(true))
	assert.Equal(t, StoppedPanelHeight+2, panel.Height(false))
}

func TestDashboardPanelHeights(t *testing.T) {
	assert.Equal(t, 7, PanelHeight)
	assert.Equal(t, 3, StoppedPanelHeight)
}

func TestDashboardPanelSelectedHasIndicator(t *testing.T) {
	panel := testPanel(true)
	view := panel.View(true, false, true, 80)

	// Selected panels have the indicator character
	assert.Contains(t, view, "▐")
	assert.Contains(t, view, "▗")
	assert.Contains(t, view, "▝")
}

func TestDashboardPanelNotSelectedNoIndicator(t *testing.T) {
	panel := testPanel(true)
	view := panel.View(false, false, true, 80)

	assert.NotContains(t, view, "▐")
	assert.NotContains(t, view, "▗")
	assert.NotContains(t, view, "▝")
}

func TestDashboardPanelNarrowWidthHidesCards(t *testing.T) {
	panel := testPanel(true)
	// Width of 30 is too narrow for cards (need minCardWidth*4+3 = 35 inner)
	view := panel.View(false, false, true, 30)

	// Should not contain card borders
	assert.NotContains(t, view, "╭─")
}

func TestDashboardPanelWideWidthShowsCards(t *testing.T) {
	panel := testPanel(true)
	view := panel.View(false, false, true, 120)

	// Cards should have borders
	assert.Contains(t, view, "╭─")
	assert.Contains(t, view, "╰")
}

func TestDashboardPanelTogglingShowsStatus(t *testing.T) {
	panel := testPanel(true)
	view := panel.View(false, true, true, 120)
	stripped := ansi.Strip(view)

	assert.Contains(t, stripped, "stopping...")
}

func TestDashboardPanelViewHasThreeLines(t *testing.T) {
	panel := testPanel(false)
	view := panel.View(false, false, false, 80)
	lines := strings.Split(view, "\n")

	// top transition + body lines + bottom transition
	assert.GreaterOrEqual(t, len(lines), 3)
}

func TestDashboardPanelIssuesLineAllClear(t *testing.T) {
	panel := testPanel(true)
	panel.Refresh(DashboardScales{})

	stripped := ansi.Strip(panel.View(false, false, true, 120))

	assert.Contains(t, stripped, "✓ no issues")
}

func TestDashboardPanelIssuesLineStoppedApp(t *testing.T) {
	panel := testPanel(false)
	panel.Refresh(DashboardScales{})

	stripped := ansi.Strip(panel.View(false, false, true, 120))

	assert.Contains(t, stripped, "✓ no issues")
	assert.NotContains(t, stripped, "⚠")
}

func TestDashboardPanelIssuesLineExited(t *testing.T) {
	panel := testPanelWithState(true, "exited")
	panel.app.ExitCode = 137
	panel.Refresh(DashboardScales{})

	stripped := ansi.Strip(panel.View(false, false, true, 120))

	assert.Contains(t, stripped, "⚠ exited with code 137")
}

func TestDashboardPanelIssuesLineRestarting(t *testing.T) {
	panel := testPanelWithState(true, "restarting")
	panel.app.RestartCount = 3
	panel.Refresh(DashboardScales{})

	stripped := ansi.Strip(panel.View(false, false, true, 120))

	assert.Contains(t, stripped, "⚠ restarting, 3 restarts")
}

func TestDashboardPanelIssuesLinePaused(t *testing.T) {
	panel := testPanelWithState(true, "paused")
	panel.Refresh(DashboardScales{})

	stripped := ansi.Strip(panel.View(false, false, true, 120))

	assert.Contains(t, stripped, "⚠ paused")
}

func TestDashboardPanelIssuesLineCreated(t *testing.T) {
	panel := testPanelWithState(true, "created")
	panel.Refresh(DashboardScales{})

	stripped := ansi.Strip(panel.View(false, false, true, 120))

	assert.Contains(t, stripped, "⚠ not started")
}

func TestDashboardPanelIssuesLineMetricBreach(t *testing.T) {
	panel := testPanel(true)
	panel.cards = [3]MetricCard{
		NewMetricCard("CPU", []float64{82}, ChartScale{max: 100}, UnitPercent, "", defaultWarningPct, defaultErrorPct),
	}
	panel.issues = panelIssues(&panel.app, panel.cards)

	stripped := ansi.Strip(panel.View(false, false, true, 120))

	assert.Contains(t, stripped, "⚠ CPU 82%")
}

func TestDashboardPanelIssuesLineMemoryBreach(t *testing.T) {
	panel := testPanel(true)
	panel.cards = [3]MetricCard{
		NewMetricCard("Memory", []float64{91}, ChartScale{max: 100}, UnitBytes, "", defaultWarningPct, defaultErrorPct),
	}
	panel.issues = panelIssues(&panel.app, panel.cards)

	stripped := ansi.Strip(panel.View(false, false, true, 120))

	assert.Contains(t, stripped, "⚠ memory 91%")
}

func TestDashboardPanelIssuesLineMetricBreachErrorRate(t *testing.T) {
	panel := testPanel(true)
	panel.cards = [3]MetricCard{
		NewTrafficCard([]float64{100}, []float64{4.2}, ChartScale{max: 100}, 4.2, trafficWarningPct, trafficErrorPct),
	}
	panel.issues = panelIssues(&panel.app, panel.cards)

	stripped := ansi.Strip(panel.View(false, false, true, 120))

	assert.Contains(t, stripped, "⚠ error rate 4.2%")
}

func TestFitIssues(t *testing.T) {
	t.Run("everything fits", func(t *testing.T) {
		shown, more := fitIssues([]string{"paused", "not started"}, 40)

		assert.Equal(t, []string{"paused", "not started"}, shown)
		assert.Equal(t, 0, more)
	})

	t.Run("some are left out", func(t *testing.T) {
		shown, more := fitIssues([]string{"exited with code 1", "restarting, 12 restarts", "paused"}, 30)

		assert.Equal(t, []string{"exited with code 1"}, shown)
		assert.Equal(t, 2, more)
	})

	t.Run("the last issue needs no room for a suffix", func(t *testing.T) {
		shown, more := fitIssues([]string{"paused", "not started"}, 24)

		assert.Equal(t, []string{"paused", "not started"}, shown)
		assert.Equal(t, 0, more)
	})

	t.Run("no room at all", func(t *testing.T) {
		shown, more := fitIssues([]string{"paused"}, 0)

		assert.Empty(t, shown)
		assert.Equal(t, 1, more)
	})
}

func TestDashboardPanelIssuesLineTruncates(t *testing.T) {
	panel := testPanel(true)
	panel.issues = []PanelIssue{
		{Text: "exited with code 1", Health: healthError},
		{Text: "restarting, 12 restarts", Health: healthWarning},
		{Text: "paused", Health: healthWarning},
		{Text: "not started", Health: healthWarning},
	}

	stripped := ansi.Strip(panel.View(false, false, true, 40))

	assert.Contains(t, stripped, "⚠ exited with code 1")
	assert.Contains(t, stripped, "+3 more")
	assert.NotContains(t, stripped, "paused")
}

func TestDashboardPanelIssuesLineJoinsWithSeparator(t *testing.T) {
	panel := testPanel(true)
	panel.issues = []PanelIssue{
		{Text: "exited with code 1", Health: healthError},
		{Text: "paused", Health: healthWarning},
	}

	stripped := ansi.Strip(panel.View(false, false, true, 120))

	assert.Contains(t, stripped, "exited with code 1 · paused")
}

func TestDashboardPanelBadgeIsWorstOfMetricsAndIssues(t *testing.T) {
	panel := testPanel(true)
	panel.cards = healthyCards()
	panel.issues = []PanelIssue{{Text: "paused", Health: healthWarning}}

	badge := panel.renderHealthBadge(panel.cards)

	assert.Equal(t, badgeWithColor(healthWarning.Color()), badge)
}

func TestDashboardPanelBadgeIsErrorForCrashedApp(t *testing.T) {
	panel := testPanelWithState(true, "exited")
	panel.cards = healthyCards()
	panel.issues = panelIssues(&panel.app, panel.cards)

	badge := panel.renderHealthBadge(panel.cards)

	assert.Equal(t, badgeWithColor(healthError.Color()), badge)
}

func TestDashboardPanelBadgeIsNormalWhenAllClear(t *testing.T) {
	panel := testPanel(true)
	panel.cards = healthyCards()

	badge := panel.renderHealthBadge(panel.cards)

	assert.Equal(t, badgeWithColor(healthNormal.Color()), badge)
}

func TestDashboardPanelBadgeIsWorstOfMetricAndIssue(t *testing.T) {
	panel := testPanel(true)
	panel.cards = [3]MetricCard{
		NewMetricCard("CPU", []float64{90}, ChartScale{max: 100}, UnitPercent, "", defaultWarningPct, defaultErrorPct),
	}
	panel.issues = []PanelIssue{{Text: "paused", Health: healthWarning}}

	badge := panel.renderHealthBadge(panel.cards)

	assert.Equal(t, badgeWithColor(healthError.Color()), badge, "the worse of the two wins")
}

func TestRenderStateInfo(t *testing.T) {
	running := &docker.Application{Running: true}
	stopped := &docker.Application{Running: false}

	assert.Contains(t, ansi.Strip(renderStateInfo(running, false)), "running")
	assert.Contains(t, ansi.Strip(renderStateInfo(stopped, false)), "stopped")
	assert.Contains(t, ansi.Strip(renderStateInfo(running, true)), "stopping...")
	assert.Contains(t, ansi.Strip(renderStateInfo(stopped, true)), "starting...")
}

func TestRenderBar(t *testing.T) {
	bar := renderBar(50, 0, 100, Colors.Success, 10)
	assert.NotEmpty(t, bar)
	// Bar contains braille characters
	assert.Contains(t, bar, "⢾")
	assert.Contains(t, bar, "⡷")
}

func TestRenderBarZeroWidth(t *testing.T) {
	assert.Empty(t, renderBar(50, 0, 100, Colors.Success, 0))
}

// Helpers

func testPanel(running bool) DashboardPanel {
	return testPanelWithState(running, "")
}

func testPanelWithState(running bool, state string) DashboardPanel {
	app := &docker.Application{
		Running: running,
		State:   state,
		Settings: docker.ApplicationSettings{
			Name:  "test-app",
			Host:  "app.example.com",
			Image: "ghcr.io/basecamp/test-app:latest",
		},
	}
	scraper := metrics.NewMetricsScraper(metrics.ScraperSettings{})
	dockerScraper := &docker.Scraper{}

	return NewDashboardPanel(app, scraper, dockerScraper, nil)
}

// badgeWithColor renders the health badge in the given colour, so a test can
// compare a badge against the severity it should have been drawn in.
func badgeWithColor(c color.Color) string {
	return lipgloss.NewStyle().Foreground(c).Render("●")
}

// healthyCards are the metric cards of an app with nothing to report.
func healthyCards() [3]MetricCard {
	return [3]MetricCard{
		NewMetricCard("CPU", []float64{10}, ChartScale{max: 100}, UnitPercent, "", defaultWarningPct, defaultErrorPct),
		NewMetricCard("Memory", []float64{10}, ChartScale{max: 100}, UnitBytes, "", defaultWarningPct, defaultErrorPct),
		NewTrafficCard([]float64{10}, []float64{0}, ChartScale{max: 100}, 0, trafficWarningPct, trafficErrorPct),
	}
}

func TestRenderStateInfoReflectsContainerState(t *testing.T) {
	state := func(s string, running bool, exit int) *docker.Application {
		return &docker.Application{State: s, Running: running, ExitCode: exit}
	}

	assert.Contains(t, renderStateInfo(state("running", true, 0), false), "running")
	assert.Contains(t, renderStateInfo(state("restarting", true, 0), false), "restarting")
	assert.Contains(t, renderStateInfo(state("exited", false, 1), false), "crashed")
	assert.Contains(t, renderStateInfo(state("exited", false, 0), false), "stopped")
	assert.Contains(t, renderStateInfo(state("paused", false, 0), false), "paused")
	assert.Contains(t, renderStateInfo(state("running", true, 0), true), "stopping")
	assert.Contains(t, renderStateInfo(state("exited", false, 0), true), "starting")
}
