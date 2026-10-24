package ui

import (
	"fmt"

	"github.com/basecamp/once/internal/docker"
)

// PanelIssue is one problem worth telling the user about on a dashboard
// panel: either the app's container is misbehaving, or one of its metrics has
// breached its threshold.
type PanelIssue struct {
	// Text is the issue as it reads on the panel.
	Text string
	// Health is the severity of the issue.
	Health HealthState
}

// Helpers

// panelIssues collects the issues for an app from the container state it is
// known to have and the metric cards it is showing. It makes no network
// calls: everything it needs is already in hand.
func panelIssues(app *docker.Application, cards [3]MetricCard) []PanelIssue {
	var issues []PanelIssue

	issues = append(issues, containerIssues(app)...)
	issues = append(issues, metricIssues(cards)...)

	return issues
}

// containerIssues reports the problems visible in the app's container state.
// A stopped app has no container to report on, so it has no issues.
func containerIssues(app *docker.Application) []PanelIssue {
	if !app.Running {
		return nil
	}

	switch app.State {
	case "exited", "dead":
		return []PanelIssue{{
			Text:   fmt.Sprintf("exited with code %d", app.ExitCode),
			Health: healthError,
		}}
	case "restarting":
		return []PanelIssue{{
			Text:   fmt.Sprintf("restarting, %d restarts", app.RestartCount),
			Health: healthWarning,
		}}
	case "paused":
		return []PanelIssue{{
			Text:   "paused",
			Health: healthWarning,
		}}
	case "created":
		return []PanelIssue{{
			Text:   "not started",
			Health: healthWarning,
		}}
	}

	if app.Health == "unhealthy" {
		return []PanelIssue{{
			Text:   "unhealthy",
			Health: healthError,
		}}
	}

	return nil
}

// metricIssues reports the metrics that have breached their warning
// threshold.
func metricIssues(cards [3]MetricCard) []PanelIssue {
	var issues []PanelIssue

	for _, c := range cards {
		if c.Health() < healthWarning {
			continue
		}

		issues = append(issues, PanelIssue{
			Text:   metricIssueText(c),
			Health: c.Health(),
		})
	}

	return issues
}

// metricIssueText describes a metric breach, e.g. "CPU 82%".
func metricIssueText(card MetricCard) string {
	if card.isTraffic {
		return fmt.Sprintf("%s %.1f%%", card.issueLabel, card.healthPct)
	}
	return fmt.Sprintf("%s %s", card.issueLabel, UnitPercent.Format(card.healthPct))
}

// worstIssueHealth is the severity of the worst of the given issues.
func worstIssueHealth(issues []PanelIssue) HealthState {
	worst := healthNormal
	for _, issue := range issues {
		worst = max(worst, issue.Health)
	}
	return worst
}
