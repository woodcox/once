package command

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

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
		"docktail.service.enable":       "true",
		"docktail.service.name":         "budget",
		"docktail.service.port":         "5006",
		"docktail.service.service-port": "443",
		"docktail.service.path":         "/budget",
		"docktail.tags":                 "tag:server",
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
