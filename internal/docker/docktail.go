package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

const tailscaleCommandTimeout = 10 * time.Second

// tailscaleCommand is a variable so tests can substitute a stub.
var tailscaleCommand = "tailscale"

// verifyDocktail checks that docktail has advertised this app as a Tailscale
// Service by inspecting the host's serve configuration. Reading serve status
// needs no root, which is what makes this check possible from here.
func (a *Application) verifyDocktail(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, tailscaleCommandTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, tailscaleCommand, "serve", "status", "--json").Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return ErrTailscaleCLIMissing
		}
		return fmt.Errorf("%w: running %s serve status: %w", ErrDocktailVerifyFailed, tailscaleCommand, err)
	}

	return parseTailscaleServeStatus(out, a.Settings.EffectiveServiceName(), a.Settings.EffectiveTailscalePort())
}

// DocktailWarnings reports host-level problems that would stop this app from
// being advertised as a Tailscale Service. They are advisory: docktail keeps
// reconciling, so an install that starts before docktail is ready can still
// succeed, so these are surfaced as warnings rather than failures.
func (a *Application) DocktailWarnings(ctx context.Context) []string {
	if !a.Settings.DocktailEnabled() || a.namespace == nil {
		return nil
	}

	var warnings []string

	if running, err := a.namespace.runningDocktailContainer(ctx); err == nil && !running {
		warnings = append(warnings, "no running docktail container found on this Docker host, so this app won't be advertised as a Tailscale Service until one is started")
	}

	if tags, err := tailscaleSelfTags(ctx); err == nil && len(tags) == 0 {
		warnings = append(warnings, "the Tailscale node has no tags, and docktail needs a tagged node (for example tag:server) to create Tailscale Services")
	}

	return warnings
}

// Private

func (n *Namespace) runningDocktailContainer(ctx context.Context) (bool, error) {
	containers, err := n.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return false, err
	}

	for _, c := range containers.Items {
		if c.State == "running" && imageIsDocktail(c.Image) {
			return true, nil
		}
	}

	return false, nil
}

func tailscaleSelfTags(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, tailscaleCommandTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, tailscaleCommand, "status", "--json").Output()
	if err != nil {
		return nil, err
	}

	var status struct {
		Self struct {
			Tags []string `json:"Tags"`
		} `json:"Self"`
	}
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, err
	}

	return status.Self.Tags, nil
}

// tailscaleServeStatus mirrors the shape of `tailscale serve status --json`,
// which nests each Service's web handlers beneath its tailnet hostname.
type tailscaleServeStatus struct {
	Services map[string]tailscaleServiceStatus `json:"Services"`
}

type tailscaleServiceStatus struct {
	Web map[string]tailscaleWebConfig `json:"Web"`
}

type tailscaleWebConfig struct {
	Handlers map[string]tailscaleHandler `json:"Handlers"`
}

type tailscaleHandler struct {
	Proxy string `json:"Proxy"`
}

func parseTailscaleServeStatus(data []byte, serviceName string, port int) error {
	service := "svc:" + serviceName

	var status tailscaleServeStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return fmt.Errorf("%w: parsing tailscale serve status: %w", ErrDocktailVerifyFailed, err)
	}

	target := ":" + strconv.Itoa(port)

	for name, advertised := range status.Services {
		if normalizeServiceName(name) != normalizeServiceName(service) {
			continue
		}

		for _, web := range advertised.Web {
			for _, handler := range web.Handlers {
				if strings.Contains(handler.Proxy, target) {
					return nil
				}
			}
		}

		return fmt.Errorf("%w: service %q is advertised, but has no endpoint on port %d", ErrDocktailVerifyFailed, service, port)
	}

	return fmt.Errorf("%w: no Tailscale Service named %q yet; check that the service name matches the one defined in your Tailscale admin console", ErrDocktailVerifyFailed, service)
}

// Helpers

func normalizeServiceName(name string) string {
	return strings.ToLower(strings.TrimPrefix(name, "svc:"))
}

func imageIsDocktail(image string) bool {
	return strings.Contains(strings.ToLower(image), "docktail")
}
