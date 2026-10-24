package docker

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

var (
	ErrApplicationExists               = errors.New("application already exists")
	ErrHostnameInUse                   = errors.New("hostname already in use")
	ErrHostRequired                    = errors.New("host is required")
	ErrInvalidBackup                   = errors.New("invalid backup archive")
	ErrImageRequired                   = errors.New("image is required")
	ErrApplicationNotRunning           = errors.New("the application is not running")
	ErrBackupPathRelative              = errors.New("backup path must be absolute")
	ErrBackupFileExists                = errors.New("backup file already exists")
	ErrAutoBackupWithoutPath           = errors.New("auto-backup requires a backup path")
	ErrInvalidAppPort                  = errors.New("app port must be between 0 and 65535")
	ErrInvalidVolumePath               = errors.New("volume paths must be absolute")
	ErrInvalidHealthCheckPath          = errors.New("health check path must start with /")
	ErrInvalidDocktailPort             = errors.New("docktail ports must be between 0 and 65535")
	ErrInvalidDocktailMode             = errors.New("unsupported docktail mode")
	ErrInvalidDocktailPath             = errors.New("docktail path must start with /")
	ErrInvalidDocktailProtocol         = errors.New("docktail protocol must be one of http, https, https+insecure, tcp or tls-terminated-tcp")
	ErrInvalidDocktailServiceProtocol  = errors.New("docktail service protocol must be one of http, https, tcp or tls-terminated-tcp")
	ErrInvalidDocktailProxyProtocol    = errors.New("docktail proxy protocol must be 1 or 2")
	ErrDocktailProxyProtocolWithoutTCP = errors.New("docktail proxy protocol requires a TCP service protocol")
	ErrDocktailPathOnTCP               = errors.New("docktail path is not supported for a TCP service protocol")
	ErrDocktailServiceRequired         = errors.New("a Tailscale service name is required when docktail mode is enabled")
	ErrSetupFailed                     = errors.New("setup failed")
	ErrPullFailed                      = &describedError{
		msg:         "pull failed",
		description: "Failed to download the application image. Check that the image name is correct and try again.",
	}
	ErrPullMaybeUnauthorized = &describedError{
		msg:         "image missing or authentication required",
		description: "Could not pull the image. The image may not exist, or it may require registry credentials.",
	}
	ErrDeployFailed       = errors.New("deploy failed")
	ErrVerificationFailed = &describedError{
		msg:         "verification failed",
		description: "The application couldn't be verified. Please check that you have a valid DNS record set up.",
	}
	ErrUnpauseFailed                   = errors.New("failed to unpause container after backup")
	ErrRegistryPasswordWithoutUsername = errors.New("registry password requires a username")
	ErrRegistryUsernameWithoutPassword = errors.New("registry username requires a password")
	ErrDocktailVerifyFailed            = &describedError{
		msg:         "tailscale service not advertised",
		description: "The application is running, but docktail hasn't advertised it as a Tailscale Service. Check that the docktail container is running, and that the service name matches the Service defined in your Tailscale admin console.",
	}
	ErrDocktailNotAdvertised = &describedError{
		msg:         "tailscale service not advertised yet",
		description: "The application is running, but the Tailscale Service hasn't appeared. It may still be reconciling, or it may be awaiting approval in your Tailscale admin console.",
	}
	ErrDocktailServiceNameInUse = &describedError{
		msg:         "docktail Tailscale Service name is already in use by another application",
		description: "Another application in this namespace is already advertised as this Tailscale Service via docktail. Choose a different --docktail-service (or app name), or remove the other application first.",
	}
)

const (
	AutomaticTaskInterval = 24 * time.Hour
	HealthCheckPath       = "/up"
	DefaultAppPort        = 80
	httpVerifyTimeout     = 30 * time.Second
)

// AppVolumeMountTargets defines the default paths where the app data volume is
// mounted inside the container. The first entry is the primary path used for
// backups. Applications can override these via their volume path settings.
var AppVolumeMountTargets = []string{"/storage", "/rails/storage"}

type Application struct {
	namespace    *Namespace
	Settings     ApplicationSettings
	Running      bool
	RunningSince time.Time

	// State is the container's Docker state: created, running, paused,
	// restarting, removing, exited or dead. It's empty when the container
	// isn't known.
	State string
	// RestartCount is how many times Docker has restarted the container.
	RestartCount int
	// ExitCode is the container's exit code, meaningful once it has exited.
	ExitCode int
	// Health is the container's health check status (healthy, unhealthy,
	// starting), empty when the container has no health check or its
	// health isn't known.
	Health string
}

func NewApplication(ns *Namespace, settings ApplicationSettings) *Application {
	return &Application{
		namespace: ns,
		Settings:  settings,
	}
}

func (a *Application) ContainerName(ctx context.Context) (string, error) {
	containers, err := a.namespace.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return "", err
	}

	for _, c := range containers.Items {
		if len(c.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		if a.namespace.containerAppName(name) == a.Settings.Name {
			return name, nil
		}
	}

	return "", fmt.Errorf("no container found for app %s", a.Settings.Name)
}

func (a *Application) Volume(ctx context.Context) (*ApplicationVolume, error) {
	vol, err := FindVolume(ctx, a.namespace, a.Settings.Name)
	if err == nil {
		return vol, nil
	}
	if !errors.Is(err, ErrVolumeNotFound) {
		return nil, err
	}

	return CreateVolume(ctx, a.namespace, a.Settings.Name, ApplicationLegacyVolumeSettings{})
}

func (a *Application) URL() string {
	if a.Settings.Host == "" {
		return ""
	}

	scheme := "http"
	defaultPort := 80
	if a.Settings.TLSEnabled() {
		scheme = "https"
		defaultPort = 443
	}

	base := scheme + "://" + a.Settings.Host

	if a.namespace == nil {
		return base
	}

	proxy := a.namespace.Proxy()
	if proxy.Settings == nil {
		return base
	}

	port := proxy.Settings.HTTPPort
	if a.Settings.TLSEnabled() {
		port = proxy.Settings.HTTPSPort
	}

	if port != 0 && port != defaultPort {
		return base + ":" + strconv.Itoa(port)
	}
	return base
}

func (a *Application) Stop(ctx context.Context) error {
	name, err := a.ContainerName(ctx)
	if err != nil {
		return err
	}

	_, err = a.namespace.client.ContainerStop(ctx, name, client.ContainerStopOptions{})
	return err
}

func (a *Application) Start(ctx context.Context) error {
	name, err := a.ContainerName(ctx)
	if err != nil {
		return err
	}

	_, err = a.namespace.client.ContainerStart(ctx, name, client.ContainerStartOptions{})
	return err
}

func (a *Application) Exec(ctx context.Context, cmd []string) error {
	name, err := a.ContainerName(ctx)
	if err != nil {
		return err
	}

	if err := execAttachedInContainer(ctx, a.namespace.client, name, cmd); err != nil {
		if errdefs.IsConflict(err) {
			return ErrApplicationNotRunning
		}
		return err
	}

	return nil
}

func (a *Application) Update(ctx context.Context, progress DeployProgressCallback) (bool, error) {
	changed, err := a.pullImage(ctx, progress)
	if err != nil {
		a.saveOperationResult(ctx, func(s *State) { s.RecordUpdate(a.Settings.Name, err) })
		return false, err
	}

	if !changed {
		a.saveOperationResult(ctx, func(s *State) { s.RecordUpdate(a.Settings.Name, nil) })
		return false, nil
	}

	vol, err := a.Volume(ctx)
	if err != nil {
		err = fmt.Errorf("getting volume: %w", err)
		a.saveOperationResult(ctx, func(s *State) { s.RecordUpdate(a.Settings.Name, err) })
		return false, err
	}

	err = a.deployWithVolume(ctx, vol, progress)
	a.saveOperationResult(ctx, func(s *State) { s.RecordUpdate(a.Settings.Name, err) })
	return true, err
}

func (a *Application) Deploy(ctx context.Context, progress DeployProgressCallback) error {
	if a.Settings.Host == "" {
		return ErrHostRequired
	}

	if _, err := a.pullImage(ctx, progress); err != nil {
		return err
	}

	vol, err := a.Volume(ctx)
	if err != nil {
		return fmt.Errorf("getting volume: %w", err)
	}

	return a.deployWithVolume(ctx, vol, progress)
}

// UpdateSettings redeploys the app with the given settings. Unlike Deploy,
// it never pulls, so the app is not updated even if its image tag has moved.
// On failure, the previous settings are restored.
func (a *Application) UpdateSettings(ctx context.Context, settings ApplicationSettings, progress DeployProgressCallback) error {
	vol, err := a.Volume(ctx)
	if err != nil {
		return fmt.Errorf("getting volume: %w", err)
	}

	oldSettings := a.Settings
	a.Settings = settings

	if err := a.deployWithVolume(ctx, vol, progress); err != nil {
		a.Settings = oldSettings
		return err
	}

	return nil
}

// Verify checks that the deployed application is actually reachable. Apps
// advertised as Tailscale Services are verified against Tailscale rather than
// over HTTP. It never removes the app: callers decide what a failed
// verification should mean.
func (a *Application) Verify(ctx context.Context) error {
	return a.verify(ctx)
}

func (a *Application) Remove(ctx context.Context, removeData bool) error {
	if a.usesProxy() {
		if err := a.namespace.Proxy().Remove(ctx, a.Settings.Name); err != nil {
			return fmt.Errorf("removing from proxy: %w", err)
		}
	}

	return a.Destroy(ctx, removeData)
}

// usesProxy reports whether this app is registered with kamal-proxy. Docktail
// apps are reached over the tailnet by their docktail labels, so ONCE keeps
// them out of the proxy entirely.
func (a *Application) usesProxy() bool {
	return !a.Settings.DocktailEnabled()
}

func (a *Application) Destroy(ctx context.Context, destroyVolumes bool) error {
	containers, err := a.namespace.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers.Items {
		for _, name := range c.Names {
			name = strings.TrimPrefix(name, "/")
			if a.namespace.containerAppName(name) == a.Settings.Name {
				if _, err := a.namespace.client.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
					return fmt.Errorf("removing container: %w", err)
				}
				break
			}
		}
	}

	if destroyVolumes {
		vol, err := FindVolume(ctx, a.namespace, a.Settings.Name)
		if err != nil && !errors.Is(err, ErrVolumeNotFound) {
			return fmt.Errorf("getting volume: %w", err)
		}
		if vol != nil {
			if err := vol.Destroy(ctx); err != nil {
				return err
			}
		}
	}

	return nil
}

// Private

// applyInspect copies the details of an inspected container onto the
// application: its start time, restart count, exit code and health. Values
// the inspect doesn't carry are left alone.
func (a *Application) applyInspect(inspected container.InspectResponse) {
	a.RestartCount = inspected.RestartCount

	if inspected.State == nil {
		return
	}

	if t, err := time.Parse(time.RFC3339Nano, inspected.State.StartedAt); err == nil {
		a.RunningSince = t
	}

	a.ExitCode = inspected.State.ExitCode

	if inspected.State.Health != nil {
		a.Health = string(inspected.State.Health.Status)
	}
}

func (a *Application) saveOperationResult(ctx context.Context, record func(*State)) {
	state, err := a.namespace.LoadState(ctx)
	if err != nil {
		return
	}
	record(state)
	a.namespace.SaveState(ctx, state)
}

func (a *Application) pullImage(ctx context.Context, progress DeployProgressCallback) (bool, error) {
	opts := client.ImagePullOptions{RegistryAuth: registryAuthFor(a.Settings)}
	reader, err := a.namespace.client.ImagePull(ctx, a.Settings.Image, opts)
	if err != nil {
		return false, wrapPullError(err)
	}
	defer reader.Close()

	tracker := newPullProgressTracker(progress)
	if err := tracker.Track(reader); err != nil {
		return false, wrapPullError(err)
	}

	pulledInspect, err := a.namespace.client.ImageInspect(ctx, a.Settings.Image)
	if err != nil {
		return false, fmt.Errorf("%w: inspecting image after pull: %w", ErrPullFailed, err)
	}

	return pulledInspect.ID != a.runningImageID(ctx), nil
}

func (a *Application) runningImageID(ctx context.Context) string {
	name, err := a.ContainerName(ctx)
	if err != nil {
		return ""
	}
	info, err := a.namespace.client.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
	if err != nil {
		return ""
	}
	return info.Container.Image
}

func (a *Application) deployWithVolume(ctx context.Context, vol *ApplicationVolume, progress DeployProgressCallback) error {
	if a.Settings.DocktailEnabled() {
		if service := a.Settings.EffectiveServiceName(); service != "" {
			if owner, conflict := a.namespace.docktailServiceOwner(service, a.Settings.Name); conflict {
				return fmt.Errorf("%w: %s (another application %q is already using it)", ErrDocktailServiceNameInUse, service, owner)
			}
		}
	}

	if progress != nil {
		progress(DeployProgress{Stage: DeployStageStarting})
	}

	id, err := ContainerRandomID()
	if err != nil {
		return fmt.Errorf("generating container id: %w", err)
	}

	containerName := fmt.Sprintf("%s-app-%s-%s", a.namespace.name, a.Settings.Name, id)

	if err := a.ensureKeys(vol); err != nil {
		return err
	}

	env := a.BuildEnv()

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyAlways},
		LogConfig:     ContainerLogConfig(),
		Mounts:        a.volumeMounts(vol),
	}
	hostConfig.Resources = container.Resources{
		Memory:   int64(a.Settings.Resources.MemoryMB) * 1024 * 1024,
		NanoCPUs: int64(a.Settings.Resources.CPUs) * 1e9,
	}

	// For docktail `direct=false` mode, publish the backend port on
	// loopback only so docktail can proxy through it without needing to
	// reach the container IP. The host binding never escapes the host.
	if port := a.loopbackPortBinding(); port != "" {
		portKey := network.MustParsePort(port + "/tcp")
		hostConfig.PortBindings = network.PortMap{
			portKey: []network.PortBinding{{
				HostIP:   netip.MustParseAddr("127.0.0.1"),
				HostPort: port,
			}},
		}
	}

	resp, err := a.namespace.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:       containerName,
		Config:     a.containerConfig(env),
		HostConfig: hostConfig,
		NetworkingConfig: &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				a.namespace.name: {},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	if _, err := a.namespace.client.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); err != nil {
		a.namespace.client.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
		return fmt.Errorf("starting container: %w", err)
	}

	shortContainerID := resp.ID[:12]

	if a.Settings.DocktailEnabled() {
		if err := waitForContainerHealthy(ctx, a.namespace.client, a.namespace.name, resp.ID, a.Settings.effectiveProtocol(), a.Settings.EffectiveAppPort(), a.Settings.EffectiveHealthCheckPath()); err != nil {
			a.namespace.client.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
			slog.Error("Application failed to start", "app", a.Settings.Name, "error", err)
			return ErrAppNotStarted
		}

		// Remove the proxy route only after the new container is proven
		// healthy: a failed switch would otherwise leave the previous app
		// running but unroutable on this hostname.
		if err := a.namespace.Proxy().Remove(ctx, a.Settings.Name); err != nil {
			a.namespace.client.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
			return fmt.Errorf("removing pre-existing proxy route: %w", err)
		}
	} else {
		if err := a.namespace.Proxy().Deploy(ctx, DeployOptions{
			AppName:         a.Settings.Name,
			Target:          a.Settings.DeployTarget(shortContainerID),
			Host:            a.Settings.Host,
			TLS:             a.Settings.TLSEnabled(),
			HealthCheckPath: a.Settings.HealthCheckPath,
		}); err != nil {
			a.namespace.client.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
			if strings.Contains(err.Error(), "target not healthy") || strings.Contains(err.Error(), "deploy timed out") {
				slog.Error("Application failed to start", "app", a.Settings.Name, "error", err)
				return ErrAppNotStarted
			}
			return fmt.Errorf("registering with proxy: %w", err)
		}
	}

	if err := a.removeContainersExcept(ctx, containerName); err != nil {
		return fmt.Errorf("removing old containers: %w", err)
	}

	if progress != nil {
		progress(DeployProgress{Stage: DeployStageFinished})
	}

	return nil
}

func (a *Application) ensureKeys(vol *ApplicationVolume) error {
	// Older installs may still have keys stored on the volume label
	keys := cmp.Or(a.Settings.Keys, vol.Settings.Keys)

	if keys.Empty() {
		if err := keys.Regenerate(true, true); err != nil {
			return fmt.Errorf("generating keys: %w", err)
		}
	}

	a.Settings.Keys = keys
	return nil
}

// verify checks that the deployed application is actually reachable. Apps
// exposed as Tailscale Services are verified against Tailscale rather than
// over HTTP, since docktail advertises them on the tailnet directly.
func (a *Application) verify(ctx context.Context) error {
	if a.Settings.DocktailEnabled() {
		return a.verifyDocktail(ctx)
	}

	return a.verifyHTTP(ctx)
}

func (a *Application) verifyHTTP(ctx context.Context) error {
	url := a.URL()
	if url == "" {
		return nil
	}

	client := &http.Client{Timeout: httpVerifyTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+a.Settings.EffectiveHealthCheckPath(), nil)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerificationFailed, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrVerificationFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%w: unexpected status %d from %s", ErrVerificationFailed, resp.StatusCode, url)
	}

	return nil
}

func (a *Application) removeContainersExcept(ctx context.Context, keep string) error {
	containers, err := a.namespace.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return err
	}

	for _, c := range containers.Items {
		if len(c.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		if a.namespace.containerAppName(name) == a.Settings.Name && name != keep {
			if _, err := a.namespace.client.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *Application) volumeMounts(vol *ApplicationVolume) []mount.Mount {
	var mounts []mount.Mount
	for _, target := range a.Settings.EffectiveVolumePaths() {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: vol.Name(),
			Target: target,
		})
	}
	return mounts
}

// loopbackPortBinding is the host-side port number to publish for the
// backend container port when the docktail Service runs in `direct=false`
// mode. It returns the empty string when docktail should talk directly to
// the container IP and no host binding is needed.
func (a *Application) loopbackPortBinding() string {
	if !a.Settings.DocktailEnabled() || a.Settings.effectiveDirect() {
		return ""
	}
	return strconv.Itoa(a.Settings.EffectiveTailscalePort())
}

func (a *Application) BuildEnv() []string {
	s := a.Settings

	var env []string

	if !s.SkipRailsEnv {
		env = append(env,
			"SECRET_KEY_BASE="+s.Keys.SecretKeyBase,
			"VAPID_PUBLIC_KEY="+s.Keys.VAPIDPublicKey,
			"VAPID_PRIVATE_KEY="+s.Keys.VAPIDPrivateKey,
		)

		if !s.TLSEnabled() {
			env = append(env, "DISABLE_SSL=true")
		}
	}

	if s.Resources.CPUs > 0 {
		env = append(env, "NUM_CPUS="+strconv.Itoa(s.Resources.CPUs))
	}

	if url := a.URL(); url != "" {
		env = append(env, "BASE_URL="+url)
	}

	env = append(env, s.SMTP.BuildEnv()...)

	for k, v := range s.EnvVars {
		env = append(env, k+"="+v)
	}

	return env
}

func (a *Application) containerConfig(env []string) *container.Config {
	labels := map[string]string{
		labelKey: a.Settings.Marshal(),
	}

	// Every deploy recreates the container, so docktail labels can simply be
	// merged in here.
	maps.Copy(labels, a.Settings.DocktailLabels())

	return &container.Config{
		Image:  a.Settings.Image,
		Labels: labels,
		Env:    env,
	}
}

// Helpers

// Health check timings mirror kamal-proxy's --deploy-timeout 120s and
// health-check-period 1s defaults, with a per-request timeout to fail fast on
// individual probes. These are variables so tests can shrink them.
var (
	healthCheckInterval = 1 * time.Second
	healthCheckTimeout  = 120 * time.Second
	healthCheckRequest  = 5 * time.Second
)

// healthCheckProbe performs a single HTTP health check request. It is a
// variable so tests can substitute it without opening sockets.
var healthCheckProbe = func(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	client := &http.Client{Timeout: healthCheckRequest}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// healthCheckTCPProbe opens a TCP connection to the given address. It is a
// variable so tests can substitute it without opening sockets.
var healthCheckTCPProbe = func(ctx context.Context, addr string) error {
	d := net.Dialer{Timeout: healthCheckRequest}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return conn.Close()
}

// healthCheckClient abstracts the Docker API calls needed by the health check
// path. *client.Client satisfies it; tests can substitute their own.
type healthCheckClient interface {
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
}

// waitForContainerHealthy polls the application's health check over the
// container's Docker network IP until it succeeds, mirroring the timing
// kamal-proxy uses for non-docktail apps. The check goes directly to the
// container, not through DNS or the proxy, so docktail apps don't need to be
// reachable any other way to be considered healthy. The probe is chosen by
// the backend protocol so HTTPS or TCP backends don't fail just because an
// HTTP probe can't be served.
func waitForContainerHealthy(ctx context.Context, c healthCheckClient, network, containerID, protocol string, port int, path string) error {
	ip, err := containerNetworkIP(ctx, c, network, containerID)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(healthCheckTimeout)

	for {
		if runHealthCheckProbe(ctx, protocol, ip, port, path) == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("application did not become healthy (%s) within %s", protocol, healthCheckTimeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthCheckInterval):
		}
	}
}

// runHealthCheckProbe performs a single health probe using the probe that
// matches the backend protocol so HTTPS or TCP backends don't fail just
// because an HTTP probe can't be served. HTTPS probes disable TLS
// verification because the target is a container IP on a private Docker
// network without a serving certificate: this is a liveness check, not an
// authentication check.
func runHealthCheckProbe(ctx context.Context, protocol, ip string, port int, path string) error {
	switch protocol {
	case "https", "https+insecure":
		url := fmt.Sprintf("https://%s:%d%s", ip, port, path)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		client := &http.Client{
			Timeout:   healthCheckRequest,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return nil
	case "tcp", "tls-terminated-tcp":
		return healthCheckTCPProbe(ctx, net.JoinHostPort(ip, strconv.Itoa(port)))
	default:
		status, err := healthCheckProbe(ctx, fmt.Sprintf("http://%s:%d%s", ip, port, path))
		if err != nil {
			return err
		}
		if status < 200 || status > 299 {
			return fmt.Errorf("status %d", status)
		}
		return nil
	}
}

// containerNetworkIP returns the container's IP on the given Docker network,
// or an error when the inspect fails or the endpoint is missing. Errors
// propagate as-is so callers can distinguish "container/network gone" from a
// real health check timeout.
func containerNetworkIP(ctx context.Context, c healthCheckClient, network, containerID string) (string, error) {
	info, err := c.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return "", fmt.Errorf("inspecting container: %w", err)
	}

	if info.Container.NetworkSettings == nil {
		return "", fmt.Errorf("container has no network settings yet")
	}

	endpoint := info.Container.NetworkSettings.Networks[network]
	if endpoint == nil {
		return "", fmt.Errorf("container is not attached to the %q network", network)
	}

	addr := endpoint.IPAddress
	if !addr.IsValid() {
		return "", fmt.Errorf("container has no IP address on the %q network", network)
	}

	return addr.String(), nil
}
