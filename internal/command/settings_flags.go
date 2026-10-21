package command

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/basecamp/once/internal/docker"
)

type settingsFlags struct {
	host            string
	disableTLS      bool
	env             []string
	smtpServer      string
	smtpPort        string
	smtpUsername    string
	smtpPassword    string
	smtpFrom        string
	cpus            int
	memory          int
	autoUpdate      bool
	backupPath      string
	autoBackup      bool
	healthCheckPath string
	appPort         int
	volumePaths     []string
	skipRailsEnv    bool

	docktail                bool
	docktailService         string
	docktailPort            int
	docktailServicePort     int
	docktailPath            string
	docktailTags            string
	docktailDescription     string
	docktailProtocol        string
	docktailServiceProtocol string
	docktailProxyProtocol   string
	docktailNetwork         string
	docktailDirect          string

	registryUsername      string
	registryPassword      string
	registryPasswordStdin bool
}

func (f *settingsFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.host, "host", "", "hostname for the application")
	cmd.Flags().BoolVar(&f.disableTLS, "disable-tls", false, "disable TLS for this application")
	cmd.Flags().StringArrayVar(&f.env, "env", nil, "environment variable in KEY=VALUE format (can be repeated)")
	cmd.Flags().StringVar(&f.smtpServer, "smtp-server", "", "SMTP server address")
	cmd.Flags().StringVar(&f.smtpPort, "smtp-port", "", "SMTP server port")
	cmd.Flags().StringVar(&f.smtpUsername, "smtp-username", "", "SMTP username")
	cmd.Flags().StringVar(&f.smtpPassword, "smtp-password", "", "SMTP password")
	cmd.Flags().StringVar(&f.smtpFrom, "smtp-from", "", "SMTP from address")
	cmd.Flags().IntVar(&f.cpus, "cpus", 0, "CPU limit for the container")
	cmd.Flags().IntVar(&f.memory, "memory", 0, "memory limit in MB for the container")
	cmd.Flags().BoolVar(&f.autoUpdate, "auto-update", true, "automatically update the application")
	cmd.Flags().StringVar(&f.backupPath, "backup-path", "", "path for backups")
	cmd.Flags().BoolVar(&f.autoBackup, "auto-backup", false, "enable automatic backups")
	cmd.Flags().StringVar(&f.healthCheckPath, "health-check-path", "", "path used to check the application is healthy")
	cmd.Flags().IntVar(&f.appPort, "app-port", 0, "port the application listens on inside its container")
	cmd.Flags().StringSliceVar(&f.volumePaths, "volume-paths", nil, "paths where the application's data volume is mounted (comma-separated)")
	cmd.Flags().BoolVar(&f.skipRailsEnv, "skip-rails-env", false, "skip injecting the Rails-specific environment variables")
	cmd.Flags().BoolVar(&f.docktail, "docktail", false, "expose the application as a Tailscale Service via docktail")
	cmd.Flags().StringVar(&f.docktailService, "docktail-service", "", "Tailscale Service name (defaults to the application name)")
	cmd.Flags().IntVar(&f.docktailPort, "docktail-port", 0, "container port docktail should proxy to (defaults to the app port)")
	cmd.Flags().IntVar(&f.docktailServicePort, "docktail-service-port", 0, "port Tailscale should listen on (defaults to 443 when HTTPS, 80 otherwise)")
	cmd.Flags().StringVar(&f.docktailPath, "docktail-path", "", "URL path to serve the application under (defaults to /)")
	cmd.Flags().StringVar(&f.docktailTags, "docktail-tags", "", "comma-separated Tailscale Service tags")
	cmd.Flags().StringVar(&f.docktailDescription, "docktail-description", "", "human-readable description for the Tailscale Service")
	cmd.Flags().StringVar(&f.docktailProtocol, "docktail-protocol", "", "backend protocol (http, https, https+insecure, tcp or tls-terminated-tcp; defaults to https on port 443, otherwise http)")
	cmd.Flags().StringVar(&f.docktailServiceProtocol, "docktail-service-protocol", "", "Tailscale-facing protocol (http, https, tcp or tls-terminated-tcp)")
	cmd.Flags().StringVar(&f.docktailProxyProtocol, "docktail-proxy-protocol", "", "PROXY protocol version to prepend on TCP forwards (1 or 2)")
	cmd.Flags().StringVar(&f.docktailNetwork, "docktail-network", "", "Docker network used for container IP detection")
	cmd.Flags().StringVar(&f.docktailDirect, "docktail-direct", "", "proxy directly to the container IP (default true); false publishes the backend port on loopback for docktail to reach")
	cmd.Flags().StringVar(&f.registryUsername, "registry-username", "", "registry username for pulling the application image")
	cmd.Flags().StringVar(&f.registryPassword, "registry-password", "", "registry password for pulling the application image")
	cmd.Flags().BoolVar(&f.registryPasswordStdin, "registry-password-stdin", false, "read the registry password from stdin")
	cmd.MarkFlagsMutuallyExclusive("registry-password", "registry-password-stdin")
}

func (f *settingsFlags) buildSettings(cmd *cobra.Command, image, host string) (docker.ApplicationSettings, error) {
	envVars, err := f.parseEnvVars()
	if err != nil {
		return docker.ApplicationSettings{}, err
	}

	direct, err := parseOptionalBool(f.docktailDirect)
	if err != nil {
		return docker.ApplicationSettings{}, err
	}

	if f.backupPath != "" && !filepath.IsAbs(f.backupPath) {
		return docker.ApplicationSettings{}, docker.ErrBackupPathRelative
	}

	s := docker.ApplicationSettings{
		Image:      image,
		Host:       host,
		DisableTLS: f.disableTLS,
		EnvVars:    envVars,
		SMTP: docker.SMTPSettings{
			Server:   f.smtpServer,
			Port:     f.smtpPort,
			Username: f.smtpUsername,
			Password: f.smtpPassword,
			From:     f.smtpFrom,
		},
		Resources: docker.ContainerResources{
			CPUs:     f.cpus,
			MemoryMB: f.memory,
		},
		AutoUpdate: f.autoUpdate,
		Backup: docker.BackupSettings{
			Path:       f.backupPath,
			AutoBackup: f.autoBackup,
		},
		HealthCheckPath: f.healthCheckPath,
		AppPort:         f.appPort,
		VolumePaths:     docker.ParseVolumePaths(strings.Join(f.volumePaths, ",")),
		SkipRailsEnv:    f.skipRailsEnv,
		Tailscale: docker.TailscaleSettings{
			Enabled:         f.docktail,
			Service:         f.docktailService,
			Port:            f.docktailPort,
			ServicePort:     f.docktailServicePort,
			Path:            f.docktailPath,
			Tags:            f.docktailTags,
			Description:     f.docktailDescription,
			Protocol:        f.docktailProtocol,
			ServiceProtocol: f.docktailServiceProtocol,
			ProxyProtocol:   f.docktailProxyProtocol,
			Direct:          direct,
			Network:         f.docktailNetwork,
		},
	}

	if f.docktail {
		s.Tailscale.Mode = docker.DocktailMode
	}

	if f.registryUsername != "" || f.registryPassword != "" || f.registryPasswordStdin {
		password, err := f.registryPasswordValue(cmd)
		if err != nil {
			return docker.ApplicationSettings{}, err
		}
		host, err := docker.RegistryHost(image)
		if err != nil {
			return docker.ApplicationSettings{}, err
		}
		s.Registry = docker.RegistrySettings{
			Host:     host,
			Username: f.registryUsername,
			Password: password,
		}
	}

	if err := s.Validate(); err != nil {
		return docker.ApplicationSettings{}, err
	}

	return s, nil
}

func (f *settingsFlags) applyChanges(cmd *cobra.Command, existing docker.ApplicationSettings, image string) (docker.ApplicationSettings, error) {
	s := existing

	s.Image = image

	if cmd.Flags().Changed("host") {
		s.Host = f.host
	}
	if cmd.Flags().Changed("disable-tls") {
		s.DisableTLS = f.disableTLS
	}
	if cmd.Flags().Changed("env") {
		envVars, err := f.parseEnvVars()
		if err != nil {
			return s, err
		}
		s.EnvVars = envVars
	}
	if cmd.Flags().Changed("smtp-server") {
		s.SMTP.Server = f.smtpServer
	}
	if cmd.Flags().Changed("smtp-port") {
		s.SMTP.Port = f.smtpPort
	}
	if cmd.Flags().Changed("smtp-username") {
		s.SMTP.Username = f.smtpUsername
	}
	if cmd.Flags().Changed("smtp-password") {
		s.SMTP.Password = f.smtpPassword
	}
	if cmd.Flags().Changed("smtp-from") {
		s.SMTP.From = f.smtpFrom
	}
	if cmd.Flags().Changed("cpus") {
		s.Resources.CPUs = f.cpus
	}
	if cmd.Flags().Changed("memory") {
		s.Resources.MemoryMB = f.memory
	}
	if cmd.Flags().Changed("auto-update") {
		s.AutoUpdate = f.autoUpdate
	}
	if cmd.Flags().Changed("backup-path") {
		if f.backupPath != "" && !filepath.IsAbs(f.backupPath) {
			return s, docker.ErrBackupPathRelative
		}
		s.Backup.Path = f.backupPath
	}
	if cmd.Flags().Changed("auto-backup") {
		s.Backup.AutoBackup = f.autoBackup
	}
	if cmd.Flags().Changed("health-check-path") {
		s.HealthCheckPath = f.healthCheckPath
	}
	if cmd.Flags().Changed("app-port") {
		s.AppPort = f.appPort
	}
	if cmd.Flags().Changed("volume-paths") {
		s.VolumePaths = docker.ParseVolumePaths(strings.Join(f.volumePaths, ","))
	}
	if cmd.Flags().Changed("skip-rails-env") {
		s.SkipRailsEnv = f.skipRailsEnv
	}
	if cmd.Flags().Changed("docktail") {
		s.Tailscale.Enabled = f.docktail
		if f.docktail {
			s.Tailscale.Mode = docker.DocktailMode
		} else {
			s.Tailscale.Mode = ""
		}
	}
	if cmd.Flags().Changed("docktail-service") {
		s.Tailscale.Service = f.docktailService
	}
	if cmd.Flags().Changed("docktail-port") {
		s.Tailscale.Port = f.docktailPort
	}
	if cmd.Flags().Changed("docktail-service-port") {
		s.Tailscale.ServicePort = f.docktailServicePort
	}
	if cmd.Flags().Changed("docktail-path") {
		s.Tailscale.Path = f.docktailPath
	}
	if cmd.Flags().Changed("docktail-tags") {
		s.Tailscale.Tags = f.docktailTags
	}
	if cmd.Flags().Changed("docktail-description") {
		s.Tailscale.Description = f.docktailDescription
	}
	if cmd.Flags().Changed("docktail-protocol") {
		s.Tailscale.Protocol = f.docktailProtocol
	}
	if cmd.Flags().Changed("docktail-service-protocol") {
		s.Tailscale.ServiceProtocol = f.docktailServiceProtocol
	}
	if cmd.Flags().Changed("docktail-proxy-protocol") {
		s.Tailscale.ProxyProtocol = f.docktailProxyProtocol
	}
	if cmd.Flags().Changed("docktail-network") {
		s.Tailscale.Network = f.docktailNetwork
	}
	if cmd.Flags().Changed("docktail-direct") {
		direct, err := parseOptionalBool(f.docktailDirect)
		if err != nil {
			return s, err
		}
		s.Tailscale.Direct = direct
	}
	usernameChanged := cmd.Flags().Changed("registry-username")
	passwordChanged := cmd.Flags().Changed("registry-password") || cmd.Flags().Changed("registry-password-stdin")
	if usernameChanged != passwordChanged {
		// Requiring the pair prevents re-scoping a stored credential to a
		// new registry host alongside a partial update.
		if usernameChanged {
			return s, docker.ErrRegistryUsernameWithoutPassword
		}
		return s, docker.ErrRegistryPasswordWithoutUsername
	}
	if usernameChanged {
		password, err := f.registryPasswordValue(cmd)
		if err != nil {
			return s, err
		}
		host, err := docker.RegistryHost(image)
		if err != nil {
			return s, err
		}
		s.Registry = docker.RegistrySettings{
			Host:     host,
			Username: f.registryUsername,
			Password: password,
		}
		if f.registryUsername == "" && password == "" {
			s.Registry = docker.RegistrySettings{}
		}
	}

	if err := s.Validate(); err != nil {
		return s, err
	}

	return s, nil
}

func (f *settingsFlags) registryPasswordValue(cmd *cobra.Command) (string, error) {
	if !f.registryPasswordStdin {
		return f.registryPassword, nil
	}
	b, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return "", fmt.Errorf("reading registry password from stdin: %w", err)
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

func (f *settingsFlags) parseEnvVars() (map[string]string, error) {
	if f.env == nil {
		return nil, nil
	}

	envVars := make(map[string]string, len(f.env))
	for _, e := range f.env {
		key, value, ok := strings.Cut(e, "=")
		if !ok {
			return nil, fmt.Errorf("invalid environment variable %q: must be in KEY=VALUE format", e)
		}
		if key == "" {
			return nil, fmt.Errorf("invalid environment variable %q: key must not be empty", e)
		}
		envVars[key] = value
	}

	return envVars, nil
}

// Helpers

func parseOptionalBool(value string) (*bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return nil, nil
	case "true":
		enabled := true
		return &enabled, nil
	case "false":
		enabled := false
		return &enabled, nil
	}
	return nil, fmt.Errorf("invalid boolean %q: must be true or false", value)
}
