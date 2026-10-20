package docker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveStatusWithBudgetService was captured from a host running docktail with
// a service named "budget" proxying to container port 5006.
const serveStatusWithBudgetService = `{
  "Services": {
    "svc:budget": {
      "TCP": {
        "443": {
          "HTTPS": true
        }
      },
      "Web": {
        "budget.lemur-carat.ts.net:443": {
          "Handlers": {
            "/": {
              "Proxy": "http://172.21.0.2:5006"
            }
          }
        }
      }
    }
  }
}`

func TestParseTailscaleServeStatus(t *testing.T) {
	statusForProxy := func(proxy string) string {
		return `{"Services":{"svc:budget":{"Web":{"host.ts.net:443":{"Handlers":{"/":{"Proxy":"` + proxy + `"}}}}}}}`
	}

	tests := []struct {
		name         string
		data         string
		service      string
		port         int
		expectedHost string
		wantErr      bool
	}{
		{
			name:         "matching service and port",
			data:         serveStatusWithBudgetService,
			service:      "budget",
			port:         5006,
			expectedHost: "172.21.0.2",
		},
		{
			name:         "service name is matched case insensitively",
			data:         serveStatusWithBudgetService,
			service:      "Budget",
			port:         5006,
			expectedHost: "172.21.0.2",
		},
		{
			name:         "no services advertised",
			data:         `{"Services":{}}`,
			service:      "budget",
			port:         5006,
			expectedHost: "172.21.0.2",
			wantErr:      true,
		},
		{
			name:         "different service name",
			data:         serveStatusWithBudgetService,
			service:      "other",
			port:         5006,
			expectedHost: "172.21.0.2",
			wantErr:      true,
		},
		{
			name:         "service advertised on another port",
			data:         serveStatusWithBudgetService,
			service:      "budget",
			port:         80,
			expectedHost: "172.21.0.2",
			wantErr:      true,
		},
		{
			name:         "port 8080 does not match port 80",
			data:         statusForProxy("http://172.21.0.2:8080"),
			service:      "budget",
			port:         80,
			expectedHost: "172.21.0.2",
			wantErr:      true,
		},
		{
			name:         "port 50061 does not match port 5006",
			data:         statusForProxy("http://172.21.0.2:50061"),
			service:      "budget",
			port:         5006,
			expectedHost: "172.21.0.2",
			wantErr:      true,
		},
		{
			name:         "explicit port matches exactly",
			data:         statusForProxy("http://172.21.0.2:5006"),
			service:      "budget",
			port:         5006,
			expectedHost: "172.21.0.2",
		},
		{
			name:         "portless http proxy matches port 80",
			data:         statusForProxy("http://172.21.0.2"),
			service:      "budget",
			port:         80,
			expectedHost: "172.21.0.2",
		},
		{
			name:         "portless https proxy matches port 443",
			data:         statusForProxy("https://172.21.0.2"),
			service:      "budget",
			port:         443,
			expectedHost: "172.21.0.2",
		},
		{
			name:         "portless http proxy does not match port 443",
			data:         statusForProxy("http://172.21.0.2"),
			service:      "budget",
			port:         443,
			expectedHost: "172.21.0.2",
			wantErr:      true,
		},
		{
			name:         "invalid json",
			data:         `not json`,
			service:      "budget",
			port:         5006,
			expectedHost: "172.21.0.2",
			wantErr:      true,
		},
		{
			name:         "same name and port but a different host does not verify",
			data:         statusForProxy("http://172.21.0.2:5006"),
			service:      "budget",
			port:         5006,
			expectedHost: "172.21.0.99",
			wantErr:      true,
		},
		{
			name:         "empty expected host skips the host check",
			data:         statusForProxy("http://172.21.0.2:5006"),
			service:      "budget",
			port:         5006,
			expectedHost: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseTailscaleServeStatus([]byte(tt.data), tt.service, tt.port, tt.expectedHost)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrDocktailVerifyFailed)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestVerifyDocktail(t *testing.T) {
	// Polling is exercised by the "appears on a later poll" case; the rest
	// should give up immediately.
	originalWindow, originalInterval := docktailVerifyWindow, docktailVerifyInterval
	docktailVerifyWindow, docktailVerifyInterval = 0, time.Millisecond
	t.Cleanup(func() {
		docktailVerifyWindow, docktailVerifyInterval = originalWindow, originalInterval
	})

	stubTailscale := func(t *testing.T, output string) {
		t.Helper()

		path := filepath.Join(t.TempDir(), "tailscale")
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho '"+output+"'\n"), 0o755))

		original := tailscaleCommand
		tailscaleCommand = path
		t.Cleanup(func() { tailscaleCommand = original })
	}

	stubTailscaleSequence := func(t *testing.T, outputs ...string) {
		t.Helper()

		dir := t.TempDir()
		counter := filepath.Join(dir, "calls")

		var script strings.Builder
		script.WriteString("#!/bin/sh\n")
		script.WriteString("n=$(cat " + counter + " 2>/dev/null || echo 0)\n")
		script.WriteString("echo $((n+1)) > " + counter + "\n")
		script.WriteString("case $n in\n")
		for i, output := range outputs {
			fmt.Fprintf(&script, "%d) echo '%s' ;;\n", i, output)
		}
		script.WriteString("*) echo '{\"Services\":{}}' ;;\n")
		script.WriteString("esac\n")

		path := filepath.Join(dir, "tailscale")
		require.NoError(t, os.WriteFile(path, []byte(script.String()), 0o755))

		original := tailscaleCommand
		tailscaleCommand = path
		t.Cleanup(func() { tailscaleCommand = original })
	}

	newDocktailApp := func() *Application {
		return &Application{Settings: ApplicationSettings{
			Name:      "budget",
			Host:      "127.0.0.1:1",
			AppPort:   5006,
			Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode},
		}}
	}

	t.Run("advertised on the app port", func(t *testing.T) {
		stubTailscale(t, serveStatusWithBudgetService)

		require.NoError(t, newDocktailApp().verifyDocktail(context.Background()))
	})

	t.Run("service absent fails with the not-advertised sentinel", func(t *testing.T) {
		stubTailscale(t, `{"Services":{}}`)

		assert.ErrorIs(t, newDocktailApp().verifyDocktail(context.Background()), ErrDocktailNotAdvertised)
	})

	t.Run("target is the wrong port fails with the not-advertised sentinel", func(t *testing.T) {
		stubTailscale(t, `{"Services":{"svc:budget":{"Web":{"host.ts.net:443":{"Handlers":{"/":{"Proxy":"http://172.21.0.2:80"}}}}}}}`)

		assert.ErrorIs(t, newDocktailApp().verifyDocktail(context.Background()), ErrDocktailNotAdvertised)
	})

	t.Run("unparsable serve status warns instead of failing", func(t *testing.T) {
		stubTailscale(t, `not json`)

		require.NoError(t, newDocktailApp().verifyDocktail(context.Background()))
	})

	t.Run("missing tailscale cli warns instead of failing", func(t *testing.T) {
		original := tailscaleCommand
		tailscaleCommand = filepath.Join(t.TempDir(), "missing-tailscale")
		t.Cleanup(func() { tailscaleCommand = original })

		require.NoError(t, newDocktailApp().verifyDocktail(context.Background()))
	})

	t.Run("service appears on a later poll", func(t *testing.T) {
		docktailVerifyWindow = time.Second
		docktailVerifyInterval = time.Millisecond
		t.Cleanup(func() { docktailVerifyWindow = 0 })

		stubTailscaleSequence(t, `{"Services":{}}`, serveStatusWithBudgetService)

		require.NoError(t, newDocktailApp().verifyDocktail(context.Background()))
	})

	t.Run("verify uses the tailscale path for docktail apps", func(t *testing.T) {
		stubTailscale(t, `{"Services":{}}`)

		// The host is unreachable over HTTP, so a fall back to HTTP
		// verification would surface as ErrVerificationFailed.
		assert.ErrorIs(t, newDocktailApp().Verify(context.Background()), ErrDocktailNotAdvertised)
	})

	t.Run("cancelled context is reported as not verified", func(t *testing.T) {
		// Keep the loop alive long enough to exercise the ctx.Done()
		// branch rather than the deadline-exhausted branch, so this test
		// pins the behaviour of reporting cancellation as not verified.
		docktailVerifyWindow = time.Hour
		docktailVerifyInterval = time.Hour
		t.Cleanup(func() { docktailVerifyWindow = 0 })

		stubTailscale(t, `{"Services":{}}`)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := newDocktailApp().verifyDocktail(ctx)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestDocktailWarningsNoneWhenDisabled(t *testing.T) {
	app := &Application{Settings: ApplicationSettings{Name: "budget"}}

	assert.Nil(t, app.DocktailWarnings(context.Background()))
}

func TestImageIsDocktail(t *testing.T) {
	assert.True(t, imageIsDocktail("ghcr.io/marvinvr/docktail:latest"))
	assert.True(t, imageIsDocktail("docktail"))
	assert.False(t, imageIsDocktail("ghcr.io/basecamp/once-campfire:latest"))
}
