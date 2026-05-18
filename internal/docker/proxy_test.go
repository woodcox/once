package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		args, err := proxy.deployArgs(DeployOptions{AppName: "chat", Target: "localhost:3000"})

		assert.NoError(t, err)
		assert.Equal(t, []string{
			"kamal-proxy", "deploy", "chat",
			"--target", "localhost:3000",
			"--deploy-timeout", "120s",
		}, args)
	})

	t.Run("with host", func(t *testing.T) {
		args, err := proxy.deployArgs(DeployOptions{AppName: "chat", Target: "localhost:3000", Host: "chat.example.com"})

		assert.NoError(t, err)
		assert.Contains(t, args, "--host")
		assert.Contains(t, args, "chat.example.com")
	})

	t.Run("with TLS", func(t *testing.T) {
		args, err := proxy.deployArgs(DeployOptions{AppName: "chat", Target: "localhost:3000", TLS: true})

		assert.NoError(t, err)
		assert.Contains(t, args, "--tls")
	})

	t.Run("with TLS custom certificate paths", func(t *testing.T) {
		args, err := proxy.deployArgs(DeployOptions{
			AppName:     "chat",
			Target:      "localhost:3000",
			TLS:         true,
			TLSCertPath: "/home/pi/ts.crt",
			TLSKeyPath:  "/home/pi/ts.key",
		})

		assert.NoError(t, err)
		assert.Equal(t, []string{
			"kamal-proxy", "deploy", "chat",
			"--target", "localhost:3000",
			"--deploy-timeout", "120s",
			"--tls",
			"--tls-certificate-path", "/home/pi/ts.crt",
			"--tls-private-key-path", "/home/pi/ts.key",
		}, args)
	})

	t.Run("with host and TLS", func(t *testing.T) {
		args, err := proxy.deployArgs(DeployOptions{
			AppName: "chat",
			Target:  "localhost:3000",
			Host:    "chat.example.com",
			TLS:     true,
		})

		assert.NoError(t, err)
		assert.Equal(t, []string{
			"kamal-proxy", "deploy", "chat",
			"--target", "localhost:3000",
			"--deploy-timeout", "120s",
			"--host", "chat.example.com",
			"--tls",
		}, args)
	})

	t.Run("with TLS cert path but no key path", func(t *testing.T) {
		_, err := proxy.deployArgs(DeployOptions{
			AppName:     "chat",
			Target:      "localhost:3000",
			TLS:         true,
			TLSCertPath: "/home/pi/ts.crt",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "both TLS certificate path and TLS key path must be provided together")
	})

	t.Run("with TLS key path but no cert path", func(t *testing.T) {
		_, err := proxy.deployArgs(DeployOptions{
			AppName:    "chat",
			Target:     "localhost:3000",
			TLS:        true,
			TLSKeyPath: "/home/pi/ts.key",
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "both TLS certificate path and TLS key path must be provided together")
	})
}
