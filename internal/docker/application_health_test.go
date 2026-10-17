package docker

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHealthClient is a minimal stand-in for *client.Client used by the
// health check path. It only implements the one method the helpers call.
type fakeHealthClient struct {
	info container.InspectResponse
	err  error
}

func (f *fakeHealthClient) ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{Container: f.info}, f.err
}

// withHealthCheckTimings shrinks the health-check timings so individual tests
// don't take minutes. Restore via the returned cleanup.
func withHealthCheckTimings(t *testing.T, interval, timeout time.Duration) {
	t.Helper()
	origInterval, origTimeout := healthCheckInterval, healthCheckTimeout
	healthCheckInterval, healthCheckTimeout = interval, timeout
	t.Cleanup(func() {
		healthCheckInterval, healthCheckTimeout = origInterval, origTimeout
	})
}

// stubHealthCheckProbe replaces healthCheckProbe for the duration of the test,
// returning the configured status/error in order.
func stubHealthCheckProbe(t *testing.T, statuses []int, errs []error) {
	t.Helper()
	orig := healthCheckProbe
	healthCheckProbe = func(ctx context.Context, url string) (int, error) {
		var status int
		if len(statuses) > 0 {
			status = statuses[0]
			statuses = statuses[1:]
		}
		var err error
		if len(errs) > 0 {
			err = errs[0]
			errs = errs[1:]
		}
		return status, err
	}
	t.Cleanup(func() { healthCheckProbe = orig })
}

func containerWithNetworkIP(ip string) container.InspectResponse {
	return container.InspectResponse{
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				"once": {IPAddress: netip.MustParseAddr(ip)},
			},
		},
	}
}

func TestWaitForContainerHealthy_SuccessFirstAttempt(t *testing.T) {
	withHealthCheckTimings(t, time.Millisecond, 5*time.Second)
	stubHealthCheckProbe(t, []int{200}, nil)

	err := waitForContainerHealthy(context.Background(), &fakeHealthClient{info: containerWithNetworkIP("172.20.0.5")}, "once", "container", 80, "/up")
	require.NoError(t, err)
}

func TestWaitForContainerHealthy_SuccessAfterFailures(t *testing.T) {
	withHealthCheckTimings(t, time.Millisecond, 5*time.Second)
	stubHealthCheckProbe(t, []int{500, 502, 200}, nil)

	err := waitForContainerHealthy(context.Background(), &fakeHealthClient{info: containerWithNetworkIP("172.20.0.5")}, "once", "container", 5006, "/healthz")
	require.NoError(t, err)
}

func TestWaitForContainerHealthy_Treats4xxAsUnhealthy(t *testing.T) {
	withHealthCheckTimings(t, time.Millisecond, 5*time.Second)
	stubHealthCheckProbe(t, []int{404, 404, 200}, nil)

	err := waitForContainerHealthy(context.Background(), &fakeHealthClient{info: containerWithNetworkIP("172.20.0.5")}, "once", "container", 80, "/up")
	require.NoError(t, err)
}

func TestWaitForContainerHealthy_FailureAfterWindow(t *testing.T) {
	// The deploy caller maps any wait error to ErrAppNotStarted so the
	// existing rollback path runs; the helper itself signals the timeout
	// with a descriptive error.
	withHealthCheckTimings(t, time.Millisecond, 20*time.Millisecond)
	stubHealthCheckProbe(t, nil, []error{errors.New("connection refused")})

	err := waitForContainerHealthy(context.Background(), &fakeHealthClient{info: containerWithNetworkIP("172.20.0.5")}, "once", "container", 80, "/up")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not respond with 2xx")
}

func TestWaitForContainerHealthy_ContextCancellation(t *testing.T) {
	withHealthCheckTimings(t, time.Millisecond, 5*time.Second)

	stubHealthCheckProbe(t, []int{500}, []error{nil})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	err := waitForContainerHealthy(ctx, &fakeHealthClient{info: containerWithNetworkIP("172.20.0.5")}, "once", "container", 80, "/up")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestWaitForContainerHealthy_NoIP(t *testing.T) {
	withHealthCheckTimings(t, time.Millisecond, 5*time.Second)

	t.Run("inspect fails", func(t *testing.T) {
		err := waitForContainerHealthy(context.Background(), &fakeHealthClient{err: errors.New("no such container")}, "once", "container", 80, "/up")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "inspecting container")
	})

	t.Run("not attached to network", func(t *testing.T) {
		err := waitForContainerHealthy(context.Background(), &fakeHealthClient{info: container.InspectResponse{
			NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{}},
		}}, "once", "container", 80, "/up")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not attached to the \"once\" network")
	})

	t.Run("endpoint has no IP", func(t *testing.T) {
		err := waitForContainerHealthy(context.Background(), &fakeHealthClient{info: container.InspectResponse{
			NetworkSettings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"once": {},
				},
			},
		}}, "once", "container", 80, "/up")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no IP address on the \"once\" network")
	})
}

func TestWaitForContainerHealthy_URLConstruction(t *testing.T) {
	withHealthCheckTimings(t, time.Millisecond, 5*time.Second)

	var captured string
	orig := healthCheckProbe
	healthCheckProbe = func(ctx context.Context, url string) (int, error) {
		captured = url
		return 200, nil
	}
	t.Cleanup(func() { healthCheckProbe = orig })

	t.Run("default port and path", func(t *testing.T) {
		captured = ""
		require.NoError(t, waitForContainerHealthy(context.Background(), &fakeHealthClient{info: containerWithNetworkIP("172.20.0.5")}, "once", "c", 80, "/up"))
		assert.Equal(t, "http://172.20.0.5:80/up", captured)
	})

	t.Run("custom port and path", func(t *testing.T) {
		captured = ""
		require.NoError(t, waitForContainerHealthy(context.Background(), &fakeHealthClient{info: containerWithNetworkIP("172.20.0.7")}, "once", "c", 5006, "/healthz"))
		assert.Equal(t, "http://172.20.0.7:5006/healthz", captured)
	})
}
