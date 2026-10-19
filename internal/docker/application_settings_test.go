package docker

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateVAPIDKeyPair(t *testing.T) {
	pub, priv, err := generateVAPIDKeyPair()
	require.NoError(t, err)
	assert.NotEmpty(t, pub)
	assert.NotEmpty(t, priv)

	pubBytes, err := base64.RawURLEncoding.DecodeString(pub)
	require.NoError(t, err)
	assert.Len(t, pubBytes, 65)

	privBytes, err := base64.RawURLEncoding.DecodeString(priv)
	require.NoError(t, err)
	assert.Len(t, privBytes, 32)
}

func TestGenerateVAPIDKeyPairUniqueness(t *testing.T) {
	pub1, priv1, err := generateVAPIDKeyPair()
	require.NoError(t, err)

	pub2, priv2, err := generateVAPIDKeyPair()
	require.NoError(t, err)

	assert.NotEqual(t, pub1, pub2)
	assert.NotEqual(t, priv1, priv2)
}

func TestKeysRegenerate(t *testing.T) {
	original := Keys{
		SecretKeyBase:   "original-secret",
		VAPIDPublicKey:  "original-pub",
		VAPIDPrivateKey: "original-priv",
	}

	t.Run("secret key base only", func(t *testing.T) {
		keys := original
		require.NoError(t, keys.Regenerate(true, false))
		assert.NotEqual(t, original.SecretKeyBase, keys.SecretKeyBase)
		assert.Len(t, keys.SecretKeyBase, 64)
		assert.Equal(t, original.VAPIDPublicKey, keys.VAPIDPublicKey)
		assert.Equal(t, original.VAPIDPrivateKey, keys.VAPIDPrivateKey)
	})

	t.Run("vapid only", func(t *testing.T) {
		keys := original
		require.NoError(t, keys.Regenerate(false, true))
		assert.Equal(t, original.SecretKeyBase, keys.SecretKeyBase)
		assert.NotEqual(t, original.VAPIDPublicKey, keys.VAPIDPublicKey)
		assert.NotEqual(t, original.VAPIDPrivateKey, keys.VAPIDPrivateKey)
	})

	t.Run("both", func(t *testing.T) {
		keys := original
		require.NoError(t, keys.Regenerate(true, true))
		assert.NotEqual(t, original.SecretKeyBase, keys.SecretKeyBase)
		assert.NotEqual(t, original.VAPIDPublicKey, keys.VAPIDPublicKey)
		assert.NotEqual(t, original.VAPIDPrivateKey, keys.VAPIDPrivateKey)
	})

	t.Run("neither", func(t *testing.T) {
		keys := original
		require.NoError(t, keys.Regenerate(false, false))
		assert.Equal(t, original, keys)
	})
}

func TestKeysSetVAPIDKey(t *testing.T) {
	original := Keys{
		SecretKeyBase:   "original-secret",
		VAPIDPublicKey:  "original-pub",
		VAPIDPrivateKey: "original-priv",
	}

	t.Run("derives public key", func(t *testing.T) {
		pub, priv, err := generateVAPIDKeyPair()
		require.NoError(t, err)

		keys := original
		require.NoError(t, keys.SetVAPIDKey(priv))
		assert.Equal(t, original.SecretKeyBase, keys.SecretKeyBase)
		assert.Equal(t, pub, keys.VAPIDPublicKey)
		assert.Equal(t, priv, keys.VAPIDPrivateKey)
	})

	t.Run("normalizes base64 dialects", func(t *testing.T) {
		pub, priv, err := generateVAPIDKeyPair()
		require.NoError(t, err)

		privBytes, err := base64.RawURLEncoding.DecodeString(priv)
		require.NoError(t, err)

		dialects := []string{
			base64.URLEncoding.EncodeToString(privBytes),
			base64.StdEncoding.EncodeToString(privBytes),
			base64.RawStdEncoding.EncodeToString(privBytes),
			" " + priv + "\n",
		}

		for _, dialect := range dialects {
			keys := original
			require.NoError(t, keys.SetVAPIDKey(dialect))
			assert.Equal(t, pub, keys.VAPIDPublicKey)
			assert.Equal(t, priv, keys.VAPIDPrivateKey)
		}
	})

	t.Run("invalid private key", func(t *testing.T) {
		keys := original
		require.Error(t, keys.SetVAPIDKey("not-a-key"))
		assert.Equal(t, original, keys)
	})

	t.Run("empty private key", func(t *testing.T) {
		keys := original
		require.Error(t, keys.SetVAPIDKey(""))
		assert.Equal(t, original, keys)
	})

	t.Run("wrong key length", func(t *testing.T) {
		keys := original
		short := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
		require.Error(t, keys.SetVAPIDKey(short))
		assert.Equal(t, original, keys)
	})
}

func TestContainerResourcesEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 1, MemoryMB: 512}}

	differentCPUs := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 2, MemoryMB: 512}}
	assert.False(t, base.Equal(differentCPUs))

	differentMemory := ApplicationSettings{Name: "app", Resources: ContainerResources{CPUs: 1, MemoryMB: 1024}}
	assert.False(t, base.Equal(differentMemory))

	zeroResources := ApplicationSettings{Name: "app"}
	assert.False(t, base.Equal(zeroResources))
}

func TestContainerResourcesMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:      "app",
		Image:     "img:latest",
		Resources: ContainerResources{CPUs: 2, MemoryMB: 512},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.Equal(t, 2, restored.Resources.CPUs)
	assert.Equal(t, 512, restored.Resources.MemoryMB)
	assert.True(t, original.Equal(restored))
}

func TestAutoUpdateEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", AutoUpdate: false}
	different := ApplicationSettings{Name: "app", AutoUpdate: true}
	assert.False(t, base.Equal(different))
}

func TestBackupSettingsEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/backups", AutoBackup: true}}

	differentPath := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/other", AutoBackup: true}}
	assert.False(t, base.Equal(differentPath))

	differentAutoBackupup := ApplicationSettings{Name: "app", Backup: BackupSettings{Path: "/backups", AutoBackup: false}}
	assert.False(t, base.Equal(differentAutoBackupup))

	noBackup := ApplicationSettings{Name: "app"}
	assert.False(t, base.Equal(noBackup))
}

func TestKeysEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Keys: Keys{SecretKeyBase: "secret"}}

	different := ApplicationSettings{Name: "app", Keys: Keys{SecretKeyBase: "rotated"}}
	assert.False(t, base.Equal(different))

	same := ApplicationSettings{Name: "app", Keys: Keys{SecretKeyBase: "secret"}}
	assert.True(t, base.Equal(same))
}

func TestEnsureKeys(t *testing.T) {
	newApp := func(keys Keys) *Application {
		return NewApplication(nil, ApplicationSettings{Name: "app", Keys: keys})
	}
	volWithKeys := func(keys Keys) *ApplicationVolume {
		return &ApplicationVolume{Settings: ApplicationLegacyVolumeSettings{Keys: keys}}
	}

	t.Run("keeps existing keys", func(t *testing.T) {
		keys := Keys{SecretKeyBase: "existing"}
		app := newApp(keys)
		require.NoError(t, app.ensureKeys(volWithKeys(Keys{SecretKeyBase: "legacy"})))
		assert.Equal(t, keys, app.Settings.Keys)
	})

	t.Run("falls back to legacy volume keys", func(t *testing.T) {
		legacy := Keys{SecretKeyBase: "legacy", VAPIDPublicKey: "pub", VAPIDPrivateKey: "priv"}
		app := newApp(Keys{})
		require.NoError(t, app.ensureKeys(volWithKeys(legacy)))
		assert.Equal(t, legacy, app.Settings.Keys)
	})

	t.Run("generates keys when none exist", func(t *testing.T) {
		app := newApp(Keys{})
		require.NoError(t, app.ensureKeys(volWithKeys(Keys{})))
		assert.NotEmpty(t, app.Settings.Keys.SecretKeyBase)
		assert.NotEmpty(t, app.Settings.Keys.VAPIDPublicKey)
		assert.NotEmpty(t, app.Settings.Keys.VAPIDPrivateKey)
	})
}

func TestEnvVarsMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:  "app",
		Image: "img:latest",
		EnvVars: map[string]string{
			"FOO": "bar",
			"BAZ": "qux",
		},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.Equal(t, "bar", restored.EnvVars["FOO"])
	assert.Equal(t, "qux", restored.EnvVars["BAZ"])
	assert.True(t, original.Equal(restored))
}

func TestEnvVarsEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", EnvVars: map[string]string{"A": "1"}}

	different := ApplicationSettings{Name: "app", EnvVars: map[string]string{"A": "2"}}
	assert.False(t, base.Equal(different))

	extra := ApplicationSettings{Name: "app", EnvVars: map[string]string{"A": "1", "B": "2"}}
	assert.False(t, base.Equal(extra))

	none := ApplicationSettings{Name: "app"}
	assert.False(t, base.Equal(none))
}

func TestRegistrySettingsValidate(t *testing.T) {
	base := ApplicationSettings{Name: "app", Image: "img:latest"}

	t.Run("empty registry settings", func(t *testing.T) {
		assert.NoError(t, base.Validate())
	})

	t.Run("username and password pair", func(t *testing.T) {
		s := base
		s.Registry = RegistrySettings{Host: "docker.io", Username: "user", Password: "pass"}
		assert.NoError(t, s.Validate())
	})

	t.Run("password without username", func(t *testing.T) {
		s := base
		s.Registry = RegistrySettings{Host: "docker.io", Password: "pass"}
		assert.ErrorIs(t, s.Validate(), ErrRegistryPasswordWithoutUsername)
	})

	t.Run("username without password", func(t *testing.T) {
		s := base
		s.Registry = RegistrySettings{Host: "docker.io", Username: "user"}
		assert.ErrorIs(t, s.Validate(), ErrRegistryUsernameWithoutPassword)
	})
}

func TestRegistrySettingsEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "app", Registry: RegistrySettings{Host: "docker.io", Username: "user", Password: "pass"}}

	differentPassword := base
	differentPassword.Registry.Password = "rotated"
	assert.False(t, base.Equal(differentPassword))

	differentScope := base
	differentScope.Registry.Host = "ghcr.io"
	assert.False(t, base.Equal(differentScope))

	same := base
	assert.True(t, base.Equal(same))
}

func TestRegistrySettingsMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:     "app",
		Image:    "img:latest",
		Registry: RegistrySettings{Host: "docker.io", Username: "user", Password: "pass"},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.Equal(t, original.Registry, restored.Registry)
	assert.True(t, original.Equal(restored))
}

func TestAutoUpdateAndBackupMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:       "app",
		Image:      "img:latest",
		AutoUpdate: true,
		Backup:     BackupSettings{Path: "/backups", AutoBackup: true},
	}
	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)
	assert.True(t, restored.AutoUpdate)
	assert.Equal(t, "/backups", restored.Backup.Path)
	assert.True(t, restored.Backup.AutoBackup)
	assert.True(t, original.Equal(restored))
}

func TestEffectiveHealthCheckPath(t *testing.T) {
	assert.Equal(t, HealthCheckPath, ApplicationSettings{}.EffectiveHealthCheckPath())
	assert.Equal(t, "/healthz", ApplicationSettings{HealthCheckPath: "/healthz"}.EffectiveHealthCheckPath())
}

func TestEffectiveAppPort(t *testing.T) {
	assert.Equal(t, DefaultAppPort, ApplicationSettings{}.EffectiveAppPort())
	assert.Equal(t, 5006, ApplicationSettings{AppPort: 5006}.EffectiveAppPort())
}

func TestDeployTarget(t *testing.T) {
	assert.Equal(t, "abc123", ApplicationSettings{}.DeployTarget("abc123"))
	assert.Equal(t, "abc123:5006", ApplicationSettings{AppPort: 5006}.DeployTarget("abc123"))
}

func TestEffectiveVolumePaths(t *testing.T) {
	assert.Equal(t, AppVolumeMountTargets, ApplicationSettings{}.EffectiveVolumePaths())
	assert.Equal(t, "/storage, /rails/storage", ApplicationSettings{}.VolumePathsString())

	custom := ApplicationSettings{VolumePaths: []string{"/data"}}
	assert.Equal(t, []string{"/data"}, custom.EffectiveVolumePaths())
	assert.Equal(t, "/data", custom.VolumePathsString())
}

func TestParseVolumePaths(t *testing.T) {
	assert.Equal(t, []string{"/data", "/config"}, ParseVolumePaths(" /data , /config "))
	assert.Equal(t, []string{"/data"}, ParseVolumePaths("/data,,  ,"))
	assert.Nil(t, ParseVolumePaths(""))
}

func TestAdvancedSettingsValidate(t *testing.T) {
	base := ApplicationSettings{Name: "app", Image: "img:latest"}

	t.Run("default app port and volume paths", func(t *testing.T) {
		assert.NoError(t, base.Validate())
	})

	t.Run("custom app port", func(t *testing.T) {
		s := base
		s.AppPort = 5006
		assert.NoError(t, s.Validate())
	})

	t.Run("negative app port", func(t *testing.T) {
		s := base
		s.AppPort = -1
		assert.ErrorIs(t, s.Validate(), ErrInvalidAppPort)
	})

	t.Run("out of range app port", func(t *testing.T) {
		s := base
		s.AppPort = 65536
		assert.ErrorIs(t, s.Validate(), ErrInvalidAppPort)
	})

	t.Run("relative volume path", func(t *testing.T) {
		s := base
		s.VolumePaths = []string{"data"}
		assert.ErrorIs(t, s.Validate(), ErrInvalidVolumePath)
	})

	t.Run("absolute volume paths", func(t *testing.T) {
		s := base
		s.VolumePaths = []string{"/data", "/config"}
		assert.NoError(t, s.Validate())
	})

	t.Run("relative health check path", func(t *testing.T) {
		s := base
		s.HealthCheckPath = "health"
		assert.ErrorIs(t, s.Validate(), ErrInvalidHealthCheckPath)
	})

	t.Run("absolute health check path", func(t *testing.T) {
		s := base
		s.HealthCheckPath = "/healthz"
		assert.NoError(t, s.Validate())
	})
}

func TestAdvancedSettingsEqualDiffers(t *testing.T) {
	base := ApplicationSettings{
		Name:            "app",
		HealthCheckPath: "/healthz",
		AppPort:         5006,
		VolumePaths:     []string{"/data"},
		SkipRailsEnv:    true,
	}

	assert.True(t, base.Equal(base))

	healthCheck := base
	healthCheck.HealthCheckPath = HealthCheckPath
	assert.False(t, base.Equal(healthCheck))

	appPort := base
	appPort.AppPort = DefaultAppPort
	assert.False(t, base.Equal(appPort))

	volumePaths := base
	volumePaths.VolumePaths = []string{"/data", "/config"}
	assert.False(t, base.Equal(volumePaths))

	skipRailsEnv := base
	skipRailsEnv.SkipRailsEnv = false
	assert.False(t, base.Equal(skipRailsEnv))
}

func TestAdvancedSettingsMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:            "app",
		Image:           "img:latest",
		HealthCheckPath: "/healthz",
		AppPort:         5006,
		VolumePaths:     []string{"/data"},
		SkipRailsEnv:    true,
	}

	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)

	assert.Equal(t, "/healthz", restored.HealthCheckPath)
	assert.Equal(t, 5006, restored.AppPort)
	assert.Equal(t, []string{"/data"}, restored.VolumePaths)
	assert.True(t, restored.SkipRailsEnv)
	assert.True(t, original.Equal(restored))
}

func TestSanitizeDNSLabel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"budget", "budget"},
		{"example-app", "example-app"},
		{"My App", "my-app"},
		{"  Budget  ", "budget"},
		{"app_name.v2", "app-name-v2"},
		{"a///b", "a-b"},
		{"---leading", "leading"},
		{"trailing---", "trailing"},
		{"", ""},
		{"///", ""},
		{"Ünïcodé", "n-cod"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, SanitizeDNSLabel(tt.in))
		})
	}
}

func TestSanitizeDNSLabelTruncation(t *testing.T) {
	assert.Equal(t, strings.Repeat("a", 63), SanitizeDNSLabel(strings.Repeat("a", 70)))
	assert.Equal(t, strings.Repeat("a", 62), SanitizeDNSLabel(strings.Repeat("a", 62)+"-bcd"))
}

func TestDocktailEnabled(t *testing.T) {
	assert.False(t, ApplicationSettings{}.DocktailEnabled())
	assert.False(t, ApplicationSettings{Tailscale: TailscaleSettings{Enabled: true}}.DocktailEnabled())
	assert.False(t, ApplicationSettings{Tailscale: TailscaleSettings{Mode: DocktailMode}}.DocktailEnabled())
	assert.True(t, ApplicationSettings{Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode}}.DocktailEnabled())
}

func TestEffectiveServiceName(t *testing.T) {
	assert.Equal(t, "my-app", ApplicationSettings{Name: "My App"}.EffectiveServiceName())
	assert.Equal(t, "example", ApplicationSettings{Name: "my-app", Tailscale: TailscaleSettings{Service: "Example"}}.EffectiveServiceName())
}

func TestEffectiveTailscalePort(t *testing.T) {
	assert.Equal(t, DefaultAppPort, ApplicationSettings{}.EffectiveTailscalePort())
	assert.Equal(t, 5006, ApplicationSettings{AppPort: 5006}.EffectiveTailscalePort())
	assert.Equal(t, 8080, ApplicationSettings{AppPort: 5006, Tailscale: TailscaleSettings{Port: 8080}}.EffectiveTailscalePort())
}

func TestDocktailLabels(t *testing.T) {
	disabled := ApplicationSettings{Name: "budget"}
	assert.Nil(t, disabled.DocktailLabels())

	enabledWithoutMode := ApplicationSettings{Name: "budget", Tailscale: TailscaleSettings{Enabled: true}}
	assert.Nil(t, enabledWithoutMode.DocktailLabels())

	unknownMode := ApplicationSettings{Name: "budget", Tailscale: TailscaleSettings{Enabled: true, Mode: "funnel"}}
	assert.Nil(t, unknownMode.DocktailLabels())

	derived := ApplicationSettings{Name: "budget", AppPort: 5006, Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode}}
	assert.Equal(t, map[string]string{
		"docktail.service.enable":       "true",
		"docktail.service.name":         "budget",
		"docktail.service.port":         "5006",
		"docktail.service.service-port": "443",
	}, derived.DocktailLabels())

	overridden := ApplicationSettings{
		Name:    "budget",
		AppPort: 5006,
		Tailscale: TailscaleSettings{
			Enabled:     true,
			Mode:        DocktailMode,
			Service:     "Example App",
			Port:        3000,
			ServicePort: 8443,
			Path:        "/budget",
			Tags:        "tag:server,tag:app",
		},
	}
	assert.Equal(t, map[string]string{
		"docktail.service.enable":       "true",
		"docktail.service.name":         "example-app",
		"docktail.service.port":         "3000",
		"docktail.service.service-port": "8443",
		"docktail.service.path":         "/budget",
		"docktail.tags":                 "tag:server,tag:app",
	}, overridden.DocktailLabels())

	conventional := ApplicationSettings{Name: "budget", Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode, ServicePort: 80, Path: "/"}}
	assert.Equal(t, map[string]string{
		"docktail.service.enable":       "true",
		"docktail.service.name":         "budget",
		"docktail.service.port":         "80",
		"docktail.service.service-port": "80",
	}, conventional.DocktailLabels())
}

func TestEffectiveServicePort(t *testing.T) {
	assert.Equal(t, 443, ApplicationSettings{}.EffectiveServicePort())
	assert.Equal(t, 443, ApplicationSettings{Tailscale: TailscaleSettings{ServicePort: 0}}.EffectiveServicePort())
	assert.Equal(t, 80, ApplicationSettings{Tailscale: TailscaleSettings{ServicePort: 80}}.EffectiveServicePort())
	assert.Equal(t, 8443, ApplicationSettings{Tailscale: TailscaleSettings{ServicePort: 8443}}.EffectiveServicePort())
}

func TestDocktailLabelsMarshalRoundTrip(t *testing.T) {
	original := ApplicationSettings{
		Name:      "budget",
		Image:     "example/app:latest",
		AppPort:   5006,
		Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode, Service: "budget", Tags: "tag:app"},
	}

	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)

	assert.Equal(t, original.Tailscale, restored.Tailscale)
	assert.Equal(t, original.DocktailLabels(), restored.DocktailLabels())
	assert.True(t, original.Equal(restored))
}

func TestTailscaleSettingsEqualDiffers(t *testing.T) {
	base := ApplicationSettings{Name: "budget", Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode}}

	disabled := base
	disabled.Tailscale.Enabled = false
	assert.False(t, base.Equal(disabled))

	renamed := base
	renamed.Tailscale.Service = "other"
	assert.False(t, base.Equal(renamed))

	assert.True(t, base.Equal(base))
}

func TestValidateTailscale(t *testing.T) {
	base := ApplicationSettings{Name: "budget", Image: "img:latest"}

	t.Run("disabled", func(t *testing.T) {
		assert.NoError(t, base.ValidateTailscale())
	})

	t.Run("enabled with a mode", func(t *testing.T) {
		s := base
		s.Tailscale = TailscaleSettings{Enabled: true, Mode: DocktailMode}
		assert.NoError(t, s.ValidateTailscale())
	})

	t.Run("enabled without a mode", func(t *testing.T) {
		s := base
		s.Tailscale = TailscaleSettings{Enabled: true}
		assert.ErrorIs(t, s.ValidateTailscale(), ErrInvalidDocktailMode)
	})

	t.Run("unsupported mode", func(t *testing.T) {
		s := base
		s.Tailscale = TailscaleSettings{Enabled: true, Mode: "tsnet"}
		assert.ErrorIs(t, s.ValidateTailscale(), ErrInvalidDocktailMode)
	})

	t.Run("service name sanitizes to nothing", func(t *testing.T) {
		s := base
		s.Tailscale = TailscaleSettings{Enabled: true, Mode: DocktailMode, Service: "///"}
		assert.ErrorIs(t, s.ValidateTailscale(), ErrDocktailServiceRequired)
	})

	t.Run("app name sanitizes to nothing", func(t *testing.T) {
		s := base
		s.Name = "///"
		s.Tailscale = TailscaleSettings{Enabled: true, Mode: DocktailMode}
		assert.ErrorIs(t, s.ValidateTailscale(), ErrDocktailServiceRequired)
	})

	t.Run("ports out of range", func(t *testing.T) {
		s := base
		s.Tailscale = TailscaleSettings{Enabled: true, Mode: DocktailMode, Port: 70000}
		assert.ErrorIs(t, s.ValidateTailscale(), ErrInvalidDocktailPort)

		s.Tailscale = TailscaleSettings{Enabled: true, Mode: DocktailMode, ServicePort: -1}
		assert.ErrorIs(t, s.ValidateTailscale(), ErrInvalidDocktailPort)
	})

	t.Run("relative path", func(t *testing.T) {
		s := base
		s.Tailscale = TailscaleSettings{Enabled: true, Mode: DocktailMode, Path: "budget"}
		assert.ErrorIs(t, s.ValidateTailscale(), ErrInvalidDocktailPath)
	})

	t.Run("absolute path", func(t *testing.T) {
		s := base
		s.Tailscale = TailscaleSettings{Enabled: true, Mode: DocktailMode, Path: "/budget"}
		assert.NoError(t, s.ValidateTailscale())
	})

	t.Run("valid ports", func(t *testing.T) {
		s := base
		s.Tailscale = TailscaleSettings{Enabled: true, Mode: DocktailMode, Port: 5006, ServicePort: 443}
		assert.NoError(t, s.ValidateTailscale())
	})
}

func TestDocktailLabelsOptionalFields(t *testing.T) {
	pointer := func(value bool) *bool { return &value }

	t.Run("unset optional labels are omitted", func(t *testing.T) {
		s := ApplicationSettings{Name: "budget", Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode}}
		assert.Equal(t, map[string]string{
			"docktail.service.enable":       "true",
			"docktail.service.name":         "budget",
			"docktail.service.port":         "80",
			"docktail.service.service-port": "443",
		}, s.DocktailLabels())
	})

	t.Run("all optional labels are emitted", func(t *testing.T) {
		s := ApplicationSettings{
			Name: "budget",
			Tailscale: TailscaleSettings{
				Enabled:         true,
				Mode:            DocktailMode,
				Port:            5432,
				ServicePort:     5432,
				Path:            "/budget",
				Tags:            "tag:server",
				Description:     "Budget app",
				Protocol:        "tcp",
				ServiceProtocol: "tls-terminated-tcp",
				ProxyProtocol:   "2",
				Direct:          pointer(false),
				Network:         "once",
			},
		}
		assert.Equal(t, map[string]string{
			"docktail.service.enable":           "true",
			"docktail.service.name":             "budget",
			"docktail.service.port":             "5432",
			"docktail.service.service-port":     "5432",
			"docktail.service.path":             "/budget",
			"docktail.tags":                     "tag:server",
			"docktail.service.description":      "Budget app",
			"docktail.service.protocol":         "tcp",
			"docktail.service.service-protocol": "tls-terminated-tcp",
			"docktail.service.proxy-protocol":   "2",
			"docktail.service.direct":           "false",
			"docktail.service.network":          "once",
		}, s.DocktailLabels())
	})

	t.Run("direct true is emitted", func(t *testing.T) {
		s := ApplicationSettings{Name: "budget", Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode, Direct: pointer(true)}}
		assert.Equal(t, "true", s.DocktailLabels()["docktail.service.direct"])
	})
}

func TestEffectiveDocktailProtocols(t *testing.T) {
	assert.Equal(t, "http", ApplicationSettings{}.effectiveProtocol())
	assert.Equal(t, "https", ApplicationSettings{AppPort: 443}.effectiveProtocol())
	assert.Equal(t, "https", ApplicationSettings{Tailscale: TailscaleSettings{Port: 443}}.effectiveProtocol())
	assert.Equal(t, "tcp", ApplicationSettings{Tailscale: TailscaleSettings{Protocol: "tcp"}}.effectiveProtocol())

	assert.Equal(t, "https", ApplicationSettings{}.effectiveServiceProtocol())
	assert.Equal(t, "http", ApplicationSettings{Tailscale: TailscaleSettings{ServicePort: 80}}.effectiveServiceProtocol())
	assert.Equal(t, "http", ApplicationSettings{Tailscale: TailscaleSettings{ServicePort: 5432}}.effectiveServiceProtocol())
	assert.Equal(t, "tcp", ApplicationSettings{Tailscale: TailscaleSettings{ServicePort: 5432, Protocol: "tcp"}}.effectiveServiceProtocol())
	assert.Equal(t, "https", ApplicationSettings{Tailscale: TailscaleSettings{ServicePort: 8443, ServiceProtocol: "https"}}.effectiveServiceProtocol())
	assert.Equal(t, "tls-terminated-tcp", ApplicationSettings{Tailscale: TailscaleSettings{ServiceProtocol: "tls-terminated-tcp"}}.effectiveServiceProtocol())
}

func TestValidateTailscaleProtocols(t *testing.T) {
	settings := func(tailscale TailscaleSettings) ApplicationSettings {
		tailscale.Enabled, tailscale.Mode = true, DocktailMode
		return ApplicationSettings{Name: "budget", Image: "img:latest", Tailscale: tailscale}
	}

	t.Run("backend protocol", func(t *testing.T) {
		for _, protocol := range []string{"http", "https", "tcp"} {
			assert.NoError(t, settings(TailscaleSettings{Protocol: protocol}).ValidateTailscale())
		}
		assert.ErrorIs(t, settings(TailscaleSettings{Protocol: "grpc"}).ValidateTailscale(), ErrInvalidDocktailProtocol)
	})

	t.Run("service protocol", func(t *testing.T) {
		for _, protocol := range []string{"http", "https", "tcp", "tls-terminated-tcp"} {
			assert.NoError(t, settings(TailscaleSettings{ServiceProtocol: protocol}).ValidateTailscale())
		}
		assert.ErrorIs(t, settings(TailscaleSettings{ServiceProtocol: "udp"}).ValidateTailscale(), ErrInvalidDocktailServiceProtocol)
	})

	t.Run("proxy protocol enum", func(t *testing.T) {
		assert.NoError(t, settings(TailscaleSettings{Port: 5432, ServicePort: 5432, Protocol: "tcp", ProxyProtocol: "1"}).ValidateTailscale())
		assert.NoError(t, settings(TailscaleSettings{Port: 5432, ServicePort: 5432, Protocol: "tcp", ProxyProtocol: "2"}).ValidateTailscale())
		assert.ErrorIs(t, settings(TailscaleSettings{ProxyProtocol: "3"}).ValidateTailscale(), ErrInvalidDocktailProxyProtocol)
	})

	t.Run("proxy protocol requires TCP", func(t *testing.T) {
		assert.ErrorIs(t, settings(TailscaleSettings{ProxyProtocol: "2"}).ValidateTailscale(), ErrDocktailProxyProtocolWithoutTCP)
		assert.ErrorIs(t, settings(TailscaleSettings{ServicePort: 8443, ServiceProtocol: "https", ProxyProtocol: "2"}).ValidateTailscale(), ErrDocktailProxyProtocolWithoutTCP)
		assert.NoError(t, settings(TailscaleSettings{ServiceProtocol: "tcp", ProxyProtocol: "2"}).ValidateTailscale())
		assert.NoError(t, settings(TailscaleSettings{ServiceProtocol: "tls-terminated-tcp", ProxyProtocol: "2"}).ValidateTailscale())
	})

	t.Run("path is rejected for TCP", func(t *testing.T) {
		assert.ErrorIs(t, settings(TailscaleSettings{Port: 5432, ServicePort: 5432, Protocol: "tcp", Path: "/budget"}).ValidateTailscale(), ErrDocktailPathOnTCP)
		assert.NoError(t, settings(TailscaleSettings{Port: 5432, ServicePort: 5432, Protocol: "tcp", Path: "/"}).ValidateTailscale())
		assert.NoError(t, settings(TailscaleSettings{Path: "/budget"}).ValidateTailscale())
	})

	t.Run("install validation includes tailscale checks", func(t *testing.T) {
		assert.ErrorIs(t, settings(TailscaleSettings{ProxyProtocol: "2"}).Validate(), ErrDocktailProxyProtocolWithoutTCP)
		assert.ErrorIs(t, settings(TailscaleSettings{Protocol: "grpc"}).Validate(), ErrInvalidDocktailProtocol)
	})
}

func TestTailscaleSettingsMarshalWithoutOptionalFields(t *testing.T) {
	s := ApplicationSettings{Name: "budget", Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode}}
	assert.Equal(t, `{"name":"budget","image":"","host":"","disableTLS":false,"tailscale":{"enabled":true,"mode":"docktail"},"env":null,"smtp":{},"resources":{},"autoUpdate":false,"backup":{},"keys":{},"registry":{}}`, s.Marshal())
}

func TestDocktailLabelsOptionalFieldsMarshalRoundTrip(t *testing.T) {
	direct := false
	original := ApplicationSettings{
		Name:    "budget",
		Image:   "example/app:latest",
		AppPort: 5006,
		Tailscale: TailscaleSettings{
			Enabled:         true,
			Mode:            DocktailMode,
			Service:         "budget",
			Port:            5432,
			ServicePort:     5432,
			Path:            "/budget",
			Tags:            "tag:app",
			Description:     "Budget app",
			Protocol:        "tcp",
			ServiceProtocol: "tls-terminated-tcp",
			ProxyProtocol:   "2",
			Direct:          &direct,
			Network:         "once",
		},
	}

	restored, err := UnmarshalApplicationSettings(original.Marshal())
	require.NoError(t, err)

	assert.Equal(t, original.Tailscale, restored.Tailscale)
	assert.Equal(t, original.DocktailLabels(), restored.DocktailLabels())
	assert.True(t, original.Equal(restored))
}

func TestTailscaleSettingsEqualDirect(t *testing.T) {
	first, second := false, false
	truth := true

	assert.True(t, TailscaleSettings{Direct: &first}.Equal(TailscaleSettings{Direct: &second}))
	assert.False(t, TailscaleSettings{}.Equal(TailscaleSettings{Direct: &first}))
	assert.False(t, TailscaleSettings{Direct: &first}.Equal(TailscaleSettings{Direct: &truth}))
}
