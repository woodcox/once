package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyHTTP_Success(t *testing.T) {
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	app := &Application{
		Settings: ApplicationSettings{Host: server.Listener.Addr().String(), DisableTLS: true},
	}

	err := app.verifyHTTP(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, HealthCheckPath, requestPath)
}

func TestVerifyHTTP_RedirectToSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/home", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	app := &Application{
		Settings: ApplicationSettings{Host: server.Listener.Addr().String(), DisableTLS: true},
	}

	err := app.verifyHTTP(context.Background())
	assert.NoError(t, err)
}

func TestVerifyHTTP_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	app := &Application{
		Settings: ApplicationSettings{Host: server.Listener.Addr().String(), DisableTLS: true},
	}

	err := app.verifyHTTP(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVerificationFailed)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestVerifyHTTP_Unreachable(t *testing.T) {
	app := &Application{
		Settings: ApplicationSettings{Host: "127.0.0.1:1", DisableTLS: true},
	}

	err := app.verifyHTTP(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVerificationFailed)
}

func TestVerifyHTTP_NoHost(t *testing.T) {
	app := &Application{
		Settings: ApplicationSettings{},
	}

	err := app.verifyHTTP(context.Background())
	assert.NoError(t, err)
}

func TestURL(t *testing.T) {
	newAppWithProxy := func(host string, disableTLS bool, proxySettings *ProxySettings) *Application {
		ns := &Namespace{}
		ns.proxy = &Proxy{Settings: proxySettings}
		return &Application{
			namespace: ns,
			Settings:  ApplicationSettings{Host: host, DisableTLS: disableTLS},
		}
	}

	t.Run("empty host", func(t *testing.T) {
		app := &Application{Settings: ApplicationSettings{}}
		assert.Equal(t, "", app.URL())
	})

	t.Run("nil namespace", func(t *testing.T) {
		app := &Application{Settings: ApplicationSettings{Host: "app.example.com"}}
		assert.Equal(t, "https://app.example.com", app.URL())
	})

	t.Run("nil proxy settings", func(t *testing.T) {
		ns := &Namespace{}
		ns.proxy = &Proxy{}
		app := &Application{
			namespace: ns,
			Settings:  ApplicationSettings{Host: "app.localhost", DisableTLS: true},
		}
		assert.Equal(t, "http://app.localhost", app.URL())
	})

	t.Run("default HTTP port", func(t *testing.T) {
		app := newAppWithProxy("app.localhost", true, &ProxySettings{HTTPPort: 80})
		assert.Equal(t, "http://app.localhost", app.URL())
	})

	t.Run("custom HTTP port", func(t *testing.T) {
		app := newAppWithProxy("app.localhost", true, &ProxySettings{HTTPPort: 8080})
		assert.Equal(t, "http://app.localhost:8080", app.URL())
	})

	t.Run("default HTTPS port", func(t *testing.T) {
		app := newAppWithProxy("app.example.com", false, &ProxySettings{HTTPSPort: 443})
		assert.Equal(t, "https://app.example.com", app.URL())
	})

	t.Run("custom HTTPS port", func(t *testing.T) {
		app := newAppWithProxy("app.example.com", false, &ProxySettings{HTTPSPort: 8443})
		assert.Equal(t, "https://app.example.com:8443", app.URL())
	})

	t.Run("localhost disables TLS", func(t *testing.T) {
		app := newAppWithProxy("chat.localhost", false, &ProxySettings{HTTPPort: 9090})
		assert.Equal(t, "http://chat.localhost:9090", app.URL())
	})
}

func TestBuildEnvWithSMTP(t *testing.T) {
	settings := ApplicationSettings{
		SMTP: SMTPSettings{
			Server:   "smtp.example.com",
			Port:     "587",
			Username: "user@example.com",
			Password: "secret",
			From:     "noreply@example.com",
		},
	}

	env := (&Application{Settings: settings}).BuildEnv()

	assert.Contains(t, env, "SMTP_ADDRESS=smtp.example.com")
	assert.Contains(t, env, "SMTP_PORT=587")
	assert.Contains(t, env, "SMTP_USERNAME=user@example.com")
	assert.Contains(t, env, "SMTP_PASSWORD=secret")
	assert.Contains(t, env, "MAILER_FROM_ADDRESS=noreply@example.com")
}

func TestBuildEnvWithCPULimit(t *testing.T) {
	settings := ApplicationSettings{Resources: ContainerResources{CPUs: 4}}

	env := (&Application{Settings: settings}).BuildEnv()

	assert.Contains(t, env, "NUM_CPUS=4")
}

func TestBuildEnvWithoutCPULimit(t *testing.T) {
	settings := ApplicationSettings{}

	env := (&Application{Settings: settings}).BuildEnv()

	assert.NotContains(t, env, "NUM_CPUS=0")
}

func TestBuildEnvWithBaseURL(t *testing.T) {
	settings := ApplicationSettings{Host: "app.example.com"}

	env := (&Application{Settings: settings}).BuildEnv()

	assert.Contains(t, env, "BASE_URL=https://app.example.com")
}

func TestBuildEnvWithoutBaseURL(t *testing.T) {
	settings := ApplicationSettings{}

	for _, e := range (&Application{Settings: settings}).BuildEnv() {
		assert.NotContains(t, e, "BASE_URL=")
	}
}

func TestBuildEnvCustomBaseURLOverridesGenerated(t *testing.T) {
	settings := ApplicationSettings{
		Host:    "app.example.com",
		EnvVars: map[string]string{"BASE_URL": "https://custom.example.com"},
	}

	env := (&Application{Settings: settings}).BuildEnv()

	generated := slices.Index(env, "BASE_URL=https://app.example.com")
	custom := slices.Index(env, "BASE_URL=https://custom.example.com")
	assert.NotEqual(t, -1, generated)
	assert.NotEqual(t, -1, custom)
	assert.Greater(t, custom, generated)
}

func TestBuildEnvWithoutSMTP(t *testing.T) {
	settings := ApplicationSettings{}

	env := (&Application{Settings: settings}).BuildEnv()

	for _, e := range env {
		assert.NotContains(t, e, "SMTP_")
	}
}

func TestBuildEnvWithKeys(t *testing.T) {
	settings := ApplicationSettings{
		Keys: Keys{
			SecretKeyBase:   "test-secret-key",
			VAPIDPublicKey:  "test-vapid-public",
			VAPIDPrivateKey: "test-vapid-private",
		},
	}

	env := (&Application{Settings: settings}).BuildEnv()

	assert.Contains(t, env, "SECRET_KEY_BASE=test-secret-key")
	assert.Contains(t, env, "VAPID_PUBLIC_KEY=test-vapid-public")
	assert.Contains(t, env, "VAPID_PRIVATE_KEY=test-vapid-private")
}

func TestBuildEnvWithEnvVars(t *testing.T) {
	settings := ApplicationSettings{
		EnvVars: map[string]string{
			"DB_HOST": "postgres.local",
			"DB_NAME": "mydb",
		},
	}

	env := (&Application{Settings: settings}).BuildEnv()

	assert.Contains(t, env, "DB_HOST=postgres.local")
	assert.Contains(t, env, "DB_NAME=mydb")
}

func TestBuildEnvExcludesRegistryCredentials(t *testing.T) {
	settings := ApplicationSettings{
		Registry: RegistrySettings{Host: "docker.io", Username: "registry-user", Password: "registry-pass"},
	}

	for _, e := range (&Application{Settings: settings}).BuildEnv() {
		assert.NotContains(t, e, "registry-user")
		assert.NotContains(t, e, "registry-pass")
	}
}

func TestVerifyHTTP_UsesCustomHealthCheckPath(t *testing.T) {
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	app := &Application{
		Settings: ApplicationSettings{
			Host:            server.Listener.Addr().String(),
			DisableTLS:      true,
			HealthCheckPath: "/healthz",
		},
	}

	require.NoError(t, app.verifyHTTP(context.Background()))
	assert.Equal(t, "/healthz", requestPath)
}

func TestBuildEnvSkipsRailsEnv(t *testing.T) {
	settings := ApplicationSettings{
		Keys: Keys{
			SecretKeyBase:   "test-secret-key",
			VAPIDPublicKey:  "test-vapid-public",
			VAPIDPrivateKey: "test-vapid-private",
		},
		SkipRailsEnv: true,
	}

	env := (&Application{Settings: settings}).BuildEnv()

	for _, e := range env {
		assert.NotContains(t, e, "SECRET_KEY_BASE=")
		assert.NotContains(t, e, "VAPID_PUBLIC_KEY=")
		assert.NotContains(t, e, "VAPID_PRIVATE_KEY=")
		assert.NotContains(t, e, "DISABLE_SSL=")
	}
}

func TestBuildEnvIncludesRailsEnvByDefault(t *testing.T) {
	settings := ApplicationSettings{Keys: Keys{SecretKeyBase: "test-secret-key"}}

	env := (&Application{Settings: settings}).BuildEnv()

	assert.Contains(t, env, "SECRET_KEY_BASE=test-secret-key")
}

func TestVolumeMountsUseConfiguredPaths(t *testing.T) {
	vol := &ApplicationVolume{name: "once-app-myapp"}

	t.Run("defaults", func(t *testing.T) {
		app := &Application{Settings: ApplicationSettings{Name: "myapp"}}
		mounts := app.volumeMounts(vol)

		targets := make([]string, 0, len(mounts))
		for _, m := range mounts {
			targets = append(targets, m.Target)
		}
		assert.Equal(t, AppVolumeMountTargets, targets)
	})

	t.Run("custom paths", func(t *testing.T) {
		app := &Application{Settings: ApplicationSettings{Name: "myapp", VolumePaths: []string{"/data"}}}
		mounts := app.volumeMounts(vol)

		require.Len(t, mounts, 1)
		assert.Equal(t, "/data", mounts[0].Target)
		assert.Equal(t, "once-app-myapp", mounts[0].Source)
	})
}

func TestContainerConfigMergesDocktailLabels(t *testing.T) {
	settings := ApplicationSettings{
		Name:      "budget",
		Image:     "example/app:latest",
		AppPort:   5006,
		Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode},
	}

	config := (&Application{Settings: settings}).containerConfig([]string{"FOO=bar"})

	assert.Equal(t, settings.Marshal(), config.Labels[labelKey])
	assert.Equal(t, "true", config.Labels[docktailLabelEnable])
	assert.Equal(t, "budget", config.Labels[docktailLabelName])
	assert.Equal(t, "5006", config.Labels[docktailLabelPort])
	assert.Equal(t, []string{"FOO=bar"}, config.Env)
}

func TestContainerConfigWithoutDocktailLabels(t *testing.T) {
	settings := ApplicationSettings{Name: "budget", Image: "img:latest"}

	config := (&Application{Settings: settings}).containerConfig(nil)

	assert.Equal(t, settings.Marshal(), config.Labels[labelKey])
	assert.Len(t, config.Labels, 1)
}

func TestUsesProxy(t *testing.T) {
	t.Run("plain app uses proxy", func(t *testing.T) {
		app := &Application{Settings: ApplicationSettings{Name: "chat", Host: "chat.example.com"}}
		assert.True(t, app.usesProxy())
	})

	t.Run("docktail app skips proxy", func(t *testing.T) {
		app := &Application{Settings: ApplicationSettings{
			Name:      "budget",
			Host:      "budget.example.com",
			Tailscale: TailscaleSettings{Enabled: true, Mode: DocktailMode},
		}}
		assert.False(t, app.usesProxy())
	})

	t.Run("tailscale toggled on but mode unset uses proxy", func(t *testing.T) {
		app := &Application{Settings: ApplicationSettings{
			Name:      "budget",
			Host:      "budget.example.com",
			Tailscale: TailscaleSettings{Enabled: true},
		}}
		assert.True(t, app.usesProxy())
	})
}
