package command

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestDecideDocktailNotAdvertised(t *testing.T) {
	failingVerify := func(t *testing.T) func(context.Context) error {
		t.Helper()
		return func(context.Context) error {
			t.Fatal("verification should not have been attempted")
			return nil
		}
	}

	t.Run("keeps the app on end of input", func(t *testing.T) {
		var out strings.Builder

		err := decideDocktailNotAdvertised(context.Background(), failingVerify(t), strings.NewReader(""), &out)

		require.NoError(t, err)
		assert.Contains(t, out.String(), "Keeping the app.")
	})

	t.Run("keeps the app on unparseable input", func(t *testing.T) {
		var out strings.Builder

		err := decideDocktailNotAdvertised(context.Background(), failingVerify(t), strings.NewReader("banana\n"), &out)

		require.NoError(t, err)
		assert.Contains(t, out.String(), "Keeping the app.")
	})

	t.Run("keeps the app on enter", func(t *testing.T) {
		var out strings.Builder

		err := decideDocktailNotAdvertised(context.Background(), failingVerify(t), strings.NewReader("\n"), &out)

		require.NoError(t, err)
		assert.Contains(t, out.String(), "Keeping the app.")
	})

	t.Run("retry keeps the app once the Service appears", func(t *testing.T) {
		var out strings.Builder
		calls := 0
		verify := func(context.Context) error {
			calls++
			if calls == 1 {
				return docker.ErrDocktailNotAdvertised
			}
			return nil
		}

		err := decideDocktailNotAdvertised(context.Background(), verify, strings.NewReader("r\nr\n"), &out)

		require.NoError(t, err)
		assert.Equal(t, 2, calls)
		assert.Contains(t, out.String(), "now advertised")
	})

	t.Run("retry re-prompts while still not advertised", func(t *testing.T) {
		var out strings.Builder
		verify := func(context.Context) error { return docker.ErrDocktailNotAdvertised }

		err := decideDocktailNotAdvertised(context.Background(), verify, strings.NewReader("r\nr\nk\n"), &out)

		require.NoError(t, err)
		assert.Equal(t, 3, strings.Count(out.String(), "[r/k/d]"))
	})

	t.Run("retry reports an unrelated verification failure", func(t *testing.T) {
		var out strings.Builder
		failure := fmt.Errorf("%w: connection refused", docker.ErrVerificationFailed)
		verify := func(context.Context) error { return failure }

		err := decideDocktailNotAdvertised(context.Background(), verify, strings.NewReader("r\n"), &out)

		assert.ErrorIs(t, err, docker.ErrVerificationFailed)
	})

	t.Run("delete asks the caller to roll the app back", func(t *testing.T) {
		var out strings.Builder

		err := decideDocktailNotAdvertised(context.Background(), failingVerify(t), strings.NewReader("d\n"), &out)

		// The caller removes the app and its data on this sentinel, then
		// reports success rather than a failure.
		require.ErrorIs(t, err, errDocktailDelete)
	})

	t.Run("cancellation is reported rather than treated as success", func(t *testing.T) {
		var out strings.Builder

		verify := func(context.Context) error { return context.Canceled }

		err := decideDocktailNotAdvertised(context.Background(), verify, strings.NewReader("r\n"), &out)

		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestParseEnvVars(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		f := &settingsFlags{}
		result, err := f.parseEnvVars()
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("valid pairs", func(t *testing.T) {
		f := &settingsFlags{env: []string{"FOO=bar", "BAZ=qux"}}
		result, err := f.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"FOO": "bar", "BAZ": "qux"}, result)
	})

	t.Run("value containing equals", func(t *testing.T) {
		f := &settingsFlags{env: []string{"DSN=postgres://host?opt=val"}}
		result, err := f.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, "postgres://host?opt=val", result["DSN"])
	})

	t.Run("missing equals", func(t *testing.T) {
		f := &settingsFlags{env: []string{"INVALID"}}
		_, err := f.parseEnvVars()
		assert.ErrorContains(t, err, "must be in KEY=VALUE format")
	})

	t.Run("empty key", func(t *testing.T) {
		f := &settingsFlags{env: []string{"=value"}}
		_, err := f.parseEnvVars()
		assert.ErrorContains(t, err, "key must not be empty")
	})

	t.Run("empty value is valid", func(t *testing.T) {
		f := &settingsFlags{env: []string{"KEY="}}
		result, err := f.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, "", result["KEY"])
	})

	t.Run("duplicate keys last wins", func(t *testing.T) {
		f := &settingsFlags{env: []string{"KEY=first", "KEY=second"}}
		result, err := f.parseEnvVars()
		require.NoError(t, err)
		assert.Equal(t, "second", result["KEY"])
	})
}

func TestBuildSettingsImageRequired(t *testing.T) {
	f := &settingsFlags{}
	_, err := f.buildSettings(&cobra.Command{}, "", "app.example.com")
	assert.ErrorIs(t, err, docker.ErrImageRequired)
}

func TestBuildSettingsAutoBackupRequiresPath(t *testing.T) {
	t.Run("auto-backup without path", func(t *testing.T) {
		f := &settingsFlags{autoBackup: true}
		_, err := f.buildSettings(&cobra.Command{}, "image:latest", "app.example.com")
		assert.ErrorIs(t, err, docker.ErrAutoBackupWithoutPath)
	})

	t.Run("auto-backup with path", func(t *testing.T) {
		f := &settingsFlags{autoBackup: true, backupPath: "/backups"}
		_, err := f.buildSettings(&cobra.Command{}, "image:latest", "app.example.com")
		require.NoError(t, err)
	})
}

func TestBuildSettingsRegistry(t *testing.T) {
	newCmd := func() (*cobra.Command, *settingsFlags) {
		cmd := &cobra.Command{}
		f := &settingsFlags{}
		f.register(cmd)
		return cmd, f
	}

	t.Run("no registry flags leaves registry empty", func(t *testing.T) {
		cmd, f := newCmd()
		s, err := f.buildSettings(cmd, "image:latest", "app.example.com")
		require.NoError(t, err)
		assert.True(t, s.Registry.Empty())
	})

	t.Run("username and password set the scoped credentials", func(t *testing.T) {
		cmd, f := newCmd()
		require.NoError(t, cmd.Flags().Set("registry-username", "user"))
		require.NoError(t, cmd.Flags().Set("registry-password", "pass"))

		s, err := f.buildSettings(cmd, "image:latest", "app.example.com")
		require.NoError(t, err)
		assert.Equal(t, docker.RegistrySettings{Host: "index.docker.io", Username: "user", Password: "pass"}, s.Registry)
	})

	t.Run("password from stdin trims the trailing newline", func(t *testing.T) {
		cmd, f := newCmd()
		cmd.SetIn(strings.NewReader("hunter2\n"))
		require.NoError(t, cmd.Flags().Set("registry-username", "user"))
		require.NoError(t, cmd.Flags().Set("registry-password-stdin", "true"))

		s, err := f.buildSettings(cmd, "image:latest", "app.example.com")
		require.NoError(t, err)
		assert.Equal(t, "hunter2", s.Registry.Password)
	})

	t.Run("password without username", func(t *testing.T) {
		cmd, f := newCmd()
		require.NoError(t, cmd.Flags().Set("registry-password", "pass"))

		_, err := f.buildSettings(cmd, "image:latest", "app.example.com")
		assert.ErrorIs(t, err, docker.ErrRegistryPasswordWithoutUsername)
	})

	t.Run("username without password", func(t *testing.T) {
		cmd, f := newCmd()
		require.NoError(t, cmd.Flags().Set("registry-username", "user"))

		_, err := f.buildSettings(cmd, "image:latest", "app.example.com")
		assert.ErrorIs(t, err, docker.ErrRegistryUsernameWithoutPassword)
	})

	t.Run("password and password-stdin are mutually exclusive", func(t *testing.T) {
		cmd, _ := newCmd()
		require.NoError(t, cmd.Flags().Set("registry-password", "pass"))
		require.NoError(t, cmd.Flags().Set("registry-password-stdin", "true"))

		assert.Error(t, cmd.ValidateFlagGroups())
	})
}

func TestBuildSettingsAdvancedFlags(t *testing.T) {
	cmd := &cobra.Command{}
	f := &settingsFlags{}
	f.register(cmd)

	require.NoError(t, cmd.Flags().Set("health-check-path", "/healthz"))
	require.NoError(t, cmd.Flags().Set("app-port", "5006"))
	require.NoError(t, cmd.Flags().Set("volume-paths", "/data,/config"))
	require.NoError(t, cmd.Flags().Set("skip-rails-env", "true"))

	s, err := f.buildSettings(cmd, "example/app:latest", "budget.example.com")
	require.NoError(t, err)

	assert.Equal(t, "/healthz", s.HealthCheckPath)
	assert.Equal(t, 5006, s.AppPort)
	assert.Equal(t, []string{"/data", "/config"}, s.VolumePaths)
	assert.True(t, s.SkipRailsEnv)
}

func TestBuildSettingsVolumePathsNormalized(t *testing.T) {
	newCmd := func() (*cobra.Command, *settingsFlags) {
		cmd := &cobra.Command{}
		f := &settingsFlags{}
		f.register(cmd)
		return cmd, f
	}

	t.Run("space after the comma", func(t *testing.T) {
		cmd, f := newCmd()
		require.NoError(t, cmd.Flags().Set("volume-paths", "/data, /config"))

		s, err := f.buildSettings(cmd, "example/app:latest", "budget.example.com")
		require.NoError(t, err)
		assert.Equal(t, []string{"/data", "/config"}, s.VolumePaths)
	})

	t.Run("empty element is dropped", func(t *testing.T) {
		cmd, f := newCmd()
		require.NoError(t, cmd.Flags().Set("volume-paths", "/data,"))

		s, err := f.buildSettings(cmd, "example/app:latest", "budget.example.com")
		require.NoError(t, err)
		assert.Equal(t, []string{"/data"}, s.VolumePaths)
	})

	t.Run("applyChanges normalizes too", func(t *testing.T) {
		cmd, f := newCmd()
		require.NoError(t, cmd.Flags().Set("volume-paths", "/data, /config"))

		existing := docker.ApplicationSettings{Name: "budget", Image: "image:latest"}
		s, err := f.applyChanges(cmd, existing, existing.Image)
		require.NoError(t, err)
		assert.Equal(t, []string{"/data", "/config"}, s.VolumePaths)
	})
}

func TestBuildSettingsDocktailFlags(t *testing.T) {
	cmd := &cobra.Command{}
	f := &settingsFlags{}
	f.register(cmd)

	require.NoError(t, cmd.Flags().Set("docktail", "true"))
	require.NoError(t, cmd.Flags().Set("docktail-service", "Budget"))
	require.NoError(t, cmd.Flags().Set("docktail-port", "5006"))
	require.NoError(t, cmd.Flags().Set("docktail-service-port", "443"))
	require.NoError(t, cmd.Flags().Set("docktail-path", "/budget"))
	require.NoError(t, cmd.Flags().Set("docktail-tags", "tag:server"))

	s, err := f.buildSettings(cmd, "example/app:latest", "budget.example.com")
	require.NoError(t, err)

	assert.True(t, s.DocktailEnabled())
	assert.Equal(t, docker.DocktailMode, s.Tailscale.Mode)
	assert.Equal(t, "budget", s.EffectiveServiceName())
	assert.Equal(t, 5006, s.EffectiveTailscalePort())
	assert.Equal(t, map[string]string{
		"docktail.service.enable":           "true",
		"docktail.funnel.enable":            "false",
		"docktail.service.name":             "budget",
		"docktail.service.port":             "5006",
		"docktail.service.service-port":     "443",
		"docktail.service.protocol":         "http",
		"docktail.service.service-protocol": "https",
		"docktail.service.direct":           "true",
		"docktail.service.path":             "/budget",
		"docktail.tags":                     "tag:server",
	}, s.DocktailLabels())
}

func TestApplyChangesDocktailFlags(t *testing.T) {
	newCmd := func() (*cobra.Command, *settingsFlags) {
		cmd := &cobra.Command{}
		f := &settingsFlags{}
		f.register(cmd)
		return cmd, f
	}

	existing := docker.ApplicationSettings{
		Name:      "budget",
		Image:     "image:latest",
		Tailscale: docker.TailscaleSettings{Enabled: true, Mode: docker.DocktailMode, Service: "budget"},
	}

	t.Run("untouched flags keep the stored settings", func(t *testing.T) {
		cmd, f := newCmd()

		s, err := f.applyChanges(cmd, existing, existing.Image)
		require.NoError(t, err)

		assert.Equal(t, existing.Tailscale, s.Tailscale)
	})

	t.Run("enabling sets the mode", func(t *testing.T) {
		cmd, f := newCmd()
		require.NoError(t, cmd.Flags().Set("docktail", "true"))

		plain := docker.ApplicationSettings{Name: "budget", Image: "image:latest"}
		s, err := f.applyChanges(cmd, plain, plain.Image)
		require.NoError(t, err)

		assert.True(t, s.DocktailEnabled())
	})

	t.Run("disabling clears the mode", func(t *testing.T) {
		cmd, f := newCmd()
		require.NoError(t, cmd.Flags().Set("docktail", "false"))

		s, err := f.applyChanges(cmd, existing, existing.Image)
		require.NoError(t, err)

		assert.False(t, s.Tailscale.Enabled)
		assert.Empty(t, s.Tailscale.Mode)
	})

	t.Run("service override", func(t *testing.T) {
		cmd, f := newCmd()
		require.NoError(t, cmd.Flags().Set("docktail-service", "Other"))

		s, err := f.applyChanges(cmd, existing, existing.Image)
		require.NoError(t, err)

		assert.Equal(t, "Other", s.Tailscale.Service)
		assert.True(t, s.DocktailEnabled(), "the stored mode is preserved")
	})
}

func TestBuildSettingsDocktailOptionalFlags(t *testing.T) {
	cmd := &cobra.Command{}
	f := &settingsFlags{}
	f.register(cmd)

	require.NoError(t, cmd.Flags().Set("docktail", "true"))
	require.NoError(t, cmd.Flags().Set("docktail-port", "5006"))
	require.NoError(t, cmd.Flags().Set("docktail-description", "Budget app"))
	require.NoError(t, cmd.Flags().Set("docktail-protocol", "https"))
	require.NoError(t, cmd.Flags().Set("docktail-service-protocol", "https"))
	require.NoError(t, cmd.Flags().Set("docktail-network", "once"))
	require.NoError(t, cmd.Flags().Set("docktail-direct", "true"))

	s, err := f.buildSettings(cmd, "example/app:latest", "budget.example.com")
	require.NoError(t, err)

	assert.Equal(t, "Budget app", s.Tailscale.Description)
	assert.Equal(t, "https", s.Tailscale.Protocol)
	assert.Equal(t, "https", s.Tailscale.ServiceProtocol)
	assert.Equal(t, "once", s.Tailscale.Network)
	require.NotNil(t, s.Tailscale.Direct)
	assert.True(t, *s.Tailscale.Direct)
	assert.Equal(t, "true", s.DocktailLabels()["docktail.service.direct"])
	assert.Equal(t, "Budget app", s.DocktailLabels()["docktail.service.description"])
	assert.Equal(t, "https", s.DocktailLabels()["docktail.service.protocol"])
	assert.Equal(t, "https", s.DocktailLabels()["docktail.service.service-protocol"])
	assert.Equal(t, "once", s.DocktailLabels()["docktail.service.network"])
}

func TestBuildSettingsDocktailDirectFalseAccepted(t *testing.T) {
	cmd := &cobra.Command{}
	f := &settingsFlags{}
	f.register(cmd)

	require.NoError(t, cmd.Flags().Set("docktail", "true"))
	require.NoError(t, cmd.Flags().Set("docktail-port", "5432"))
	require.NoError(t, cmd.Flags().Set("docktail-direct", "false"))

	s, err := f.buildSettings(cmd, "example/app:latest", "budget.example.com")
	require.NoError(t, err)

	require.NotNil(t, s.Tailscale.Direct)
	assert.False(t, *s.Tailscale.Direct, "the false value round-trips through buildSettings")
	assert.Equal(t, "false", s.DocktailLabels()["docktail.service.direct"], "the docktail label still records false")
}

func TestBuildSettingsDocktailProxyProtocol(t *testing.T) {
	cmd := &cobra.Command{}
	f := &settingsFlags{}
	f.register(cmd)

	require.NoError(t, cmd.Flags().Set("docktail", "true"))
	require.NoError(t, cmd.Flags().Set("docktail-port", "5432"))
	require.NoError(t, cmd.Flags().Set("docktail-service-port", "5432"))
	require.NoError(t, cmd.Flags().Set("docktail-protocol", "tcp"))
	require.NoError(t, cmd.Flags().Set("docktail-proxy-protocol", "2"))

	s, err := f.buildSettings(cmd, "example/app:latest", "db.example.com")
	require.NoError(t, err)

	assert.Equal(t, "2", s.Tailscale.ProxyProtocol)
	assert.Equal(t, "2", s.DocktailLabels()["docktail.service.proxy-protocol"])
}

func TestBuildSettingsDocktailDirectUnset(t *testing.T) {
	cmd := &cobra.Command{}
	f := &settingsFlags{}
	f.register(cmd)

	require.NoError(t, cmd.Flags().Set("docktail", "true"))

	s, err := f.buildSettings(cmd, "example/app:latest", "budget.example.com")
	require.NoError(t, err)

	assert.Nil(t, s.Tailscale.Direct)
	assert.Equal(t, "true", s.DocktailLabels()["docktail.service.direct"])
}

func TestBuildSettingsDocktailDirectInvalid(t *testing.T) {
	cmd := &cobra.Command{}
	f := &settingsFlags{}
	f.register(cmd)

	require.NoError(t, cmd.Flags().Set("docktail", "true"))
	require.NoError(t, cmd.Flags().Set("docktail-direct", "yes"))

	_, err := f.buildSettings(cmd, "example/app:latest", "budget.example.com")
	assert.Error(t, err)
}

func TestApplyChangesDocktailOptionalFlags(t *testing.T) {
	newCmd := func() (*cobra.Command, *settingsFlags) {
		cmd := &cobra.Command{}
		f := &settingsFlags{}
		f.register(cmd)
		return cmd, f
	}

	direct := true
	existing := docker.ApplicationSettings{
		Name:  "budget",
		Image: "image:latest",
		Tailscale: docker.TailscaleSettings{
			Enabled:     true,
			Mode:        docker.DocktailMode,
			Service:     "budget",
			Description: "Budget app",
			Protocol:    "https",
			Network:     "once",
			Direct:      &direct,
		},
	}

	t.Run("untouched flags keep the stored settings", func(t *testing.T) {
		cmd, f := newCmd()

		s, err := f.applyChanges(cmd, existing, existing.Image)
		require.NoError(t, err)

		assert.Equal(t, existing.Tailscale, s.Tailscale)
	})

	t.Run("direct false is accepted and applied", func(t *testing.T) {
		cmd, f := newCmd()
		require.NoError(t, cmd.Flags().Set("docktail-direct", "false"))

		s, err := f.applyChanges(cmd, existing, existing.Image)
		require.NoError(t, err)

		require.NotNil(t, s.Tailscale.Direct)
		assert.False(t, *s.Tailscale.Direct)
	})

	t.Run("empty value clears direct", func(t *testing.T) {
		cmd, f := newCmd()
		require.NoError(t, cmd.Flags().Set("docktail-direct", ""))

		s, err := f.applyChanges(cmd, existing, existing.Image)
		require.NoError(t, err)

		assert.Nil(t, s.Tailscale.Direct)
	})
}
