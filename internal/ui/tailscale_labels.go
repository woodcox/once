package ui

import (
	"maps"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/basecamp/once/internal/docker"
)

// Helpers

func tailscaleLabelsText(settings docker.ApplicationSettings) string {
	header := Styles.Label.Render("Labels")

	labels := settings.DocktailLabels()
	if len(labels) == 0 {
		return header + "\n" + lipgloss.NewStyle().Foreground(Colors.Muted).Render("(not published)")
	}

	keys := slices.Sorted(maps.Keys(labels))
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+labels[key])
	}

	return header + "\n" + lipgloss.NewStyle().Foreground(Colors.Muted).Render(strings.Join(lines, "\n"))
}
