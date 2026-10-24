package docker

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/moby/moby/client"
)

// DoctorProbesFor returns the probes that inspect the real Docker host and
// the local tailscale CLI.
func DoctorProbesFor(ns *Namespace) DoctorProbes {
	return DoctorProbes{
		AppContainer:    ns.doctorAppContainer,
		Containers:      ns.doctorContainers,
		HTTPGet:         doctorHTTPProbe,
		TCPDial:         doctorTCPProbe,
		ServeStatus:     tailscaleServeStatusOutput,
		TailscaleStatus: tailscaleStatusOutput,
		ListeningPorts:  ns.doctorListeningPorts,
	}
}

// Private

// doctorAppContainer finds the container backing an app and reports its
// state, network IP and labels.
func (n *Namespace) doctorAppContainer(ctx context.Context, appName string) (DoctorContainer, error) {
	containers, err := n.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return DoctorContainer{}, err
	}

	for _, c := range containers.Items {
		for _, name := range c.Names {
			name = strings.TrimPrefix(name, "/")
			if n.containerAppName(name) != appName {
				continue
			}

			container := DoctorContainer{Name: name, State: string(c.State), Labels: c.Labels}

			if ip, err := containerNetworkIP(ctx, n.client, n.name, c.ID); err == nil {
				container.IP = ip
			}

			return container, nil
		}
	}

	return DoctorContainer{}, fmt.Errorf("no container found for app %s", appName)
}

// doctorContainers lists every container on the Docker host.
func (n *Namespace) doctorContainers(ctx context.Context) ([]DoctorContainer, error) {
	containers, err := n.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}

	list := make([]DoctorContainer, 0, len(containers.Items))
	for _, c := range containers.Items {
		container := DoctorContainer{State: string(c.State), Labels: c.Labels}
		if len(c.Names) > 0 {
			container.Name = strings.TrimPrefix(c.Names[0], "/")
		}
		list = append(list, container)
	}

	return list, nil
}

// doctorListeningPorts returns the TCP ports the container listens on, read
// from the container's own network tables.
func (n *Namespace) doctorListeningPorts(ctx context.Context, containerName string) ([]int, error) {
	result, err := execInContainer(ctx, n.client, containerName, []string{"cat", "/proc/net/tcp"})
	if err != nil {
		return nil, err
	}

	return parseListeningPorts(result.Stdout)
}

// Helpers

// parseListeningPorts extracts the listening ports from the contents of
// /proc/net/tcp.
func parseListeningPorts(output string) ([]int, error) {
	var ports []int

	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		address, state := fields[1], fields[3]
		if state != "0A" {
			continue
		}

		_, port, ok := strings.Cut(address, ":")
		if !ok {
			continue
		}

		number, err := strconv.ParseInt(port, 16, 32)
		if err != nil {
			continue
		}

		ports = append(ports, int(number))
	}

	return ports, nil
}

// tailscaleStatusOutput returns the raw `tailscale status --json` output.
func tailscaleStatusOutput(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, tailscaleCommandTimeout)
	defer cancel()

	return exec.CommandContext(ctx, tailscaleCommand, "status", "--json").Output()
}

// doctorHTTPProbe performs an HTTP GET and reports the response status code.
func doctorHTTPProbe(ctx context.Context, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	client := &http.Client{Timeout: doctorProbeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// doctorTCPProbe opens a TCP connection.
func doctorTCPProbe(ctx context.Context, address string) error {
	d := net.Dialer{Timeout: doctorDialTimeout}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	return conn.Close()
}
