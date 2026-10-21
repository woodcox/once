package ui

type docktailField int

const (
	docktailFieldServiceName docktailField = iota
	docktailFieldDescription
	docktailFieldBackendPort
	docktailFieldBackendProtocol
	docktailFieldServicePort
	docktailFieldServiceProtocol
	docktailFieldPath
	docktailFieldTags
	docktailFieldNetwork
	docktailFieldProxyProtocol
	docktailFieldDirect
)

// docktailFieldDescriptions is the one-line muted explanation rendered under
// each docktail form field, written to match docktail's documented semantics
// so the UI doesn't have to repeat them in error messages.
var docktailFieldDescriptions = map[docktailField]string{
	docktailFieldServiceName:     "Tailscale Service name. Defaults to the application name, sanitised to a DNS label.",
	docktailFieldDescription:     "Shown in the Tailscale admin console. Synced only when docktail has API credentials.",
	docktailFieldBackendPort:     "Container port docktail proxies to. Required to mean something — blank uses the application port.",
	docktailFieldBackendProtocol: "Backend protocol. Blank = https on port 443, otherwise http. One of http, https, https+insecure, tcp, tls-terminated-tcp.",
	docktailFieldServicePort:     "Port Tailscale listens on. Blank = 443 for HTTPS, 80 for HTTP, 80 for TCP.",
	docktailFieldServiceProtocol: "Tailscale-facing protocol. Blank is derived from the chosen port. One of http, https, tcp, tls-terminated-tcp.",
	docktailFieldPath:            "URL path the Service serves under. Blank = /. Not allowed when the service protocol is TCP.",
	docktailFieldTags:            "Comma-separated Tailscale Service tags. Defaults to tag:container.",
	docktailFieldNetwork:         "Docker network used to find the container IP. Blank lets docktail pick one.",
	docktailFieldProxyProtocol:   "PROXY protocol version. Blank = off. Allowed: 1 or 2; only meaningful for TCP service protocols.",
	docktailFieldDirect:          "When unticked, ONCE publishes the backend port on loopback only — docktail then proxies through that loopback binding instead of the container IP.",
}
