package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/once/internal/docker"
)

func TestSettingsFormApplication_InitialState_NonLocalhost(t *testing.T) {
	settings := docker.ApplicationSettings{
		Image:      "ghcr.io/basecamp/once-campfire:latest",
		Host:       "app.example.com",
		DisableTLS: false,
	}
	form := NewSettingsFormApplication(settings)

	assert.Equal(t, 0, form.form.Focused())
	assert.Equal(t, "ghcr.io/basecamp/once-campfire:latest", form.form.TextField(appImageField).Value())
	assert.Equal(t, "app.example.com", form.form.TextField(appHostnameField).Value())
	assert.True(t, form.form.CheckboxField(appTLSField).Checked())
}

func TestSettingsFormApplication_InitialState_Localhost(t *testing.T) {
	settings := docker.ApplicationSettings{
		Image:      "ghcr.io/basecamp/once-campfire:latest",
		Host:       "chat.localhost",
		DisableTLS: false,
	}
	form := NewSettingsFormApplication(settings)

	assert.Equal(t, "chat.localhost", form.form.TextField(appHostnameField).Value())
	assert.True(t, form.form.CheckboxField(appTLSField).Checked(), "checkbox is checked (DisableTLS=false)")
}

func TestSettingsFormApplication_TabNavigation(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "app.example.com"})
	assert.Equal(t, appImageField, form.form.Focused())

	for i := appHostnameField; i < form.form.ItemCount(); i++ {
		applicationPressTab(&form)
		assert.Equal(t, i, form.form.Focused())
	}

	applicationPressTab(&form)
	assert.Equal(t, form.form.ItemCount(), form.form.Focused(), "done button")

	applicationPressTab(&form)
	assert.Equal(t, form.form.ItemCount()+1, form.form.Focused(), "cancel button")

	applicationPressTab(&form)
	assert.Equal(t, appImageField, form.form.Focused(), "wraps to image")
}

func TestSettingsFormApplication_ShiftTabNavigation(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "app.example.com"})

	applicationPressShiftTab(&form)
	assert.Equal(t, form.form.ItemCount()+1, form.form.Focused(), "cancel button")

	applicationPressShiftTab(&form)
	assert.Equal(t, form.form.ItemCount(), form.form.Focused(), "done button")
}

func TestSettingsFormApplication_SpaceTogglesTLS(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "app.example.com"})
	assert.True(t, form.form.CheckboxField(appTLSField).Checked())

	applicationPressTab(&form)
	applicationPressTab(&form)
	assert.Equal(t, 2, form.form.Focused())

	applicationPressSpace(&form)
	assert.False(t, form.form.CheckboxField(appTLSField).Checked())

	applicationPressSpace(&form)
	assert.True(t, form.form.CheckboxField(appTLSField).Checked())
}

func TestSettingsFormApplication_SpaceDoesNotToggleTLSForLocalhost(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "chat.localhost"})

	applicationPressTab(&form)
	applicationPressTab(&form)
	assert.Equal(t, 2, form.form.Focused())

	applicationPressSpace(&form)
	assert.True(t, form.form.CheckboxField(appTLSField).Checked(), "toggle ignored for localhost")
}

func TestSettingsFormApplication_TLSShowsDisabledForLocalhost(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "app.example.com"})
	assert.Equal(t, "[✓] Enabled", form.form.CheckboxField(appTLSField).View())

	applicationPressTab(&form)
	applicationTypeText(&form, ".localhost")
	assert.Equal(t, "Not available for localhost", form.form.CheckboxField(appTLSField).View())

	applicationClearAndType(&form, "app.example.com")
	assert.Equal(t, "[✓] Enabled", form.form.CheckboxField(appTLSField).View())
}

func TestSettingsFormApplication_Submit(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{
		Name:  "myapp",
		Image: "ghcr.io/basecamp/once-campfire:latest",
		Host:  "app.example.com",
	})

	applicationTabToSubmit(&form)

	var cmd tea.Cmd
	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormApplication)
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	assert.Equal(t, "myapp", submitMsg.Settings.Name)
	assert.Equal(t, "ghcr.io/basecamp/once-campfire:latest", submitMsg.Settings.Image)
	assert.Equal(t, "app.example.com", submitMsg.Settings.Host)
	assert.False(t, submitMsg.Settings.DisableTLS)
}

func TestSettingsFormApplication_Cancel(t *testing.T) {
	form := NewSettingsFormApplication(docker.ApplicationSettings{Host: "app.example.com"})

	applicationTabToCancel(&form)

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormApplication)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SettingsSectionCancelMsg)
	assert.True(t, ok, "expected SettingsSectionCancelMsg, got %T", msg)
}

func TestSettingsFormApplication_LeavesTailscaleSettingsAlone(t *testing.T) {
	original := docker.ApplicationSettings{
		Name:  "budget",
		Image: "example/app:latest",
		Host:  "budget.example.com",
		Tailscale: docker.TailscaleSettings{
			Enabled:     true,
			Mode:        docker.DocktailMode,
			Service:     "budget",
			ServicePort: 443,
			Path:        "/",
		},
	}

	form := NewSettingsFormApplication(original)

	assert.NotContains(t, ansi.Strip(form.View()), "Tailscale")
	assert.True(t, original.Equal(applicationSubmit(t, &form)), "the Application section keeps the Service settings untouched")
}

func TestSettingsFormTailscale_InitialState(t *testing.T) {
	settings := docker.ApplicationSettings{
		Name: "budget",
		Tailscale: docker.TailscaleSettings{
			Enabled:     true,
			Mode:        docker.DocktailMode,
			Service:     "budget",
			Port:        3000,
			ServicePort: 8443,
			Path:        "/budget",
			Tags:        "tag:server",
		},
	}

	form := NewSettingsFormTailscale(settings)

	assert.Equal(t, "Tailscale", form.Title())
	assert.True(t, form.form.CheckboxField(tailscaleEnabledField).Checked())
	assert.Equal(t, "budget", form.form.TextField(tailscaleServiceNameField).Value())
	assert.Equal(t, "3000", form.form.TextField(tailscaleBackendPortField).Value())
	assert.Equal(t, "8443", form.form.TextField(tailscaleServicePortField).Value())
	assert.Equal(t, "/budget", form.form.TextField(tailscalePathField).Value())
	assert.Equal(t, "tag:server", form.form.TextField(tailscaleTagsField).Value())
}

func TestSettingsFormTailscale_RoundTrip(t *testing.T) {
	original := docker.ApplicationSettings{
		Name:  "budget",
		Image: "example/app:latest",
		Host:  "budget.example.com",
		Tailscale: docker.TailscaleSettings{
			Enabled:     true,
			Mode:        docker.DocktailMode,
			Service:     "budget",
			Port:        3000,
			ServicePort: 8443,
			Path:        "/budget",
			Tags:        "tag:server",
		},
	}

	form := NewSettingsFormTailscale(original)

	result := tailscaleSubmit(t, &form)

	assert.True(t, original.Equal(result))
	assert.Equal(t, map[string]string{
		"docktail.service.enable":       "true",
		"docktail.service.name":         "budget",
		"docktail.service.port":         "3000",
		"docktail.service.service-port": "8443",
		"docktail.service.path":         "/budget",
		"docktail.tags":                 "tag:server",
	}, result.DocktailLabels())
}

func TestSettingsFormTailscale_RoundTripsEveryLabel(t *testing.T) {
	direct := false
	original := docker.ApplicationSettings{
		Name:    "budget",
		Image:   "example/app:latest",
		Host:    "budget.example.com",
		AppPort: 3000,
		Tailscale: docker.TailscaleSettings{
			Enabled:         true,
			Mode:            docker.DocktailMode,
			Service:         "budget",
			Description:     "Budget tracker",
			Port:            3000,
			Protocol:        "https+insecure",
			ServicePort:     8443,
			ServiceProtocol: "https",
			Path:            "/budget",
			Tags:            "tag:server",
			Network:         "tailnet0",
			Direct:          &direct,
		},
	}

	form := NewSettingsFormTailscale(original)

	assert.Equal(t, "Budget tracker", form.form.TextField(tailscaleDescriptionField).Value())
	assert.Equal(t, "https+insecure", form.form.TextField(tailscaleBackendProtocolField).Value())
	assert.Equal(t, "https", form.form.TextField(tailscaleServiceProtocolField).Value())
	assert.Equal(t, "tailnet0", form.form.TextField(tailscaleNetworkField).Value())
	assert.True(t, form.form.CheckboxField(tailscaleDirectField).Checked())

	result := tailscaleSubmit(t, &form)

	assert.True(t, original.Equal(result))
	assert.Equal(t, map[string]string{
		"docktail.service.enable":           "true",
		"docktail.service.name":             "budget",
		"docktail.service.port":             "3000",
		"docktail.service.service-port":     "8443",
		"docktail.service.protocol":         "https+insecure",
		"docktail.service.service-protocol": "https",
		"docktail.service.description":      "Budget tracker",
		"docktail.service.network":          "tailnet0",
		"docktail.service.direct":           "false",
		"docktail.service.path":             "/budget",
		"docktail.tags":                     "tag:server",
	}, result.DocktailLabels())
}

func TestSettingsFormTailscale_ProxyProtocolRoundTrip(t *testing.T) {
	original := docker.ApplicationSettings{
		Name: "postgres",
		Tailscale: docker.TailscaleSettings{
			Enabled:         true,
			Mode:            docker.DocktailMode,
			Service:         "postgres",
			Port:            5432,
			Protocol:        "tcp",
			ServicePort:     5432,
			ServiceProtocol: "tls-terminated-tcp",
			Path:            "/",
			ProxyProtocol:   "1",
		},
	}

	form := NewSettingsFormTailscale(original)

	assert.Equal(t, "1", form.form.TextField(tailscaleProxyProtocolField).Value())

	result := tailscaleSubmit(t, &form)

	assert.True(t, original.Equal(result))
	assert.Equal(t, "1", result.DocktailLabels()["docktail.service.proxy-protocol"])
}

func TestSettingsFormTailscale_DirectRoundTrip(t *testing.T) {
	form := NewSettingsFormTailscale(docker.ApplicationSettings{Name: "budget", Tailscale: docker.TailscaleSettings{
		Enabled: true,
		Mode:    docker.DocktailMode,
		Service: "budget",
	}})

	assert.Nil(t, tailscaleSubmit(t, &form).Tailscale.Direct)

	form.form.CheckboxField(tailscaleDirectField).Toggle()
	direct := tailscaleSubmit(t, &form).Tailscale.Direct
	require.NotNil(t, direct)
	assert.False(t, *direct)
}

func TestSettingsFormTailscale_ShowsTheLabelsItWillSet(t *testing.T) {
	form := NewSettingsFormTailscale(docker.ApplicationSettings{
		Name:    "budget",
		AppPort: 3000,
		Tailscale: docker.TailscaleSettings{
			Enabled: true,
			Mode:    docker.DocktailMode,
			Service: "budget",
			Path:    "/budget",
		},
	})

	view := ansi.Strip(form.View())

	assert.Contains(t, view, "Labels")
	assert.Contains(t, view, "docktail.service.enable=true")
	assert.Contains(t, view, "docktail.service.name=budget")
	assert.Contains(t, view, "docktail.service.port=3000")
	assert.Contains(t, view, "docktail.service.service-port=443")
	assert.Contains(t, view, "docktail.service.path=/budget")

	form.form.TextField(tailscaleNetworkField).SetValue("tailnet0")
	form.form.CheckboxField(tailscaleDirectField).Toggle()
	updateSettingsForm(&form, keyPressMsg("x"))

	updated := ansi.Strip(form.View())
	assert.Contains(t, updated, "docktail.service.network=tailnet0")
	assert.Contains(t, updated, "docktail.service.direct=false")
}

func TestSettingsFormTailscale_DisablingHidesTheLabels(t *testing.T) {
	original := docker.ApplicationSettings{
		Name: "budget",
		Tailscale: docker.TailscaleSettings{
			Enabled: true,
			Mode:    docker.DocktailMode,
			Service: "budget",
		},
	}

	form := NewSettingsFormTailscale(original)
	assert.Contains(t, ansi.Strip(form.View()), "docktail.service.name=budget")

	updateSettingsForm(&form, keyPressMsg(" "))
	assert.Contains(t, ansi.Strip(form.View()), "(not published)")
}

func TestSettingsFormTailscale_EnablingDefaultsTheMode(t *testing.T) {
	original := docker.ApplicationSettings{
		Name:  "budget",
		Image: "example/app:latest",
		Host:  "budget.example.com",
	}

	form := NewSettingsFormTailscale(original)
	form.form.CheckboxField(tailscaleEnabledField).Toggle()

	settings := tailscaleSubmit(t, &form)

	assert.True(t, settings.DocktailEnabled())
	assert.Equal(t, docker.DocktailMode, settings.Tailscale.Mode)
	assert.Equal(t, "budget", settings.EffectiveServiceName())
}

func TestSettingsFormTailscale_DisablingClearsTheService(t *testing.T) {
	original := docker.ApplicationSettings{
		Name:  "budget",
		Image: "example/app:latest",
		Host:  "budget.example.com",
		Tailscale: docker.TailscaleSettings{
			Enabled:     true,
			Mode:        docker.DocktailMode,
			Service:     "budget",
			ServicePort: 443,
		},
	}

	form := NewSettingsFormTailscale(original)
	form.form.CheckboxField(tailscaleEnabledField).Toggle()

	settings := tailscaleSubmit(t, &form)

	assert.False(t, settings.DocktailEnabled())
	assert.Nil(t, settings.DocktailLabels())
	assert.False(t, original.Equal(settings))
}

func TestSettingsFormTailscale_Cancel(t *testing.T) {
	form := NewSettingsFormTailscale(docker.ApplicationSettings{Name: "budget"})

	for form.form.Focused() != form.form.ItemCount()+1 {
		tailscalePressTab(&form)
	}

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormTailscale)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SettingsSectionCancelMsg)
	assert.True(t, ok, "expected SettingsSectionCancelMsg, got %T", msg)
}

func TestSettingsFormEmail_InitialState(t *testing.T) {
	settings := docker.ApplicationSettings{
		SMTP: docker.SMTPSettings{
			Server:   "smtp.example.com",
			Port:     "587",
			Username: "user@example.com",
			Password: "secret",
			From:     "noreply@example.com",
		},
	}
	form := NewSettingsFormEmail(settings)

	assert.Equal(t, 0, form.form.Focused())
	assert.Equal(t, "smtp.example.com", form.form.TextField(emailServerField).Value())
	assert.Equal(t, "587", form.form.TextField(emailPortField).Value())
	assert.Equal(t, "user@example.com", form.form.TextField(emailUsernameField).Value())
	assert.Equal(t, "secret", form.form.TextField(emailPasswordField).Value())
	assert.Equal(t, "noreply@example.com", form.form.TextField(emailFromField).Value())
}

func TestSettingsFormEmail_TabNavigation(t *testing.T) {
	form := NewSettingsFormEmail(docker.ApplicationSettings{})
	assert.Equal(t, 0, form.form.Focused())

	emailPressTab(&form)
	assert.Equal(t, 1, form.form.Focused(), "port")

	emailPressTab(&form)
	assert.Equal(t, 2, form.form.Focused(), "username")

	emailPressTab(&form)
	assert.Equal(t, 3, form.form.Focused(), "password")

	emailPressTab(&form)
	assert.Equal(t, 4, form.form.Focused(), "from")

	emailPressTab(&form)
	assert.Equal(t, 5, form.form.Focused(), "done button")

	emailPressTab(&form)
	assert.Equal(t, 6, form.form.Focused(), "cancel button")

	emailPressTab(&form)
	assert.Equal(t, 0, form.form.Focused(), "wraps to server")
}

func TestSettingsFormEmail_Submit(t *testing.T) {
	settings := docker.ApplicationSettings{
		Name: "myapp",
		SMTP: docker.SMTPSettings{
			Server: "smtp.example.com",
			Port:   "587",
		},
	}
	form := NewSettingsFormEmail(settings)

	for range 5 {
		emailPressTab(&form)
	}

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormEmail)
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	assert.Equal(t, "myapp", submitMsg.Settings.Name)
	assert.Equal(t, "smtp.example.com", submitMsg.Settings.SMTP.Server)
	assert.Equal(t, "587", submitMsg.Settings.SMTP.Port)
}

func TestSettingsFormEmail_Cancel(t *testing.T) {
	form := NewSettingsFormEmail(docker.ApplicationSettings{})

	for range 6 {
		emailPressTab(&form)
	}

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormEmail)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SettingsSectionCancelMsg)
	assert.True(t, ok, "expected SettingsSectionCancelMsg, got %T", msg)
}

func TestSettingsFormRegistry_Submit(t *testing.T) {
	settings := docker.ApplicationSettings{
		Name:     "myapp",
		Image:    "ghcr.io/basecamp/app:v1",
		Registry: docker.RegistrySettings{Host: "ghcr.io", Username: "user", Password: "pass"},
	}

	submitForm := func(t *testing.T, form SettingsFormRegistry) docker.ApplicationSettings {
		t.Helper()
		for form.form.Focused() != 2 {
			registryPressTab(&form)
		}
		result, cmd := form.Update(keyPressMsg("enter"))
		form = result.(SettingsFormRegistry)
		require.NotNil(t, cmd)
		submitMsg, ok := cmd().(SettingsSectionSubmitMsg)
		require.True(t, ok)
		return submitMsg.Settings
	}

	t.Run("shows credentials for the image's host", func(t *testing.T) {
		form := NewSettingsFormRegistry(settings)
		assert.Equal(t, "user", form.form.TextField(registryUsernameField).Value())
		assert.Equal(t, "pass", form.form.TextField(registryPasswordField).Value())
		assert.Equal(t, "Credentials for ghcr.io", form.StatusLine())
	})

	t.Run("starts empty when the stored host differs", func(t *testing.T) {
		s := settings
		s.Image = "registry.example.com/app:v1"
		form := NewSettingsFormRegistry(s)
		assert.Empty(t, form.form.TextField(registryUsernameField).Value())
		assert.Empty(t, form.form.TextField(registryPasswordField).Value())
		assert.Equal(t, "Credentials for registry.example.com", form.StatusLine())
	})

	t.Run("submit scopes credentials to the image's host", func(t *testing.T) {
		s := settings
		s.Image = "registry.example.com/app:v1"
		form := NewSettingsFormRegistry(s)
		form.form.TextField(registryUsernameField).SetValue("newuser")
		form.form.TextField(registryPasswordField).SetValue("newpass")

		result := submitForm(t, form)
		expected := docker.RegistrySettings{Host: "registry.example.com", Username: "newuser", Password: "newpass"}
		assert.Equal(t, expected, result.Registry)
	})

	t.Run("submit with empty fields clears the credentials", func(t *testing.T) {
		form := NewSettingsFormRegistry(settings)
		form.form.TextField(registryUsernameField).SetValue("")
		form.form.TextField(registryPasswordField).SetValue("")

		result := submitForm(t, form)
		assert.True(t, result.Registry.Empty())
	})
}

func TestSettingsFormRegistry_Cancel(t *testing.T) {
	form := NewSettingsFormRegistry(docker.ApplicationSettings{Image: "ghcr.io/basecamp/app:v1"})

	for range 3 {
		registryPressTab(&form)
	}

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormRegistry)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SettingsSectionCancelMsg)
	assert.True(t, ok, "expected SettingsSectionCancelMsg, got %T", msg)
}

func TestSettingsFormResources_InitialState(t *testing.T) {
	settings := docker.ApplicationSettings{
		Resources: docker.ContainerResources{
			CPUs:     2,
			MemoryMB: 512,
		},
	}
	form := NewSettingsFormResources(settings)

	assert.Equal(t, 0, form.form.Focused())
	assert.Equal(t, "2", form.form.TextField(resourcesCPUField).Value())
	assert.Equal(t, "512", form.form.TextField(resourcesMemoryField).Value())
}

func TestSettingsFormResources_InitialState_ZeroValues(t *testing.T) {
	form := NewSettingsFormResources(docker.ApplicationSettings{})

	assert.Equal(t, 0, form.form.Focused())
	assert.Equal(t, "", form.form.TextField(resourcesCPUField).Value())
	assert.Equal(t, "", form.form.TextField(resourcesMemoryField).Value())
}

func TestSettingsFormResources_TabNavigation(t *testing.T) {
	form := NewSettingsFormResources(docker.ApplicationSettings{})
	assert.Equal(t, 0, form.form.Focused())

	resourcesPressTab(&form)
	assert.Equal(t, 1, form.form.Focused(), "memory")

	resourcesPressTab(&form)
	assert.Equal(t, 2, form.form.Focused(), "done button")

	resourcesPressTab(&form)
	assert.Equal(t, 3, form.form.Focused(), "cancel button")

	resourcesPressTab(&form)
	assert.Equal(t, 0, form.form.Focused(), "wraps to cpu")
}

func TestSettingsFormResources_Submit(t *testing.T) {
	form := NewSettingsFormResources(docker.ApplicationSettings{Name: "myapp"})

	resourcesTypeText(&form, "2")
	resourcesPressTab(&form)
	resourcesTypeText(&form, "1024")
	resourcesPressTab(&form)

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormResources)
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	assert.Equal(t, "myapp", submitMsg.Settings.Name)
	assert.Equal(t, 2, submitMsg.Settings.Resources.CPUs)
	assert.Equal(t, 1024, submitMsg.Settings.Resources.MemoryMB)
}

func TestSettingsFormResources_SubmitBlank(t *testing.T) {
	form := NewSettingsFormResources(docker.ApplicationSettings{})

	resourcesPressTab(&form)
	resourcesPressTab(&form)

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormResources)
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	assert.Equal(t, 0, submitMsg.Settings.Resources.CPUs)
	assert.Equal(t, 0, submitMsg.Settings.Resources.MemoryMB)
}

func TestSettingsFormResources_Cancel(t *testing.T) {
	form := NewSettingsFormResources(docker.ApplicationSettings{})

	for range 3 {
		resourcesPressTab(&form)
	}

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormResources)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SettingsSectionCancelMsg)
	assert.True(t, ok, "expected SettingsSectionCancelMsg, got %T", msg)
}

func TestSettingsFormBackups_ActionReadsCurrentFieldValue(t *testing.T) {
	app := &docker.Application{
		Settings: docker.ApplicationSettings{
			Name:   "chat",
			Backup: docker.BackupSettings{Path: "/old/path"},
		},
	}
	form := NewSettingsFormBackups(app, nil)

	assert.Equal(t, "/old/path", form.form.TextField(backupsPathField).Value())

	// Type a new path into the field
	form.form.TextField(backupsPathField).SetValue("/new/path")

	// Tab to Done, then to the action button, then press enter
	backupsPressTab(&form)
	backupsPressTab(&form)
	backupsPressTab(&form)
	assert.Equal(t, form.form.actionIndex(), form.form.Focused(), "action button focused")

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormBackups)
	require.NotNil(t, cmd)
	msg := cmd()
	actionMsg, ok := msg.(settingsRunActionMsg)
	require.True(t, ok, "expected settingsRunActionMsg, got %T", msg)

	// Run the action — it will fail (no Docker) but should use the new path
	_, err := actionMsg.action()
	require.Error(t, err)
	// The error should NOT be "backup location is required", proving it read "/new/path"
	assert.NotContains(t, err.Error(), "backup location is required")
}

func TestSettingsFormBackups_Submit(t *testing.T) {
	app := &docker.Application{
		Settings: docker.ApplicationSettings{Name: "chat"},
	}
	form := NewSettingsFormBackups(app, nil)

	// Type a path
	backupsTypeText(&form, "/my/backups")
	backupsPressTab(&form)

	// Toggle auto-backup
	backupsPressSpace(&form)

	// Tab to Done
	backupsPressTab(&form)

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormBackups)
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	assert.Equal(t, "/my/backups", submitMsg.Settings.Backup.Path)
	assert.True(t, submitMsg.Settings.Backup.AutoBackup)
}

// Helpers

func updateSettingsForm[T any](form *T, msg tea.Msg) {
	section, ok := any(*form).(SettingsSection)
	if !ok {
		return
	}
	result, _ := section.Update(msg)
	*form = result.(T)
}

func applicationTypeText(form *SettingsFormApplication, text string) {
	for _, r := range text {
		updateSettingsForm(form, keyPressMsg(string(r)))
	}
}

func applicationClearAndType(form *SettingsFormApplication, text string) {
	form.form.TextField(appHostnameField).SetValue("")
	applicationTypeText(form, text)
}

func applicationPressTab(form *SettingsFormApplication) {
	updateSettingsForm(form, keyPressMsg("tab"))
}

func applicationPressShiftTab(form *SettingsFormApplication) {
	updateSettingsForm(form, keyPressMsg("shift+tab"))
}

func applicationPressSpace(form *SettingsFormApplication) {
	updateSettingsForm(form, keyPressMsg(" "))
}

func applicationTabToSubmit(form *SettingsFormApplication) {
	for form.form.Focused() != form.form.ItemCount() {
		applicationPressTab(form)
	}
}

func applicationTabToCancel(form *SettingsFormApplication) {
	for form.form.Focused() != form.form.ItemCount()+1 {
		applicationPressTab(form)
	}
}

func applicationSubmit(t *testing.T, form *SettingsFormApplication) docker.ApplicationSettings {
	t.Helper()

	applicationTabToSubmit(form)

	result, cmd := form.Update(keyPressMsg("enter"))
	*form = result.(SettingsFormApplication)
	require.NotNil(t, cmd)

	msg, ok := cmd().(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	return msg.Settings
}

func emailPressTab(form *SettingsFormEmail) {
	updateSettingsForm(form, keyPressMsg("tab"))
}

func registryPressTab(form *SettingsFormRegistry) {
	updateSettingsForm(form, keyPressMsg("tab"))
}

func resourcesPressTab(form *SettingsFormResources) {
	updateSettingsForm(form, keyPressMsg("tab"))
}

func resourcesTypeText(form *SettingsFormResources, text string) {
	for _, r := range text {
		updateSettingsForm(form, keyPressMsg(string(r)))
	}
}

func backupsPressTab(form *SettingsFormBackups) {
	updateSettingsForm(form, keyPressMsg("tab"))
}

func backupsPressSpace(form *SettingsFormBackups) {
	updateSettingsForm(form, keyPressMsg(" "))
}

func backupsTypeText(form *SettingsFormBackups, text string) {
	for _, r := range text {
		updateSettingsForm(form, keyPressMsg(string(r)))
	}
}

func TestSettingsFormEnvironment_InitialState_Empty(t *testing.T) {
	form := NewSettingsFormEnvironment(docker.ApplicationSettings{})

	assert.Equal(t, 2, form.form.ItemCount(), "one empty row = 2 items")
	assert.Equal(t, "", form.form.TextField(0).Value())
	assert.Equal(t, "", form.form.TextField(1).Value())
	assert.Equal(t, 0, form.form.Focused())
}

func TestSettingsFormEnvironment_InitialState_WithVars(t *testing.T) {
	form := NewSettingsFormEnvironment(docker.ApplicationSettings{
		EnvVars: map[string]string{
			"DB_HOST": "postgres.local",
			"APP_ENV": "production",
		},
	})

	assert.Equal(t, 6, form.form.ItemCount(), "2 populated rows + 1 empty = 6 items")
	assert.Equal(t, "APP_ENV", form.form.TextField(0).Value(), "sorted alphabetically")
	assert.Equal(t, "production", form.form.TextField(1).Value())
	assert.Equal(t, "DB_HOST", form.form.TextField(2).Value())
	assert.Equal(t, "postgres.local", form.form.TextField(3).Value())
	assert.Equal(t, "", form.form.TextField(4).Value(), "trailing empty key")
}

func TestSettingsFormEnvironment_TabNavigation(t *testing.T) {
	form := NewSettingsFormEnvironment(docker.ApplicationSettings{
		EnvVars: map[string]string{"A": "1"},
	})
	// items: [A.key, A.val, empty.key, empty.val] + Done + Cancel
	assert.Equal(t, 0, form.form.Focused())

	environmentPressTab(&form)
	assert.Equal(t, 1, form.form.Focused(), "A value")

	environmentPressTab(&form)
	assert.Equal(t, 2, form.form.Focused(), "empty key")

	environmentPressTab(&form)
	assert.Equal(t, 3, form.form.Focused(), "empty value")

	environmentPressTab(&form)
	assert.Equal(t, 4, form.form.Focused(), "done button")

	environmentPressTab(&form)
	assert.Equal(t, 5, form.form.Focused(), "cancel button")

	environmentPressTab(&form)
	assert.Equal(t, 0, form.form.Focused(), "wraps to first key")
}

func TestSettingsFormEnvironment_Submit_WithValues(t *testing.T) {
	form := NewSettingsFormEnvironment(docker.ApplicationSettings{Name: "myapp"})

	environmentTypeText(&form, "DB_HOST")
	environmentPressTab(&form)
	environmentTypeText(&form, "localhost")
	// Auto-grow added a new row, so: key, val, empty key, empty val, done
	environmentPressTab(&form) // new empty key
	environmentPressTab(&form) // new empty value
	environmentPressTab(&form) // done button

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormEnvironment)
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	assert.Equal(t, "myapp", submitMsg.Settings.Name)
	assert.Equal(t, map[string]string{"DB_HOST": "localhost"}, submitMsg.Settings.EnvVars)
}

func TestSettingsFormEnvironment_Submit_Empty(t *testing.T) {
	form := NewSettingsFormEnvironment(docker.ApplicationSettings{Name: "myapp"})

	environmentPressTab(&form) // empty value
	environmentPressTab(&form) // done

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormEnvironment)
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg, ok := msg.(SettingsSectionSubmitMsg)
	require.True(t, ok)
	assert.Nil(t, submitMsg.Settings.EnvVars)
}

func TestSettingsFormEnvironment_Submit_SkipsEmptyKeys(t *testing.T) {
	form := NewSettingsFormEnvironment(docker.ApplicationSettings{
		EnvVars: map[string]string{"KEEP": "yes"},
	})

	// Tab to the empty row's value and type something (key stays empty)
	environmentPressTab(&form) // KEEP value
	environmentPressTab(&form) // empty key
	environmentPressTab(&form) // empty value
	environmentTypeText(&form, "orphan")
	// Navigate to done
	for form.form.Focused() != form.form.submitIndex() {
		environmentPressTab(&form)
	}

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormEnvironment)
	require.NotNil(t, cmd)
	msg := cmd()
	submitMsg := msg.(SettingsSectionSubmitMsg)
	assert.Equal(t, map[string]string{"KEEP": "yes"}, submitMsg.Settings.EnvVars)
}

func TestSettingsFormEnvironment_Cancel(t *testing.T) {
	form := NewSettingsFormEnvironment(docker.ApplicationSettings{})

	// Navigate to cancel button
	environmentPressTab(&form) // empty value
	environmentPressTab(&form) // done
	environmentPressTab(&form) // cancel

	result, cmd := form.Update(keyPressMsg("enter"))
	form = result.(SettingsFormEnvironment)
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SettingsSectionCancelMsg)
	assert.True(t, ok, "expected SettingsSectionCancelMsg, got %T", msg)
}

func TestSettingsFormEnvironment_AutoGrow(t *testing.T) {
	form := NewSettingsFormEnvironment(docker.ApplicationSettings{})
	assert.Equal(t, 2, form.form.ItemCount(), "1 empty row = 2 items")

	environmentTypeText(&form, "X")
	assert.Equal(t, 4, form.form.ItemCount(), "typing in last key adds new row")

	environmentTypeText(&form, "Y")
	assert.Equal(t, 4, form.form.ItemCount(), "no extra row while still in same key")
}

func TestSettingsFormEnvironment_AutoGrow_VisibleInView(t *testing.T) {
	form := NewSettingsFormEnvironment(docker.ApplicationSettings{})
	environmentSendWindowSize(&form, 80, 40)

	environmentTypeText(&form, "NEW_KEY")
	view := form.View()

	assert.Contains(t, view, "NEW_KEY", "typed key visible in rendered view")
}

func TestSettingsFormEnvironment_FocusBorder_MovesOnTab(t *testing.T) {
	form := NewSettingsFormEnvironment(docker.ApplicationSettings{
		EnvVars: map[string]string{"A": "1"},
	})
	environmentSendWindowSize(&form, 80, 40)

	view1 := form.View()

	environmentPressTab(&form)
	view2 := form.View()

	assert.NotEqual(t, view1, view2, "view changes when focus moves")
}

func environmentPressTab(form *SettingsFormEnvironment) {
	updateSettingsForm(form, keyPressMsg("tab"))
}

func environmentTypeText(form *SettingsFormEnvironment, text string) {
	for _, r := range text {
		updateSettingsForm(form, keyPressMsg(string(r)))
	}
}

func environmentSendWindowSize(form *SettingsFormEnvironment, w, h int) {
	updateSettingsForm(form, tea.WindowSizeMsg{Width: w, Height: h})
}

func tailscalePressTab(form *SettingsFormTailscale) {
	updateSettingsForm(form, keyPressMsg("tab"))
}

func tailscaleSubmit(t *testing.T, form *SettingsFormTailscale) docker.ApplicationSettings {
	t.Helper()

	for form.form.Focused() != form.form.ItemCount() {
		tailscalePressTab(form)
	}

	result, cmd := form.Update(keyPressMsg("enter"))
	*form = result.(SettingsFormTailscale)
	require.NotNil(t, cmd)

	msg, ok := cmd().(SettingsSectionSubmitMsg)
	require.True(t, ok, "expected SettingsSectionSubmitMsg, got %T", msg)
	return msg.Settings
}
