package docker

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyContainerName(t *testing.T) {
	ns := &Namespace{name: "once"}
	proxy := NewProxy(ns)
	assert.Equal(t, "once-proxy", proxy.containerName())

	ns2 := &Namespace{name: "staging"}
	proxy2 := NewProxy(ns2)
	assert.Equal(t, "staging-proxy", proxy2.containerName())
}

func TestDeployArgs(t *testing.T) {
	proxy := &Proxy{}

	t.Run("basic deploy includes timeout", func(t *testing.T) {
		args := proxy.deployArgs(DeployOptions{AppName: "chat", Target: "localhost:3000"})

		assert.Equal(t, []string{
			"kamal-proxy", "deploy", "chat",
			"--target", "localhost:3000",
			"--deploy-timeout", "120s",
		}, args)
	})

	t.Run("with host", func(t *testing.T) {
		args := proxy.deployArgs(DeployOptions{AppName: "chat", Target: "localhost:3000", Host: "chat.example.com"})

		assert.Contains(t, args, "--host")
		assert.Contains(t, args, "chat.example.com")
	})

	t.Run("with TLS", func(t *testing.T) {
		args := proxy.deployArgs(DeployOptions{AppName: "chat", Target: "localhost:3000", TLS: true})

		assert.Contains(t, args, "--tls")
	})

	t.Run("with host and TLS", func(t *testing.T) {
		args := proxy.deployArgs(DeployOptions{
			AppName: "chat",
			Target:  "localhost:3000",
			Host:    "chat.example.com",
			TLS:     true,
		})

		assert.Equal(t, []string{
			"kamal-proxy", "deploy", "chat",
			"--target", "localhost:3000",
			"--deploy-timeout", "120s",
			"--host", "chat.example.com",
			"--tls",
		}, args)
	})
}

func TestRemoveProxyServiceTreatsMissingServiceAsSuccess(t *testing.T) {
	notFound := errors.New("exec failed with exit code 1: error: service not found")

	t.Run("never registered", func(t *testing.T) {
		exec := func(context.Context, []string) error { return notFound }
		assert.NoError(t, removeProxyService(context.Background(), "budget", exec))
	})

	t.Run("registered", func(t *testing.T) {
		exec := func(context.Context, []string) error { return nil }
		assert.NoError(t, removeProxyService(context.Background(), "budget", exec))
	})

	t.Run("other failures still surface", func(t *testing.T) {
		other := errors.New("exec failed with exit code 1: error: connection refused")
		exec := func(context.Context, []string) error { return other }
		assert.ErrorIs(t, removeProxyService(context.Background(), "budget", exec), other)
	})

	t.Run("passes the service name", func(t *testing.T) {
		var got []string
		exec := func(_ context.Context, cmd []string) error { got = cmd; return nil }
		require.NoError(t, removeProxyService(context.Background(), "budget", exec))
		assert.Equal(t, []string{"kamal-proxy", "remove", "budget"}, got)
	})
}
