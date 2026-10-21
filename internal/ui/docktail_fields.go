package ui

import "strings"

// docktailField describes one docktail.* label the install and settings
// Tailscale forms can write and its F1 help panel entry. The title is what
// the form shows in its label; the help lines are what the panel renders
// under the docktail key.
type docktailField struct {
	key         string
	title       string
	required    bool
	summary     string
	defaultLine string
	accepted    string
}

var docktailFields = []docktailField{
	{
		title:       "Service name",
		key:         "docktail.service.name",
		required:    true,
		summary:     "Tailscale Service name shown on your tailnet.",
		defaultLine: "Default: the application name, sanitised to a DNS label.",
	},
	{
		title:       "Description",
		key:         "docktail.service.description",
		summary:     "Shown in the Tailscale admin console next to the Service.",
		defaultLine: "Default: blank.",
		accepted:    "Synced only when docktail has Tailscale API credentials.",
	},
	{
		title:       "Backend port",
		key:         "docktail.service.port",
		required:    true,
		summary:     "Container port docktail proxies to.",
		defaultLine: "Default: the application's port.",
		accepted:    "Accepted: 1–65535.",
	},
	{
		title:       "Backend protocol",
		key:         "docktail.service.protocol",
		summary:     "Protocol docktail uses between itself and the container.",
		defaultLine: "Default: https on port 443, http otherwise.",
		accepted:    "Accepted: http, https, https+insecure, tcp, tls-terminated-tcp.",
	},
	{
		title:       "Service port",
		key:         "docktail.service.service-port",
		summary:     "Port Tailscale listens on.",
		defaultLine: "Default: 443 for an HTTPS Service, 80 for HTTP or TCP.",
		accepted:    "Accepted: 1–65535.",
	},
	{
		title:       "Service protocol",
		key:         "docktail.service.service-protocol",
		summary:     "Tailscale-facing protocol.",
		defaultLine: "Default: derived from the service port and backend protocol.",
		accepted:    "Accepted: http, https, tcp, tls-terminated-tcp.",
	},
	{
		title:       "Service path",
		key:         "docktail.service.path",
		summary:     "URL path the Service serves under.",
		defaultLine: "Default: /.",
		accepted:    "Not allowed when the service protocol is TCP.",
	},
	{
		title:       "Service tags",
		key:         "docktail.tags",
		summary:     "Comma-separated Tailscale Service tags.",
		defaultLine: "Default: tag:container.",
	},
	{
		title:       "Network",
		key:         "docktail.service.network",
		summary:     "Docker network docktail uses to reach the container IP.",
		defaultLine: "Default: blank — lets docktail pick one.",
	},
	{
		title:       "Proxy protocol",
		key:         "docktail.service.proxy-protocol",
		summary:     "PROXY protocol version docktail emits towards the container.",
		defaultLine: "Default: off.",
		accepted:    "Accepted: 1 or 2; only meaningful for TCP service protocols.",
	},
	{
		title:       "Direct",
		key:         "docktail.service.direct",
		summary:     "When true, docktail proxies straight to the container IP.",
		defaultLine: "Default: true.",
		accepted:    "When false, ONCE publishes the backend port on loopback only and docktail proxies through that loopback binding instead.",
	},
}

func renderTailscaleHelpContent() string {
	var lines []string
	for _, entry := range docktailFields {
		var header string
		switch {
		case entry.required:
			header = entry.title + " (required)"
		default:
			header = entry.title + " (optional)"
		}
		lines = append(lines, header, entry.key, entry.summary)
		if entry.defaultLine != "" {
			lines = append(lines, entry.defaultLine)
		}
		if entry.accepted != "" {
			lines = append(lines, entry.accepted)
		}
		lines = append(lines, "")
	}
	return joinLines(lines)
}

func joinLines(lines []string) string {
	var sb strings.Builder
	for i, line := range lines {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(line)
	}
	return sb.String()
}
