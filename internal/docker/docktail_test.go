package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
	tests := []struct {
		name    string
		data    string
		service string
		port    int
		wantErr bool
	}{
		{
			name:    "matching service and port",
			data:    serveStatusWithBudgetService,
			service: "budget",
			port:    5006,
		},
		{
			name:    "service name is matched case insensitively",
			data:    serveStatusWithBudgetService,
			service: "Budget",
			port:    5006,
		},
		{
			name:    "no services advertised",
			data:    `{"Services":{}}`,
			service: "budget",
			port:    5006,
			wantErr: true,
		},
		{
			name:    "different service name",
			data:    serveStatusWithBudgetService,
			service: "other",
			port:    5006,
			wantErr: true,
		},
		{
			name:    "service advertised on another port",
			data:    serveStatusWithBudgetService,
			service: "budget",
			port:    80,
			wantErr: true,
		},
		{
			name:    "invalid json",
			data:    `not json`,
			service: "budget",
			port:    5006,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseTailscaleServeStatus([]byte(tt.data), tt.service, tt.port)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrDocktailVerifyFailed)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestVerifyDocktail(t *testing.T) {
	stubTailscale := func(t *testing.T, output string) {
		t.Helper()

		path := filepath.Join(t.TempDir(), "tailscale")
		require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\necho '"+output+"'\n"), 0o755))

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

	t.Run("service absent", func(t *testing.T) {
		stubTailscale(t, `{"Services":{}}`)

		err := newDocktailApp().verifyDocktail(context.Background())
		assert.ErrorIs(t, err, ErrDocktailVerifyFailed)
		assert.Contains(t, err.Error(), "svc:budget")
	})

	t.Run("target is the wrong port", func(t *testing.T) {
		stubTailscale(t, `{"Services":{"svc:budget":{"Web":{"host.ts.net:443":{"Handlers":{"/":{"Proxy":"http://172.21.0.2:80"}}}}}}}`)

		err := newDocktailApp().verifyDocktail(context.Background())
		assert.ErrorIs(t, err, ErrDocktailVerifyFailed)
		assert.Contains(t, err.Error(), "port 5006")
	})

	t.Run("tailscale cli missing", func(t *testing.T) {
		original := tailscaleCommand
		tailscaleCommand = filepath.Join(t.TempDir(), "missing-tailscale")
		t.Cleanup(func() { tailscaleCommand = original })

		err := newDocktailApp().verifyDocktail(context.Background())
		assert.ErrorIs(t, err, ErrTailscaleCLIMissing)
	})

	t.Run("verify uses the tailscale path for docktail apps", func(t *testing.T) {
		stubTailscale(t, `{"Services":{}}`)

		// The host is unreachable over HTTP, so a failure to fall back to
		// HTTP verification would surface as ErrVerificationFailed.
		err := newDocktailApp().verify(context.Background())
		assert.ErrorIs(t, err, ErrDocktailVerifyFailed)
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
