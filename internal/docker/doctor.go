package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DoctorStatus is the outcome of a single doctor check.
type DoctorStatus string

const (
	// DoctorPass marks a check that succeeded.
	DoctorPass DoctorStatus = "ok"
	// DoctorFail marks a check that failed.
	DoctorFail DoctorStatus = "FAIL"
	// DoctorSkip marks a check that doesn't apply.
	DoctorSkip DoctorStatus = "n/a"
)

// DoctorCheck is a single line of a doctor's checklist.
type DoctorCheck struct {
	Label   string
	Status  DoctorStatus
	Message string
	Remedy  string
}

// DoctorReport is the checklist for one application.
type DoctorReport struct {
	AppName string
	Host    string
	Checks  []DoctorCheck
	URL     string
	Note    string
}

// Failed reports whether any check in the report failed.
func (r DoctorReport) Failed() bool {
	for _, check := range r.Checks {
		if check.Status == DoctorFail {
			return true
		}
	}
	return false
}

// DoctorResult holds the reports for every inspected application.
type DoctorResult struct {
	Reports []DoctorReport
}

// Failed reports whether any application had a failing check.
func (r DoctorResult) Failed() bool {
	for _, report := range r.Reports {
		if report.Failed() {
			return true
		}
	}
	return false
}

// DoctorContainer describes the Docker container backing an application.
type DoctorContainer struct {
	Name   string
	State  string
	IP     string
	Labels map[string]string
}

// Running reports whether the container is running.
func (c DoctorContainer) Running() bool {
	return c.State == "running"
}

// DoctorProbes holds the read-only probes the doctor uses. Every probe is a
// function so the checks can be exercised without Docker or a live
// tailscaled. A nil probe means the check can't run and is reported as
// skipped.
type DoctorProbes struct {
	// AppContainer returns the container backing an application.
	AppContainer func(ctx context.Context, appName string) (DoctorContainer, error)
	// Containers lists every container on the Docker host.
	Containers func(ctx context.Context) ([]DoctorContainer, error)
	// HTTPGet performs a GET request and returns the response status code.
	HTTPGet func(ctx context.Context, url string) (int, error)
	// TCPDial opens a TCP connection to an address.
	TCPDial func(ctx context.Context, address string) error
	// ServeStatus returns the raw `tailscale serve status --json` output.
	ServeStatus func(ctx context.Context) ([]byte, error)
	// TailscaleStatus returns the raw `tailscale status --json` output.
	TailscaleStatus func(ctx context.Context) ([]byte, error)
	// ListeningPorts returns the ports the container listens on, when it can
	// be determined.
	ListeningPorts func(ctx context.Context, containerName string) ([]int, error)
}

// doctorProbeTimeout bounds every probe so a hung host can't stall the
// doctor.
const doctorProbeTimeout = 5 * time.Second

// doctorDialTimeout bounds the TCP check of an advertised backend port.
const doctorDialTimeout = 3 * time.Second

// ErrDoctorUnknownHost reports an argument that matches no deployed app.
var ErrDoctorUnknownHost = errors.New("unknown host")

// Doctor inspects the given applications and reports on each of them. It only
// inspects: nothing is deployed, started, stopped or written.
//
// An empty name inspects every application in the namespace; a name inspects
// only that application, matched on its host or its app name.
func Doctor(ctx context.Context, ns *Namespace, name string, probes DoctorProbes) (DoctorResult, error) {
	apps := ns.Applications()

	if name != "" {
		app := findDoctorApplication(apps, name)
		if app == nil {
			return DoctorResult{}, fmt.Errorf("%w: no application found at host or named %q", ErrDoctorUnknownHost, name)
		}
		apps = []*Application{app}
	}

	result := DoctorResult{}
	for _, app := range apps {
		result.Reports = append(result.Reports, doctorReport(ctx, app, probes))
	}

	return result, nil
}

// Private

// findDoctorApplication returns the application whose host or name matches
// the given value, or nil when there is none.
func findDoctorApplication(apps []*Application, name string) *Application {
	for _, app := range apps {
		if app.Settings.Host == name || app.Settings.Name == name {
			return app
		}
	}
	return nil
}

// doctorReport runs every check that applies to the app and collects the
// results.
func doctorReport(ctx context.Context, app *Application, probes DoctorProbes) DoctorReport {
	report := DoctorReport{
		AppName: app.Settings.Name,
		Host:    app.Settings.Host,
	}

	container, containerFound := doctorAppContainer(ctx, app, probes)

	report.Checks = append(report.Checks, doctorContainerCheck(container, containerFound))
	report.Checks = append(report.Checks, doctorAppPortCheck(ctx, app, container, containerFound, probes))

	if !app.Settings.DocktailEnabled() {
		report.URL = doctorPublicURL(app)
		return report
	}

	report.Checks = append(report.Checks, doctorLabelCheck(app, container, containerFound))
	report.Checks = append(report.Checks, doctorBackendPortCheck(ctx, app, container, containerFound, probes))
	report.Checks = append(report.Checks, doctorDocktailContainerCheck(ctx, probes))
	report.Checks = append(report.Checks, doctorAdvertisementCheck(ctx, app, container, containerFound, probes))

	url, urlCheck := doctorURLCheck(ctx, app, probes)
	report.Checks = append(report.Checks, urlCheck)
	report.URL = url

	report.Note = doctorApprovalNote

	return report
}

// doctorContainerCheck reports on the app's container.
func doctorContainerCheck(container DoctorContainer, found bool) DoctorCheck {
	if !found {
		return DoctorCheck{
			Label:   "container",
			Status:  DoctorFail,
			Message: "not found for this app",
			Remedy:  "redeploy the app so a container is created for it",
		}
	}

	if !container.Running() {
		return DoctorCheck{
			Label:   "container",
			Status:  DoctorFail,
			Message: fmt.Sprintf("%s is %s", container.Name, container.State),
			Remedy:  "start the app so its container runs",
		}
	}

	return DoctorCheck{
		Label:   "container",
		Status:  DoctorPass,
		Message: fmt.Sprintf("running (%s)", container.Name),
	}
}

// doctorAppPortCheck reports whether the app answers its health check over
// HTTP on its own port inside the container.
func doctorAppPortCheck(ctx context.Context, app *Application, container DoctorContainer, found bool, probes DoctorProbes) DoctorCheck {
	if !found {
		return DoctorCheck{
			Label:   "app port",
			Status:  DoctorSkip,
			Message: ": no container to check",
		}
	}

	if !container.Running() {
		return DoctorCheck{
			Label:   "app port",
			Status:  DoctorSkip,
			Message: ": container is not running",
		}
	}

	if container.IP == "" {
		return DoctorCheck{
			Label:   "app port",
			Status:  DoctorFail,
			Message: ": the container has no IP address on the Docker network",
			Remedy:  "redeploy the app so it is attached to the Docker network",
		}
	}

	port := app.Settings.EffectiveAppPort()
	path := app.Settings.EffectiveHealthCheckPath()

	url := fmt.Sprintf("http://%s%s", net.JoinHostPort(container.IP, strconv.Itoa(port)), path)

	status, err := doctorHTTPGet(ctx, probes, url)
	if err != nil {
		return DoctorCheck{
			Label:   "app port",
			Status:  DoctorFail,
			Message: fmt.Sprintf("%d did not answer %s: %v", port, path, err),
			Remedy:  fmt.Sprintf("make sure the app listens on port %d inside the container", port),
		}
	}

	if status < 200 || status > 299 {
		return DoctorCheck{
			Label:   "app port",
			Status:  DoctorFail,
			Message: fmt.Sprintf("%d answered %s with %d", port, path, status),
			Remedy:  fmt.Sprintf("make sure the app serves %s on port %d with a 2xx response", path, port),
		}
	}

	return DoctorCheck{
		Label:   "app port",
		Status:  DoctorPass,
		Message: fmt.Sprintf("%d answers %s with %d", port, path, status),
	}
}

// doctorLabelCheck compares the docktail labels on the container with the
// app's settings.
func doctorLabelCheck(app *Application, container DoctorContainer, found bool) DoctorCheck {
	if !found {
		return DoctorCheck{
			Label:   "docktail labels",
			Status:  DoctorSkip,
			Message: ": no container to check",
		}
	}

	expected := map[string]string{
		docktailLabelEnable:      "true",
		docktailLabelName:        app.Settings.EffectiveServiceName(),
		docktailLabelPort:        strconv.Itoa(app.Settings.EffectiveTailscalePort()),
		docktailLabelServicePort: strconv.Itoa(app.Settings.EffectiveServicePort()),
	}

	var problems []string
	for _, key := range slices.Sorted(maps.Keys(expected)) {
		want := expected[key]
		got, ok := container.Labels[key]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s is missing", key))
			continue
		}
		if got != want {
			problems = append(problems, fmt.Sprintf("%s is %q, expected %q", key, got, want))
		}
	}

	if len(problems) > 0 {
		return DoctorCheck{
			Label:   "docktail labels",
			Status:  DoctorFail,
			Message: ": " + strings.Join(problems, "; "),
			Remedy:  "redeploy the app so its docktail labels match its settings",
		}
	}

	return DoctorCheck{
		Label:  "docktail labels",
		Status: DoctorPass,
		Message: fmt.Sprintf(": service %s, backend port %d, service port %d",
			expected[docktailLabelName],
			app.Settings.EffectiveTailscalePort(),
			app.Settings.EffectiveServicePort()),
	}
}

// doctorBackendPortCheck reports whether the advertised backend port is
// actually listening inside the container.
func doctorBackendPortCheck(ctx context.Context, app *Application, container DoctorContainer, found bool, probes DoctorProbes) DoctorCheck {
	if !found || !container.Running() {
		return DoctorCheck{
			Label:   "backend port",
			Status:  DoctorSkip,
			Message: ": container is not running",
		}
	}

	if container.IP == "" {
		return DoctorCheck{
			Label:   "backend port",
			Status:  DoctorSkip,
			Message: ": the container has no IP address on the Docker network",
		}
	}

	port := app.Settings.EffectiveTailscalePort()

	// In `direct=false` mode docktail reaches the backend through a
	// loopback binding on the host rather than the container's network IP.
	address := net.JoinHostPort(container.IP, strconv.Itoa(port))
	if !app.Settings.effectiveDirect() {
		address = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}

	err := doctorTCPDial(ctx, probes, address)
	if err != nil {
		message := fmt.Sprintf("%d: nothing is listening inside the container", port)
		if ports := doctorListeningPorts(ctx, container.Name, probes); len(ports) > 0 {
			message += fmt.Sprintf(" (listening: %s)", formatPorts(ports))
		}

		return DoctorCheck{
			Label:   "backend port",
			Status:  DoctorFail,
			Message: message,
			Remedy:  fmt.Sprintf("set the Tailscale backend port to the port the app listens on (%d)", app.Settings.EffectiveAppPort()),
		}
	}

	return DoctorCheck{
		Label:   "backend port",
		Status:  DoctorPass,
		Message: fmt.Sprintf("%d is listening inside the container", port),
	}
}

// doctorDocktailContainerCheck reports whether a docktail container is running
// on this host.
func doctorDocktailContainerCheck(ctx context.Context, probes DoctorProbes) DoctorCheck {
	if probes.Containers == nil {
		return DoctorCheck{
			Label:   "docktail container",
			Status:  DoctorSkip,
			Message: ": container listing is unavailable",
		}
	}

	containers, err := probes.Containers(ctx)
	if err != nil {
		return DoctorCheck{
			Label:   "docktail container",
			Status:  DoctorFail,
			Message: fmt.Sprintf(": couldn't list containers: %v", err),
			Remedy:  "make sure the Docker daemon is reachable",
		}
	}

	for _, container := range containers {
		if container.Name == "docktail" && container.Running() {
			return DoctorCheck{
				Label:   "docktail container",
				Status:  DoctorPass,
				Message: "is running",
			}
		}
	}

	return DoctorCheck{
		Label:   "docktail container",
		Status:  DoctorFail,
		Message: ": no running docktail container found",
		Remedy:  "start the docktail container so it advertises the app as a Tailscale Service",
	}
}

// doctorAdvertisementCheck reports whether the Tailscale Service is
// advertised for this container.
func doctorAdvertisementCheck(ctx context.Context, app *Application, container DoctorContainer, found bool, probes DoctorProbes) DoctorCheck {
	if probes.ServeStatus == nil {
		return DoctorCheck{
			Label:   "advertised",
			Status:  DoctorSkip,
			Message: ": tailscale serve status is unavailable",
		}
	}

	data, err := probes.ServeStatus(ctx)
	if err != nil {
		return DoctorCheck{
			Label:   "advertised",
			Status:  DoctorFail,
			Message: fmt.Sprintf(": couldn't read the Tailscale serve status: %v", err),
			Remedy:  "make sure the tailscale CLI is installed and the node is logged in",
		}
	}

	service := app.Settings.EffectiveServiceName()
	target, advertised := tailscaleServeTargetFor(data, service)
	if !advertised {
		return DoctorCheck{
			Label:   "advertised",
			Status:  DoctorFail,
			Message: fmt.Sprintf(": svc:%s is not advertised", service),
			Remedy:  fmt.Sprintf("check that the service name %q matches the one in your Tailscale admin console", service),
		}
	}

	if !found {
		return DoctorCheck{
			Label:   "advertised",
			Status:  DoctorSkip,
			Message: fmt.Sprintf(": svc:%s is advertised at %s", service, target.URL()),
		}
	}

	wantPort := strconv.Itoa(app.Settings.EffectiveTailscalePort())
	if target.port != wantPort {
		return DoctorCheck{
			Label:   "advertised",
			Status:  DoctorFail,
			Message: fmt.Sprintf(": svc:%s is advertised, but points at %s instead of port %s", service, target.URL(), wantPort),
			Remedy:  "restart the docktail container so it advertises the current container",
		}
	}

	// In `direct=false` mode docktail proxies through the host's loopback
	// binding, so the handler's host isn't the container's network IP.
	direct := app.Settings.effectiveDirect()
	if direct && target.host != container.IP {
		return DoctorCheck{
			Label:   "advertised",
			Status:  DoctorFail,
			Message: fmt.Sprintf(": svc:%s is advertised, but points at %s instead of %s", service, target.URL(), net.JoinHostPort(container.IP, wantPort)),
			Remedy:  "restart the docktail container so it advertises the current container",
		}
	}

	return DoctorCheck{
		Label:   "advertised",
		Status:  DoctorPass,
		Message: fmt.Sprintf(": svc:%s -> %s", service, target.URL()),
	}
}

// doctorURLCheck works out the URL the app is reachable at on the tailnet.
// The URL is returned separately so it can be reported once, rather than as
// a check of its own.
func doctorURLCheck(ctx context.Context, app *Application, probes DoctorProbes) (string, DoctorCheck) {
	service := app.Settings.EffectiveServiceName()

	suffix := doctorMagicDNSSuffix(ctx, probes)
	if suffix == "" {
		return "", DoctorCheck{
			Label:   "url",
			Status:  DoctorSkip,
			Message: ": the tailnet's MagicDNS suffix is unavailable",
		}
	}

	url := "https://" + service + "." + suffix

	return url, DoctorCheck{
		Label:   "url",
		Status:  DoctorPass,
		Message: url,
	}
}

// doctorMagicDNSSuffix returns the tailnet's MagicDNS suffix, taken from the
// node's own DNS name.
func doctorMagicDNSSuffix(ctx context.Context, probes DoctorProbes) string {
	if probes.TailscaleStatus == nil {
		return ""
	}

	data, err := probes.TailscaleStatus(ctx)
	if err != nil {
		return ""
	}

	var status struct {
		Self struct {
			DNSName string `json:"DNSName"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return ""
	}

	name := strings.TrimSuffix(status.Self.DNSName, ".")
	if name == "" {
		return ""
	}

	_, suffix, ok := strings.Cut(name, ".")
	if !ok {
		return ""
	}

	return suffix
}

// doctorPublicURL returns the app's public URL, when its settings have one.
func doctorPublicURL(app *Application) string {
	if app.Settings.Host == "" {
		return ""
	}
	if app.Settings.TLSEnabled() {
		return "https://" + app.Settings.Host
	}
	return "http://" + app.Settings.Host
}

// doctorAppContainer finds the container backing the app.
func doctorAppContainer(ctx context.Context, app *Application, probes DoctorProbes) (DoctorContainer, bool) {
	if probes.AppContainer == nil {
		return DoctorContainer{}, false
	}

	container, err := probes.AppContainer(ctx, app.Settings.Name)
	if err != nil {
		return DoctorContainer{}, false
	}

	return container, true
}

// doctorListeningPorts returns the ports the container listens on, when the
// probe can determine them.
func doctorListeningPorts(ctx context.Context, containerName string, probes DoctorProbes) []int {
	if probes.ListeningPorts == nil {
		return nil
	}

	ports, err := probes.ListeningPorts(ctx, containerName)
	if err != nil {
		return nil
	}

	return ports
}

// Helpers

// doctorApprovalNote is shown after a docktail app's checklist.
const doctorApprovalNote = "service approval and client grants live in the Tailscale admin console, so they can't be verified from here"

// doctorHTTPGet performs the app's health check request through the probe.
func doctorHTTPGet(ctx context.Context, probes DoctorProbes, url string) (int, error) {
	if probes.HTTPGet == nil {
		return 0, errors.New("no HTTP probe is available")
	}

	ctx, cancel := context.WithTimeout(ctx, doctorProbeTimeout)
	defer cancel()

	return probes.HTTPGet(ctx, url)
}

// doctorTCPDial dials the backend port through the probe.
func doctorTCPDial(ctx context.Context, probes DoctorProbes, address string) error {
	if probes.TCPDial == nil {
		return errors.New("no TCP probe is available")
	}

	ctx, cancel := context.WithTimeout(ctx, doctorProbeTimeout)
	defer cancel()

	return probes.TCPDial(ctx, address)
}

// tailscaleServeTarget is a proxy target from the Tailscale serve status.
type tailscaleServeTarget struct {
	scheme string
	host   string
	port   string
}

func (t tailscaleServeTarget) String() string {
	if t.port == "" {
		return t.host
	}
	return net.JoinHostPort(t.host, t.port)
}

// URL returns the proxy target as a URL.
func (t tailscaleServeTarget) URL() string {
	scheme := t.scheme
	if scheme == "" {
		scheme = "http"
	}
	return scheme + "://" + t.String()
}

// tailscaleServeTargetFor returns the proxy target advertised for a Tailscale
// Service, and whether the Service is advertised at all. It walks the same
// serve status the verification path uses, so the two can't disagree.
func tailscaleServeTargetFor(data []byte, serviceName string) (tailscaleServeTarget, bool) {
	var status tailscaleServeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return tailscaleServeTarget{}, false
	}

	for name, advertised := range status.Services {
		if normalizeServiceName(name) != normalizeServiceName(serviceName) {
			continue
		}

		for _, web := range advertised.Web {
			for _, handler := range web.Handlers {
				return parseProxyTarget(handler.Proxy), true
			}
		}

		return tailscaleServeTarget{}, false
	}

	return tailscaleServeTarget{}, false
}

// parseProxyTarget splits a serve handler's proxy URL into a host and port.
func parseProxyTarget(proxy string) tailscaleServeTarget {
	u, err := url.Parse(proxy)
	if err != nil {
		return tailscaleServeTarget{host: proxy}
	}

	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}

	return tailscaleServeTarget{scheme: u.Scheme, host: u.Hostname(), port: port}
}

// formatPorts renders a list of ports in a compact, readable form.
func formatPorts(ports []int) string {
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, strconv.Itoa(port))
	}
	return strings.Join(parts, ", ")
}
